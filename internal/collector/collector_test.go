package collector

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/state"
)

type mockSystemReader struct {
	loadAvg    [3]float64
	cpuLoad    []int
	memInfo    MemoryState
	uptime     uint64
	interfaces []InterfaceRaw
	neighbors  []NeighborRaw
	linkStates map[string]PortLinkState
	lldpPeers  map[string]interface{}
}

func (m *mockSystemReader) ReadLoadAvg() ([3]float64, error) {
	return m.loadAvg, nil
}

func (m *mockSystemReader) ReadCPULoad() ([]int, error) {
	return m.cpuLoad, nil
}

func (m *mockSystemReader) ReadMemInfo() (MemoryState, error) {
	return m.memInfo, nil
}

func (m *mockSystemReader) ReadUptime() (uint64, error) {
	return m.uptime, nil
}

func (m *mockSystemReader) ReadInterfaces(ctx context.Context) ([]InterfaceRaw, error) {
	return m.interfaces, nil
}

func (m *mockSystemReader) ReadNeighbors(ctx context.Context) ([]NeighborRaw, error) {
	return m.neighbors, nil
}

func (m *mockSystemReader) ReadLinkState(ctx context.Context, iface string) (PortLinkState, error) {
	if m.linkStates != nil {
		if ls, ok := m.linkStates[iface]; ok {
			return ls, nil
		}
	}
	return PortLinkState{}, nil
}

func (m *mockSystemReader) ReadLLDPPeers(ctx context.Context) (map[string]interface{}, error) {
	if m.lldpPeers != nil {
		return m.lldpPeers, nil
	}
	return make(map[string]interface{}), nil
}

type mockStateStore struct {
	st  state.State
	err error
}

func (m *mockStateStore) Load(ctx context.Context) (state.State, error) {
	return m.st, m.err
}

func TestDelta_InitialBaseline(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{
				Name: "eth0",
				Counters: InterfaceCounters{
					RxBytes:   1000,
					TxBytes:   2000,
					RxPackets: 10,
					TxPackets: 20,
				},
			},
		},
	}
	store := &mockStateStore{st: state.State{AppliedUUID: "100"}}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	var payload StatePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if len(payload.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(payload.Interfaces))
	}

	deltas := payload.Interfaces[0].DeltaCounters
	if deltas.RxBytes != 0 || deltas.TxBytes != 0 || deltas.RxPackets != 0 || deltas.TxPackets != 0 {
		t.Fatalf("expected 0 deltas on initial baseline, got %+v", deltas)
	}
}

func TestDelta_MonotonicIncrements(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{
				Name: "eth0",
				Counters: InterfaceCounters{
					RxBytes:   1000,
					TxBytes:   2000,
					RxPackets: 10,
					TxPackets: 20,
				},
			},
		},
	}
	store := &mockStateStore{st: state.State{AppliedUUID: "100"}}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	// First periodic sample establishes baseline
	_, _ = c.Collect(context.Background(), true)

	// Second sample with incremented counters
	reader.interfaces = []InterfaceRaw{
		{
			Name: "eth0",
			Counters: InterfaceCounters{
				RxBytes:   1500,
				TxBytes:   2300,
				RxPackets: 15,
				TxPackets: 25,
			},
		},
	}

	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	var payload StatePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	deltas := payload.Interfaces[0].DeltaCounters
	if deltas.RxBytes != 500 || deltas.TxBytes != 300 || deltas.RxPackets != 5 || deltas.TxPackets != 5 {
		t.Fatalf("expected 500/300/5/5 deltas, got %+v", deltas)
	}
}

func TestDelta_CounterReset(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{
				Name: "eth0",
				Counters: InterfaceCounters{
					RxBytes:   5000,
					TxBytes:   5000,
					RxPackets: 50,
					TxPackets: 50,
				},
			},
		},
	}
	store := &mockStateStore{st: state.State{AppliedUUID: "100"}}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	// Baseline established at 5000
	_, _ = c.Collect(context.Background(), true)

	// Counter resets to 100 (e.g. interface restart down/up)
	reader.interfaces = []InterfaceRaw{
		{
			Name: "eth0",
			Counters: InterfaceCounters{
				RxBytes:   100,
				TxBytes:   100,
				RxPackets: 2,
				TxPackets: 2,
			},
		},
	}

	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	var payload StatePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	deltas := payload.Interfaces[0].DeltaCounters
	if deltas.RxBytes != 0 || deltas.TxBytes != 0 || deltas.RxPackets != 0 || deltas.TxPackets != 0 {
		t.Fatalf("expected 0 deltas on reset without underflow, got %+v", deltas)
	}

	// Subsequent increment calculates from the new baseline (100)
	reader.interfaces = []InterfaceRaw{
		{
			Name: "eth0",
			Counters: InterfaceCounters{
				RxBytes:   150,
				TxBytes:   130,
				RxPackets: 5,
				TxPackets: 4,
			},
		},
	}
	env2, _ := c.Collect(context.Background(), true)
	var payload2 StatePayload
	_ = json.Unmarshal(env2.Payload, &payload2)
	deltas2 := payload2.Interfaces[0].DeltaCounters
	if deltas2.RxBytes != 50 || deltas2.TxBytes != 30 || deltas2.RxPackets != 3 || deltas2.TxPackets != 2 {
		t.Fatalf("expected 50/30/3/2 deltas from new baseline, got %+v", deltas2)
	}
}

func TestDelta_OnDemandIsolation(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{
				Name: "eth0",
				Counters: InterfaceCounters{
					RxBytes:   1000,
					TxBytes:   1000,
					RxPackets: 10,
					TxPackets: 10,
				},
			},
		},
	}
	store := &mockStateStore{st: state.State{AppliedUUID: "100"}}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	// 1. Scheduled periodic sample establishes baseline at 1000
	_, _ = c.Collect(context.Background(), true)

	// 2. On-demand query arrives halfway with counters at 1200
	reader.interfaces[0].Counters.RxBytes = 1200
	reader.interfaces[0].Counters.TxBytes = 1200
	onDemandEnv, err := c.Collect(context.Background(), false)
	if err != nil {
		t.Fatalf("on-demand collect: %v", err)
	}
	var onDemandPayload StatePayload
	_ = json.Unmarshal(onDemandEnv.Payload, &onDemandPayload)
	if onDemandPayload.Interfaces[0].DeltaCounters.RxBytes != 200 {
		t.Fatalf("expected transient delta 200, got %d", onDemandPayload.Interfaces[0].DeltaCounters.RxBytes)
	}

	// 3. Next scheduled periodic sample arrives with counters at 1500
	reader.interfaces[0].Counters.RxBytes = 1500
	reader.interfaces[0].Counters.TxBytes = 1500
	periodicEnv, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("periodic collect: %v", err)
	}
	var periodicPayload StatePayload
	_ = json.Unmarshal(periodicEnv.Payload, &periodicPayload)
	// Must measure full interval from baseline (1000 -> 1500 = 500), not distorted by on-demand query
	if periodicPayload.Interfaces[0].DeltaCounters.RxBytes != 500 {
		t.Fatalf("expected periodic delta 500 from uninterrupted baseline, got %d", periodicPayload.Interfaces[0].DeltaCounters.RxBytes)
	}
}

func TestNeighbor_FilteringAndDeduplication(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{Name: "eth0"},
		},
		neighbors: []NeighborRaw{
			{DST: "172.16.0.1", Dev: "eth0", LLAdr: "AA:BB:CC:DD:EE:FF", State: []string{"REACHABLE"}},
			{DST: "fe80::1", Dev: "eth0", LLAdr: "aa:bb:cc:dd:ee:ff", State: []string{"STALE"}},
			{DST: "172.16.0.99", Dev: "eth0", LLAdr: "11:22:33:44:55:66", State: []string{"FAILED"}},
			{DST: "172.16.0.100", Dev: "eth0", LLAdr: "22:33:44:55:66:77", State: []string{"INCOMPLETE"}},
			{DST: "172.16.0.101", Dev: "eth0", LLAdr: "33:44:55:66:77:88", State: []string{"NOARP"}},
		},
	}
	store := &mockStateStore{st: state.State{AppliedUUID: "100"}}
	c, _ := New(Config{Target: "vyos", StateStore: store, Reader: reader})

	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	var payload StatePayload
	_ = json.Unmarshal(env.Payload, &payload)

	clients := payload.Interfaces[0].Clients
	if len(clients) != 1 {
		t.Fatalf("expected exactly 1 deduplicated client, got %d", len(clients))
	}

	client := clients[0]
	if client.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("expected normalized mac aa:bb:cc:dd:ee:ff, got %s", client.MAC)
	}
	if len(client.IPv4Addresses) != 1 || client.IPv4Addresses[0] != "172.16.0.1" {
		t.Fatalf("expected ipv4 address 172.16.0.1, got %+v", client.IPv4Addresses)
	}
	if len(client.IPv6Addresses) != 1 || client.IPv6Addresses[0] != "fe80::1" {
		t.Fatalf("expected ipv6 address fe80::1, got %+v", client.IPv6Addresses)
	}

	// Verify last_seen omission in serialized JSON
	jsonBytes, _ := json.Marshal(client)
	if strings.Contains(string(jsonBytes), "last_seen") {
		t.Fatalf("expected last_seen to be omitted when unknown, got %s", string(jsonBytes))
	}
}

func TestPortMap_DefaultResolutionAndOmission(t *testing.T) {
	reader := &mockSystemReader{
		linkStates: map[string]PortLinkState{
			"eth0": {Carrier: true, Speed: 1000, Duplex: "full"},
			"eth1": {Carrier: false, Speed: 0},
			"eth2": {Carrier: true, Speed: 100, Duplex: "half"},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "100",
			PhysicalPortRoles: map[string]string{
				"eth0":    "upstream",
				"eth1":    "downstream",
				"eth2":    "downstream",
				"br0":     "downstream", // unsupported bridge name
				"eth1.10": "downstream", // unsupported vlan subinterface name
			},
		},
	}
	c, _ := New(Config{Target: "vyos", StateStore: store, Reader: reader})

	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	var payload StatePayload
	_ = json.Unmarshal(env.Payload, &payload)

	if len(payload.LinkState.Upstream) != 1 || payload.LinkState.Upstream["eth0"].Speed != 1000 {
		t.Fatalf("expected eth0 under upstream with speed 1000, got %+v", payload.LinkState.Upstream)
	}
	if len(payload.LinkState.Downstream) != 2 {
		t.Fatalf("expected 2 downstream physical ports (eth1, eth2), got %d: %+v", len(payload.LinkState.Downstream), payload.LinkState.Downstream)
	}
	if _, ok := payload.LinkState.Downstream["br0"]; ok {
		t.Fatal("logical bridge br0 must be omitted from link-state")
	}
	if _, ok := payload.LinkState.Downstream["eth1.10"]; ok {
		t.Fatal("vlan subinterface eth1.10 must be omitted from link-state")
	}
}

func TestPortMap_DegradedTopologySuppression(t *testing.T) {
	reader := &mockSystemReader{
		linkStates: map[string]PortLinkState{
			"eth0": {Carrier: true, Speed: 1000},
			"eth1": {Carrier: true, Speed: 1000},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "100",
			PhysicalPortRoles: map[string]string{
				"eth0": "upstream",
				"eth1": "downstream",
			},
			TopologyDegraded: true,
		},
	}
	c, _ := New(Config{Target: "vyos", StateStore: store, Reader: reader})

	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	var payload StatePayload
	_ = json.Unmarshal(env.Payload, &payload)

	if len(payload.LinkState.Upstream) != 0 || len(payload.LinkState.Downstream) != 0 {
		t.Fatalf("expected link-state to be completely omitted when topology degraded, got %+v", payload.LinkState)
	}
}

func TestState_WiredOnlyRouterRadios(t *testing.T) {
	reader := &mockSystemReader{}
	store := &mockStateStore{st: state.State{AppliedUUID: "100"}}
	c, _ := New(Config{Target: "vyos", StateStore: store, Reader: reader})

	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	var payload StatePayload
	_ = json.Unmarshal(env.Payload, &payload)

	if payload.Radios == nil {
		t.Fatal("radios must be non-nil empty array []")
	}
	if len(payload.Radios) != 0 {
		t.Fatalf("radios must be empty array, got %d items", len(payload.Radios))
	}

	// Verify serialized JSON contains "radios": []
	if !strings.Contains(string(env.Payload), `"radios":[]`) {
		t.Fatalf("expected serialized JSON to contain \"radios\":[], got %s", string(env.Payload))
	}
}

func TestUUID_CheckpointHandling(t *testing.T) {
	reader := &mockSystemReader{}

	// 1. Missing checkpoint
	storeMissing := &mockStateStore{err: errors.New("file not found")}
	c1, _ := New(Config{Target: "vyos", StateStore: storeMissing, Reader: reader})
	env1, _ := c1.Collect(context.Background(), true)
	if env1.UUID != "" {
		t.Fatalf("expected empty UUID for missing store, got %q", env1.UUID)
	}

	// 2. Initial baseline config UUID = "0"
	storeZero := &mockStateStore{st: state.State{AppliedUUID: "0"}}
	c2, _ := New(Config{Target: "vyos", StateStore: storeZero, Reader: reader})
	env2, _ := c2.Collect(context.Background(), true)
	if env2.UUID != "0" {
		t.Fatalf("expected UUID 0, got %q", env2.UUID)
	}
}

func TestPortMap_InterfacesRoleMappingAndFiltering(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{Name: "lo", Counters: InterfaceCounters{RxBytes: 50, TxBytes: 50}},
			{Name: "eth0", Counters: InterfaceCounters{RxBytes: 1000, TxBytes: 2000}},
			{Name: "mgmt0", Counters: InterfaceCounters{RxBytes: 900, TxBytes: 900}},
			{Name: "eth1", Counters: InterfaceCounters{RxBytes: 3000, TxBytes: 4000}},
			{Name: "br0", IPv4Addresses: []string{"172.16.1.10/24"}, Counters: InterfaceCounters{RxBytes: 1000, TxBytes: 2000}},
			{Name: "br1", IPv4Addresses: []string{"192.168.100.1/24"}, Counters: InterfaceCounters{RxBytes: 3000, TxBytes: 4000}},
			{Name: "pim6reg", Counters: InterfaceCounters{RxBytes: 0, TxBytes: 0}},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "100",
			PhysicalPortRoles: map[string]string{
				"eth0": "upstream",
				"eth1": "downstream",
			},
		},
	}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	var payload StatePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if len(payload.Interfaces) != 2 {
		t.Fatalf("expected exactly 2 interfaces (upstream & downstream), got %d: %+v", len(payload.Interfaces), payload.Interfaces)
	}

	if payload.Interfaces[0].Name != "upstream" {
		t.Fatalf("expected first interface to be 'upstream', got %q", payload.Interfaces[0].Name)
	}
	if payload.Interfaces[0].Counters.RxBytes != 1000 || payload.Interfaces[0].Counters.TxBytes != 2000 {
		t.Fatalf("expected upstream counters from eth0 (1000/2000), got %+v", payload.Interfaces[0].Counters)
	}

	if payload.Interfaces[1].Name != "downstream" {
		t.Fatalf("expected second interface to be 'downstream', got %q", payload.Interfaces[1].Name)
	}
	if payload.Interfaces[1].Counters.RxBytes != 3000 || payload.Interfaces[1].Counters.TxBytes != 4000 {
		t.Fatalf("expected downstream counters from eth1 (3000/4000), got %+v", payload.Interfaces[1].Counters)
	}

	// Verify lo, mgmt0, br0, br1, pim6reg are completely absent
	for _, iface := range payload.Interfaces {
		if iface.Name == "lo" || iface.Name == "mgmt0" || iface.Name == "pim6reg" || iface.Name == "br0" || iface.Name == "br1" {
			t.Fatalf("internal/raw interface %q must not be emitted in interfaces[]", iface.Name)
		}
	}
}
