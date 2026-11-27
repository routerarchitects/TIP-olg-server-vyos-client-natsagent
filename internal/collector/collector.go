package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/state"
)

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
	portRoles map[string]string
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
		portRoles:          make(map[string]string),
	}, nil
}

// HandleStateGet implements agentcore.StateGetHandler for on-demand live state queries.
func (c *Collector) HandleStateGet(ctx context.Context) (agentcore.StateEnvelope, error) {
	return c.Collect(ctx, false)
}

// Collect gathers a full telemetry snapshot. When isPeriodic is false, delta baselines are not mutated.
func (c *Collector) Collect(ctx context.Context, isPeriodic bool) (agentcore.StateEnvelope, error) {
	if ctx == nil {
		return agentcore.StateEnvelope{}, fmt.Errorf("collector context is nil")
	}
	if err := ctx.Err(); err != nil {
		return agentcore.StateEnvelope{}, err
	}

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
			c.logWarn("topology state is degraded; omitting physical port roles and applied uuid", "target", c.target)
			appliedUUID = ""
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

	cpuLoad, err := c.reader.ReadCPULoad(isPeriodic)
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
		return agentcore.StateEnvelope{}, fmt.Errorf("read interfaces: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return agentcore.StateEnvelope{}, err
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
		// Group physical ports by role using PhysicalPortRoles, validating interface existence
		roleToPorts := make(map[string][]string)
		for port, role := range portRoles {
			if _, exists := rawMap[port]; !exists {
				c.logWarn("omitting port from telemetry: interface does not exist", "port", port)
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

			// Discover all associated devices (physical ports, master bridges, and VLAN subinterfaces)
			orderedDevs := discoverAssociatedDevs(ports, rawMap)

			// Aggregate counters strictly from the physical wire ports (never double-count VLAN/bridge counters)
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

			// Aggregate IP addresses, DHCP metadata, and uptime across physical ports, bridges, and VLANs
			ipv4Addrs, ipv6Addrs, dhcpServer, leaseTime, maxUptime := collectInterfaceAddressing(ports, orderedDevs, rawMap)

			// Collect and merge clients by MAC across all associated devices (ports + bridges + VLANs)
			clients := mergeAssociatedClients(orderedDevs, clientsByDev)

			// Baseline and delta calculation: calculated per physical wire port before aggregation
			var deltas InterfaceDeltaCounters
			for _, p := range ports {
				raw, ok := rawMap[p]
				if !ok {
					continue
				}
				currentIfaces[p] = true

				prevRole, hadRole := c.portRoles[p]
				roleChanged := !hadRole || prevRole != role
				b, hasBaseline := c.baselines[p]
				pCounters := raw.Counters

				var pDelta InterfaceDeltaCounters
				if roleChanged || !hasBaseline ||
					pCounters.RxBytes < b.rxBytes || pCounters.TxBytes < b.txBytes ||
					pCounters.RxPackets < b.rxPackets || pCounters.TxPackets < b.txPackets {
					// Counter reset, new port, or port membership/role changed -> 0 delta, establish baseline
					pDelta = InterfaceDeltaCounters{
						RxBytes:   0,
						RxPackets: 0,
						TxBytes:   0,
						TxPackets: 0,
					}
					if isPeriodic {
						c.baselines[p] = counterBaseline{
							rxBytes:   pCounters.RxBytes,
							txBytes:   pCounters.TxBytes,
							rxPackets: pCounters.RxPackets,
							txPackets: pCounters.TxPackets,
							sampledAt: now,
						}
						c.portRoles[p] = role
					}
				} else {
					pDelta = InterfaceDeltaCounters{
						RxBytes:   pCounters.RxBytes - b.rxBytes,
						RxPackets: pCounters.RxPackets - b.rxPackets,
						TxBytes:   pCounters.TxBytes - b.txBytes,
						TxPackets: pCounters.TxPackets - b.txPackets,
					}
					if isPeriodic {
						c.baselines[p] = counterBaseline{
							rxBytes:   pCounters.RxBytes,
							txBytes:   pCounters.TxBytes,
							rxPackets: pCounters.RxPackets,
							txPackets: pCounters.TxPackets,
							sampledAt: now,
						}
						c.portRoles[p] = role
					}
				}

				deltas.RxBytes += pDelta.RxBytes
				deltas.RxPackets += pDelta.RxPackets
				deltas.TxBytes += pDelta.TxBytes
				deltas.TxPackets += pDelta.TxPackets
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
				delete(c.portRoles, name)
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
			if _, exists := rawMap[port]; !exists {
				c.logWarn("omitting port from link-state: interface does not exist", "port", port)
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
		c.populateLLDP(rawLLDP, portRoles, rawMap, &lldpReport)
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

func (c *Collector) populateLLDP(raw map[string]interface{}, portRoles map[string]string, rawMap map[string]InterfaceRaw, report *LLDPPeersReport) {
	for port, role := range portRoles {
		if _, exists := rawMap[port]; !exists {
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

// discoverAssociatedDevs performs a breadth-first traversal starting from the physical ports
// to collect master bridges and any stacked VLAN/upper subinterfaces.
func discoverAssociatedDevs(ports []string, rawMap map[string]InterfaceRaw) []string {
	seen := make(map[string]bool)
	queue := make([]string, 0, len(ports))

	enqueue := func(dev string) {
		if dev == "" || dev == "." || dev == "/" || seen[dev] {
			return
		}
		seen[dev] = true
		queue = append(queue, dev)
	}

	for _, p := range ports {
		enqueue(p)
	}

	for i := 0; i < len(queue); i++ {
		dev := queue[i]

		// 1. Check master bridge via raw metadata or sysfs
		if raw, ok := rawMap[dev]; ok && raw.Master != "" {
			enqueue(raw.Master)
		} else if link, err := os.Readlink(fmt.Sprintf("/sys/class/net/%s/master", dev)); err == nil {
			enqueue(filepath.Base(link))
		}

		// 2. Check upper interfaces (e.g. VLANs, bridges) via sysfs upper_* links
		if matches, err := filepath.Glob(fmt.Sprintf("/sys/class/net/%s/upper_*", dev)); err == nil {
			for _, m := range matches {
				upper := strings.TrimPrefix(filepath.Base(m), "upper_")
				enqueue(upper)
			}
		}

		// 3. Check rawMap for child interfaces referencing this device via Link or "<dev>.<vid>" naming
		prefix := dev + "."
		for name, raw := range rawMap {
			if raw.Link == dev || strings.HasPrefix(name, prefix) {
				enqueue(name)
			}
		}
	}

	devs := make([]string, 0, len(seen))
	for d := range seen {
		devs = append(devs, d)
	}
	sort.Strings(devs)
	return devs
}

// collectInterfaceAddressing aggregates deduplicated IPv4/IPv6 addresses, DHCP metadata,
// and maximum uptime across physical ports and their associated bridges/VLANs.
func collectInterfaceAddressing(ports, orderedDevs []string, rawMap map[string]InterfaceRaw) (
	ipv4Addrs []string,
	ipv6Addrs []InterfaceIPv6Address,
	dhcpServer string,
	leaseTime uint32,
	maxUptime uint64,
) {
	seenV4 := make(map[string]bool)
	seenV6 := make(map[string]bool)

	processDev := func(d string) {
		raw, ok := rawMap[d]
		if !ok {
			return
		}
		if raw.Uptime > maxUptime {
			maxUptime = raw.Uptime
		}
		if raw.DHCPServer != "" && dhcpServer == "" {
			dhcpServer = raw.DHCPServer
		}
		if raw.LeaseTime > 0 && leaseTime == 0 {
			leaseTime = raw.LeaseTime
		}
		for _, ip := range raw.IPv4Addresses {
			if !seenV4[ip] {
				seenV4[ip] = true
				ipv4Addrs = append(ipv4Addrs, ip)
			}
		}
		for _, ip6 := range raw.IPv6Addresses {
			if !seenV6[ip6.Address] {
				seenV6[ip6.Address] = true
				ipv6Addrs = append(ipv6Addrs, ip6)
			}
		}
	}

	// Prioritize physical ports first, then associated bridges/VLANs
	portSet := make(map[string]bool, len(ports))
	for _, p := range ports {
		portSet[p] = true
		processDev(p)
	}
	for _, d := range orderedDevs {
		if !portSet[d] {
			processDev(d)
		}
	}

	return ipv4Addrs, ipv6Addrs, dhcpServer, leaseTime, maxUptime
}

// mergeAssociatedClients collects neighbors across all associated devices and merges
// records sharing the same MAC address rather than dropping subsequent entries.
func mergeAssociatedClients(orderedDevs []string, clientsByDev map[string][]InterfaceClient) []InterfaceClient {
	type mergedClient struct {
		mac      string
		ipv4     map[string]bool
		ipv6     map[string]bool
		ports    map[string]bool
		lastSeen *uint32
	}

	byMAC := make(map[string]*mergedClient)
	for _, d := range orderedDevs {
		for _, cl := range clientsByDev[d] {
			macLower := strings.ToLower(cl.MAC)
			entry, exists := byMAC[macLower]
			if !exists {
				entry = &mergedClient{
					mac:   macLower,
					ipv4:  make(map[string]bool),
					ipv6:  make(map[string]bool),
					ports: make(map[string]bool),
				}
				byMAC[macLower] = entry
			}
			for _, ip := range cl.IPv4Addresses {
				entry.ipv4[ip] = true
			}
			for _, ip := range cl.IPv6Addresses {
				entry.ipv6[ip] = true
			}
			for _, p := range cl.Ports {
				entry.ports[p] = true
			}
			if cl.LastSeen != nil {
				if entry.lastSeen == nil || *cl.LastSeen < *entry.lastSeen {
					val := *cl.LastSeen
					entry.lastSeen = &val
				}
			}
		}
	}

	if len(byMAC) == 0 {
		return nil
	}

	macs := make([]string, 0, len(byMAC))
	for m := range byMAC {
		macs = append(macs, m)
	}
	sort.Strings(macs)

	clients := make([]InterfaceClient, 0, len(macs))
	for _, m := range macs {
		entry := byMAC[m]
		var v4List, v6List, portList []string
		for ip := range entry.ipv4 {
			v4List = append(v4List, ip)
		}
		sort.Strings(v4List)
		for ip := range entry.ipv6 {
			v6List = append(v6List, ip)
		}
		sort.Strings(v6List)
		for p := range entry.ports {
			portList = append(portList, p)
		}
		sort.Strings(portList)

		clients = append(clients, InterfaceClient{
			MAC:           entry.mac,
			IPv4Addresses: v4List,
			IPv6Addresses: v6List,
			Ports:         portList,
			LastSeen:      entry.lastSeen,
		})
	}
	return clients
}
