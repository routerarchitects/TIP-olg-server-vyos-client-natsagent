package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/state"
)

var portKeyPattern = regexp.MustCompile(`^(eth|lan|wan)[0-9]*$`)

// StateStore abstracts reading configuration state checkpoints.
type StateStore interface {
	Load(ctx context.Context) (state.State, error)
}

type counterBaseline struct {
	rxBytes   uint64
	txBytes   uint64
	rxPackets uint64
	txPackets uint64
	sampledAt time.Time
}

// Collector coordinates router telemetry gathering and packaging into StateEnvelope.
type Collector struct {
	target             string
	stateStore         StateStore
	isTopologyDegraded func() bool
	reader             SystemReader
	logger             agentcore.Logger
	now                func() time.Time

	mu        sync.Mutex
	baselines map[string]counterBaseline
}

// Config holds dependencies for Collector creation.
type Config struct {
	Target             string
	StateStore         StateStore
	IsTopologyDegraded func() bool
	Reader             SystemReader
	Logger             agentcore.Logger
	Now                func() time.Time
}

// New creates a new Collector instance.
func New(cfg Config) (*Collector, error) {
	if cfg.Target == "" {
		return nil, fmt.Errorf("collector target is required")
	}
	if cfg.StateStore == nil {
		return nil, fmt.Errorf("collector state store is required")
	}
	if cfg.Reader == nil {
		cfg.Reader = NewLinuxSystemReader()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	return &Collector{
		target:             cfg.Target,
		stateStore:         cfg.StateStore,
		isTopologyDegraded: cfg.IsTopologyDegraded,
		reader:             cfg.Reader,
		logger:             cfg.Logger,
		now:                cfg.Now,
		baselines:          make(map[string]counterBaseline),
	}, nil
}

// HandleStateGet implements agentcore.StateGetHandler for on-demand live state queries.
func (c *Collector) HandleStateGet(ctx context.Context) (agentcore.StateEnvelope, error) {
	return c.Collect(ctx, false)
}

// Collect gathers a full telemetry snapshot. When isPeriodic is false, delta baselines are not mutated.
func (c *Collector) Collect(ctx context.Context, isPeriodic bool) (agentcore.StateEnvelope, error) {
	var appliedUUID string
	var portRoles map[string]string

	st, err := c.stateStore.Load(ctx)
	if err != nil {
		c.logWarn("failed to load state checkpoint, assuming empty state", "error", err)
		appliedUUID = ""
	} else {
		appliedUUID = st.AppliedUUID
		degraded := st.TopologyDegraded
		if c.isTopologyDegraded != nil && c.isTopologyDegraded() {
			degraded = true
		}

		if degraded {
			c.logWarn("topology state is degraded; omitting physical port roles from link-state", "target", c.target)
			portRoles = nil
		} else {
			portRoles = st.PhysicalPortRoles
		}
	}

	now := c.now().UTC()

	loadAvg, err := c.reader.ReadLoadAvg()
	if err != nil {
		c.logWarn("failed to read load averages", "error", err)
	}

	cpuLoad, err := c.reader.ReadCPULoad()
	if err != nil {
		c.logWarn("failed to read cpu load", "error", err)
	}
	if cpuLoad == nil {
		cpuLoad = []int{0}
	}

	memState, err := c.reader.ReadMemInfo()
	if err != nil {
		c.logWarn("failed to read meminfo", "error", err)
	}

	uptime, err := c.reader.ReadUptime()
	if err != nil {
		c.logWarn("failed to read uptime", "error", err)
	}

	unitState := UnitState{
		Load:      loadAvg,
		CPULoad:   cpuLoad,
		LocalTime: now.Unix(),
		Memory:    memState,
		Uptime:    uptime,
	}

	rawInterfaces, err := c.reader.ReadInterfaces(ctx)
	if err != nil {
		c.logWarn("failed to read interfaces", "error", err)
	}

	rawNeighbors, err := c.reader.ReadNeighbors(ctx)
	if err != nil {
		c.logWarn("failed to read neighbors", "error", err)
	}

	clientsByDev := c.processNeighbors(rawNeighbors)

	rawMap := make(map[string]InterfaceRaw, len(rawInterfaces))
	for _, raw := range rawInterfaces {
		rawMap[raw.Name] = raw
	}

	interfaces := make([]InterfaceState, 0)
	currentIfaces := make(map[string]bool)

	c.mu.Lock()
	if len(portRoles) > 0 {
		// Group physical ports by role strictly using PhysicalPortRoles
		roleToPorts := make(map[string][]string)
		for port, role := range portRoles {
			if !portKeyPattern.MatchString(port) {
				continue
			}
			roleLower := strings.ToLower(role)
			roleToPorts[roleLower] = append(roleToPorts[roleLower], port)
		}

		// Ensure standard deterministic role ordering: "upstream", "downstream", then others
		orderedRoles := make([]string, 0, len(roleToPorts))
		if _, ok := roleToPorts["upstream"]; ok {
			orderedRoles = append(orderedRoles, "upstream")
		}
		if _, ok := roleToPorts["downstream"]; ok {
			orderedRoles = append(orderedRoles, "downstream")
		}
		for r := range roleToPorts {
			if r != "upstream" && r != "downstream" {
				orderedRoles = append(orderedRoles, r)
			}
		}

		for _, role := range orderedRoles {
			ports := roleToPorts[role]
			sort.Strings(ports)

			ifaceName := role
			switch strings.ToLower(role) {
			case "upstream":
				ifaceName = "upv0"
			case "downstream":
				ifaceName = "downv0"
			}

			// Collect associated devices (physical ports + master bridge if present)
			associatedDevs := make(map[string]bool)
			for _, p := range ports {
				associatedDevs[p] = true
				if link, err := os.Readlink(fmt.Sprintf("/sys/class/net/%s/master", p)); err == nil {
					master := filepath.Base(link)
					if master != "" && master != "." && master != "/" {
						associatedDevs[master] = true
					}
				}
			}

			// Aggregate counters strictly from the physical wire ports
			var currCounters InterfaceCounters
			for _, p := range ports {
				if raw, ok := rawMap[p]; ok {
					currCounters.RxBytes += raw.Counters.RxBytes
					currCounters.RxPackets += raw.Counters.RxPackets
					currCounters.RxErrors += raw.Counters.RxErrors
					currCounters.RxDropped += raw.Counters.RxDropped
					currCounters.TxBytes += raw.Counters.TxBytes
					currCounters.TxPackets += raw.Counters.TxPackets
					currCounters.TxErrors += raw.Counters.TxErrors
					currCounters.TxDropped += raw.Counters.TxDropped
					currCounters.Multicast += raw.Counters.Multicast
					currCounters.Collisions += raw.Counters.Collisions
				}
			}

			// IP Addresses & Uptime: check physical ports first, fallback to bridge
			var ipv4Addrs []string
			var ipv6Addrs []InterfaceIPv6Address
			var dhcpServer string
			var leaseTime uint32
			var maxUptime uint64

			for _, p := range ports {
				if raw, ok := rawMap[p]; ok {
					if raw.Uptime > maxUptime {
						maxUptime = raw.Uptime
					}
					if len(raw.IPv4Addresses) > 0 {
						ipv4Addrs = append(ipv4Addrs, raw.IPv4Addresses...)
					}
					if raw.DHCPServer != "" && dhcpServer == "" {
						dhcpServer = raw.DHCPServer
					}
					if raw.LeaseTime > 0 && leaseTime == 0 {
						leaseTime = raw.LeaseTime
					}
					if len(raw.IPv6Addresses) > 0 {
						ipv6Addrs = append(ipv6Addrs, raw.IPv6Addresses...)
					}
				}
			}

			if len(ipv4Addrs) == 0 {
				for d := range associatedDevs {
					if raw, ok := rawMap[d]; ok {
						if raw.Uptime > maxUptime {
							maxUptime = raw.Uptime
						}
						if len(raw.IPv4Addresses) > 0 {
							ipv4Addrs = append(ipv4Addrs, raw.IPv4Addresses...)
						}
						if raw.DHCPServer != "" && dhcpServer == "" {
							dhcpServer = raw.DHCPServer
						}
						if raw.LeaseTime > 0 && leaseTime == 0 {
							leaseTime = raw.LeaseTime
						}
						if len(raw.IPv6Addresses) > 0 && len(ipv6Addrs) == 0 {
							ipv6Addrs = append(ipv6Addrs, raw.IPv6Addresses...)
						}
					}
				}
			}

			// Clients: collect and deduplicate from all associated devices (bridge + ports)
			var clients []InterfaceClient
			seenMACs := make(map[string]bool)
			for d := range associatedDevs {
				for _, cl := range clientsByDev[d] {
					macLower := strings.ToLower(cl.MAC)
					if !seenMACs[macLower] {
						seenMACs[macLower] = true
						clients = append(clients, cl)
					}
				}
			}

			// Baseline and delta calculation for the interface
			var deltas InterfaceDeltaCounters
			b, hasBaseline := c.baselines[ifaceName]
			if !hasBaseline || currCounters.RxBytes < b.rxBytes || currCounters.TxBytes < b.txBytes ||
				currCounters.RxPackets < b.rxPackets || currCounters.TxPackets < b.txPackets {
				deltas = InterfaceDeltaCounters{
					RxBytes:   0,
					RxPackets: 0,
					TxBytes:   0,
					TxPackets: 0,
				}
				if isPeriodic {
					c.baselines[ifaceName] = counterBaseline{
						rxBytes:   currCounters.RxBytes,
						txBytes:   currCounters.TxBytes,
						rxPackets: currCounters.RxPackets,
						txPackets: currCounters.TxPackets,
						sampledAt: now,
					}
				}
			} else {
				deltas = InterfaceDeltaCounters{
					RxBytes:   currCounters.RxBytes - b.rxBytes,
					RxPackets: currCounters.RxPackets - b.rxPackets,
					TxBytes:   currCounters.TxBytes - b.txBytes,
					TxPackets: currCounters.TxPackets - b.txPackets,
				}
				if isPeriodic {
					c.baselines[ifaceName] = counterBaseline{
						rxBytes:   currCounters.RxBytes,
						txBytes:   currCounters.TxBytes,
						rxPackets: currCounters.RxPackets,
						txPackets: currCounters.TxPackets,
						sampledAt: now,
					}
				}
			}

			ifaceState := InterfaceState{
				Name:          ifaceName,
				Uptime:        maxUptime,
				IPv6Addresses: ipv6Addrs,
				Counters:      currCounters,
				DeltaCounters: deltas,
				Clients:       clients,
			}
			if len(ipv4Addrs) > 0 || dhcpServer != "" || leaseTime > 0 {
				ifaceState.IPv4 = &InterfaceIPv4{
					Addresses:  ipv4Addrs,
					DHCPServer: dhcpServer,
					LeaseTime:  leaseTime,
				}
			}
			interfaces = append(interfaces, ifaceState)
			currentIfaces[ifaceName] = true
		}
	} else {
		// Fallback for unconfigured or mock testing environments without port roles:
		// Filter out internal system interfaces (lo, mgmt0, pim*)
		for _, raw := range rawInterfaces {
			if raw.Name == "lo" || raw.Name == "mgmt0" || strings.HasPrefix(raw.Name, "pim") || strings.HasPrefix(raw.Name, "sit") {
				continue
			}

			currentIfaces[raw.Name] = true
			currCounters := raw.Counters

			var deltas InterfaceDeltaCounters
			b, hasBaseline := c.baselines[raw.Name]
			if !hasBaseline || currCounters.RxBytes < b.rxBytes || currCounters.TxBytes < b.txBytes ||
				currCounters.RxPackets < b.rxPackets || currCounters.TxPackets < b.txPackets {
				deltas = InterfaceDeltaCounters{
					RxBytes:   0,
					RxPackets: 0,
					TxBytes:   0,
					TxPackets: 0,
				}
				if isPeriodic {
					c.baselines[raw.Name] = counterBaseline{
						rxBytes:   currCounters.RxBytes,
						txBytes:   currCounters.TxBytes,
						rxPackets: currCounters.RxPackets,
						txPackets: currCounters.TxPackets,
						sampledAt: now,
					}
				}
			} else {
				deltas = InterfaceDeltaCounters{
					RxBytes:   currCounters.RxBytes - b.rxBytes,
					RxPackets: currCounters.RxPackets - b.rxPackets,
					TxBytes:   currCounters.TxBytes - b.txBytes,
					TxPackets: currCounters.TxPackets - b.txPackets,
				}
				if isPeriodic {
					c.baselines[raw.Name] = counterBaseline{
						rxBytes:   currCounters.RxBytes,
						txBytes:   currCounters.TxBytes,
						rxPackets: currCounters.RxPackets,
						txPackets: currCounters.TxPackets,
						sampledAt: now,
					}
				}
			}

			ifaceState := InterfaceState{
				Name:          raw.Name,
				Uptime:        raw.Uptime,
				IPv6Addresses: raw.IPv6Addresses,
				Counters:      currCounters,
				DeltaCounters: deltas,
				Clients:       clientsByDev[raw.Name],
			}
			if len(raw.IPv4Addresses) > 0 || raw.DHCPServer != "" || raw.LeaseTime > 0 {
				ifaceState.IPv4 = &InterfaceIPv4{
					Addresses:  raw.IPv4Addresses,
					DHCPServer: raw.DHCPServer,
					LeaseTime:  raw.LeaseTime,
				}
			}
			interfaces = append(interfaces, ifaceState)
		}
	}

	if isPeriodic {
		// Prune disappeared interfaces
		for name := range c.baselines {
			if !currentIfaces[name] {
				delete(c.baselines, name)
			}
		}
	}
	c.mu.Unlock()

	linkStateReport := LinkStateReport{
		Upstream:   make(map[string]PortLinkState),
		Downstream: make(map[string]PortLinkState),
	}
	if portRoles != nil {
		for port, role := range portRoles {
			if !portKeyPattern.MatchString(port) {
				c.logWarn("omitting port from link-state: does not match physical interface naming convention", "port", port)
				continue
			}
			ls, err := c.reader.ReadLinkState(ctx, port)
			if err != nil {
				c.logWarn("failed to read link state", "port", port, "error", err)
			}
			switch strings.ToLower(role) {
			case "upstream":
				linkStateReport.Upstream[port] = ls
			case "downstream":
				linkStateReport.Downstream[port] = ls
			default:
				c.logWarn("omitting port from link-state: unknown role", "port", port, "role", role)
			}
		}
	}

	lldpReport := LLDPPeersReport{
		Upstream:   make(map[string]interface{}),
		Downstream: make(map[string]interface{}),
	}
	rawLLDP, err := c.reader.ReadLLDPPeers(ctx)
	if err == nil && len(rawLLDP) > 0 && portRoles != nil {
		c.populateLLDP(rawLLDP, portRoles, &lldpReport)
	}

	payload := StatePayload{
		Version:    1,
		Unit:       unitState,
		Interfaces: interfaces,
		LinkState:  linkStateReport,
		LLDPPeers:  lldpReport,
		Radios:     make([]interface{}, 0), // wired router profile: empty array []
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return agentcore.StateEnvelope{}, fmt.Errorf("marshal state payload: %w", err)
	}

	return agentcore.StateEnvelope{
		Version:   "1.0",
		Target:    c.target,
		UUID:      appliedUUID,
		Timestamp: now,
		Payload:   json.RawMessage(payloadBytes),
	}, nil
}

func (c *Collector) processNeighbors(raw []NeighborRaw) map[string][]InterfaceClient {
	type clientAgg struct {
		mac       string
		ipv4Addrs map[string]bool
		ipv6Addrs map[string]bool
		ports     map[string]bool
		lastSeen  *uint32
	}

	clientsByDevAndMAC := make(map[string]map[string]*clientAgg)

	for _, n := range raw {
		if isIgnoredNeighborState(n.State) {
			continue
		}
		if n.LLAdr == "" || n.Dev == "" {
			continue
		}

		normMAC := strings.ToLower(n.LLAdr)
		dev := n.Dev

		if clientsByDevAndMAC[dev] == nil {
			clientsByDevAndMAC[dev] = make(map[string]*clientAgg)
		}
		agg := clientsByDevAndMAC[dev][normMAC]
		if agg == nil {
			agg = &clientAgg{
				mac:       normMAC,
				ipv4Addrs: make(map[string]bool),
				ipv6Addrs: make(map[string]bool),
				ports:     map[string]bool{dev: true},
				lastSeen:  nil, // omit when activity age is unmeasurable
			}
			clientsByDevAndMAC[dev][normMAC] = agg
		}

		if strings.Contains(n.DST, ":") {
			agg.ipv6Addrs[n.DST] = true
		} else if strings.Contains(n.DST, ".") {
			agg.ipv4Addrs[n.DST] = true
		}
	}

	result := make(map[string][]InterfaceClient)
	for dev, macMap := range clientsByDevAndMAC {
		for _, agg := range macMap {
			var ipv4List []string
			for ip := range agg.ipv4Addrs {
				ipv4List = append(ipv4List, ip)
			}
			var ipv6List []string
			for ip := range agg.ipv6Addrs {
				ipv6List = append(ipv6List, ip)
			}
			var portList []string
			for p := range agg.ports {
				portList = append(portList, p)
			}

			result[dev] = append(result[dev], InterfaceClient{
				MAC:           agg.mac,
				IPv4Addresses: ipv4List,
				IPv6Addresses: ipv6List,
				Ports:         portList,
				LastSeen:      agg.lastSeen,
			})
		}
	}
	return result
}

func isIgnoredNeighborState(states []string) bool {
	for _, s := range states {
		upper := strings.ToUpper(s)
		if upper == "FAILED" || upper == "INCOMPLETE" || upper == "NOARP" {
			return true
		}
	}
	return false
}

func (c *Collector) populateLLDP(raw map[string]interface{}, portRoles map[string]string, report *LLDPPeersReport) {
	for port, role := range portRoles {
		if !portKeyPattern.MatchString(port) {
			continue
		}
		if peerData, ok := raw[port]; ok {
			switch strings.ToLower(role) {
			case "upstream":
				report.Upstream[port] = peerData
			case "downstream":
				report.Downstream[port] = peerData
			}
		}
	}
}

func (c *Collector) logWarn(msg string, kv ...any) {
	if c.logger != nil {
		c.logger.Warn(msg, kv...)
	}
}
