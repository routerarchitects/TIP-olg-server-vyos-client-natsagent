package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// InterfaceRaw represents raw interface configuration and counters collected from kernel/tools.
type InterfaceRaw struct {
	Name          string
	Master        string
	Link          string
	IPv4Addresses []string
	DHCPServer    string
	LeaseTime     uint32
	IPv6Addresses []InterfaceIPv6Address
	Uptime        uint64
	Counters      InterfaceCounters
}

// NeighborRaw represents an entry parsed from `ip -j neigh`.
type NeighborRaw struct {
	DST   string   `json:"dst"`
	Dev   string   `json:"dev"`
	LLAdr string   `json:"lladdr"`
	State []string `json:"state"`
}

// SystemReader defines the abstraction for gathering host telemetry.
type SystemReader interface {
	ReadLoadAvg() ([3]float64, error)
	ReadCPULoad(updateBaseline bool) ([]int, error)
	ReadMemInfo() (MemoryState, error)
	ReadUptime() (uint64, error)
	ReadInterfaces(ctx context.Context) ([]InterfaceRaw, error)
	ReadNeighbors(ctx context.Context) ([]NeighborRaw, error)
	ReadLinkState(ctx context.Context, iface string) (PortLinkState, error)
	ReadLLDPPeers(ctx context.Context) (map[string]interface{}, error)
}

// LinuxSystemReader implements SystemReader on a Linux/VyOS host.
type LinuxSystemReader struct {
	mu          sync.Mutex
	prevCPULoad []cpuSample
}

type cpuSample struct {
	total uint64
	idle  uint64
}

// NewLinuxSystemReader creates a new Linux telemetry reader.
func NewLinuxSystemReader() *LinuxSystemReader {
	return &LinuxSystemReader{}
}

func (r *LinuxSystemReader) ReadLoadAvg() ([3]float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return [3]float64{}, fmt.Errorf("read /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return [3]float64{}, fmt.Errorf("invalid /proc/loadavg format: %q", string(data))
	}
	var load [3]float64
	for i := 0; i < 3; i++ {
		val, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return [3]float64{}, fmt.Errorf("parse /proc/loadavg[%d]: %w", i, err)
		}
		load[i] = val
	}
	return load, nil
}

func (r *LinuxSystemReader) ReadCPULoad(updateBaseline bool) ([]int, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil, fmt.Errorf("read /proc/stat: %w", err)
	}
	defer file.Close()

	var currentSamples []cpuSample
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		var total, idle uint64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
			if i == 4 || i == 5 { // idle and iowait
				idle += v
			}
		}
		currentSamples = append(currentSamples, cpuSample{total: total, idle: idle})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan /proc/stat: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	loads := make([]int, len(currentSamples))
	if len(r.prevCPULoad) == len(currentSamples) {
		for i := range currentSamples {
			deltaTotal := currentSamples[i].total - r.prevCPULoad[i].total
			deltaIdle := currentSamples[i].idle - r.prevCPULoad[i].idle
			if deltaTotal > 0 && deltaTotal >= deltaIdle {
				loads[i] = int((deltaTotal - deltaIdle) * 100 / deltaTotal)
			}
		}
	}
	if updateBaseline {
		r.prevCPULoad = currentSamples
	}
	return loads, nil
}

func (r *LinuxSystemReader) ReadMemInfo() (MemoryState, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemoryState{}, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	defer file.Close()

	var state MemoryState
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) == 0 {
			continue
		}
		valKB, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		valBytes := valKB * 1024

		switch key {
		case "MemTotal":
			state.Total = valBytes
		case "MemFree":
			state.Free = valBytes
		case "Cached":
			state.Cached = valBytes
		case "Buffers":
			state.Buffered = valBytes
		}
	}
	return state, nil
}

func (r *LinuxSystemReader) ReadUptime() (uint64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, fmt.Errorf("read /proc/uptime: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty /proc/uptime")
	}
	secFloat, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/uptime: %w", err)
	}
	return uint64(secFloat), nil
}

type ipAddrJSONEntry struct {
	IfName   string `json:"ifname"`
	Master   string `json:"master"`
	Link     string `json:"link"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
		ValidLife uint32 `json:"valid_life_time"`
	} `json:"addr_info"`
}

func (r *LinuxSystemReader) ReadInterfaces(ctx context.Context) ([]InterfaceRaw, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "ip", "-j", "addr")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("exec ip -j addr: %w", err)
	}

	var parsed []ipAddrJSONEntry
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal ip -j addr: %w", err)
	}

	countersMap, err := readProcNetDev()
	if err != nil {
		return nil, fmt.Errorf("read /proc/net/dev: %w", err)
	}

	result := make([]InterfaceRaw, 0, len(parsed))
	for _, entry := range parsed {
		if entry.IfName == "" {
			continue
		}
		raw := InterfaceRaw{
			Name:     entry.IfName,
			Master:   entry.Master,
			Link:     entry.Link,
			Counters: countersMap[entry.IfName],
		}
		for _, addr := range entry.AddrInfo {
			if addr.Family == "inet" {
				cidr := fmt.Sprintf("%s/%d", addr.Local, addr.PrefixLen)
				raw.IPv4Addresses = append(raw.IPv4Addresses, cidr)
			} else if addr.Family == "inet6" {
				cidr := fmt.Sprintf("%s/%d", addr.Local, addr.PrefixLen)
				raw.IPv6Addresses = append(raw.IPv6Addresses, InterfaceIPv6Address{
					Address: cidr,
					Valid:   addr.ValidLife,
				})
			}
		}
		result = append(result, raw)
	}
	return result, nil
}

func readProcNetDev() (map[string]InterfaceCounters, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	res := make(map[string]InterfaceCounters)
	scanner := bufio.NewScanner(file)
	lineIdx := 0
	for scanner.Scan() {
		lineIdx++
		if lineIdx <= 2 {
			continue // skip header lines
		}
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}
		var c InterfaceCounters
		c.RxBytes, _ = strconv.ParseUint(fields[0], 10, 64)
		c.RxPackets, _ = strconv.ParseUint(fields[1], 10, 64)
		c.RxErrors, _ = strconv.ParseUint(fields[2], 10, 64)
		c.RxDropped, _ = strconv.ParseUint(fields[3], 10, 64)
		c.Multicast, _ = strconv.ParseUint(fields[7], 10, 64)
		c.TxBytes, _ = strconv.ParseUint(fields[8], 10, 64)
		c.TxPackets, _ = strconv.ParseUint(fields[9], 10, 64)
		c.TxErrors, _ = strconv.ParseUint(fields[10], 10, 64)
		c.TxDropped, _ = strconv.ParseUint(fields[11], 10, 64)
		c.Collisions, _ = strconv.ParseUint(fields[13], 10, 64)
		res[iface] = c
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *LinuxSystemReader) ReadNeighbors(ctx context.Context) ([]NeighborRaw, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "ip", "-j", "neigh")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("exec ip -j neigh: %w", err)
	}

	var neighbors []NeighborRaw
	if err := json.Unmarshal(stdout.Bytes(), &neighbors); err != nil {
		return nil, fmt.Errorf("unmarshal ip -j neigh: %w", err)
	}
	return neighbors, nil
}

func (r *LinuxSystemReader) ReadLinkState(ctx context.Context, iface string) (PortLinkState, error) {
	basePath := "/sys/class/net/" + iface

	carrierBytes, err := os.ReadFile(basePath + "/carrier")
	carrier := false
	if err == nil && strings.TrimSpace(string(carrierBytes)) == "1" {
		carrier = true
	}

	var speed uint32
	speedBytes, err := os.ReadFile(basePath + "/speed")
	if err == nil {
		val, err := strconv.ParseInt(strings.TrimSpace(string(speedBytes)), 10, 64)
		if err == nil && val > 0 {
			speed = uint32(val)
		}
	}

	var duplex string
	duplexBytes, err := os.ReadFile(basePath + "/duplex")
	if err == nil {
		duplex = strings.TrimSpace(string(duplexBytes))
	}

	return PortLinkState{
		Carrier: carrier,
		Speed:   speed,
		Duplex:  duplex,
	}, nil
}

func (r *LinuxSystemReader) ReadLLDPPeers(ctx context.Context) (map[string]interface{}, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "lldpcli", "-f", "json", "show", "neighbors")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return make(map[string]interface{}), nil
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return make(map[string]interface{}), nil
	}
	return raw, nil
}
