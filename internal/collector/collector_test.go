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
	loadAvg       [3]float64
	cpuLoad       []int
	memInfo       MemoryState
	uptime        uint64
	interfaces    []InterfaceRaw
	interfacesErr error
	neighbors     []NeighborRaw
	linkStates    map[string]PortLinkState
	lldpPeers     map[string]interface{}
}

func (m *mockSystemReader) ReadLoadAvg() ([3]float64, error) {
	return m.loadAvg, nil
}

func (m *mockSystemReader) ReadCPULoad(updateBaseline bool) ([]int, error) {
	return m.cpuLoad, nil
}

func (m *mockSystemReader) ReadMemInfo() (MemoryState, error) {
	return m.memInfo, nil
}

func (m *mockSystemReader) ReadUptime() (uint64, error) {
	return m.uptime, nil
}

func (m *mockSystemReader) ReadInterfaces(ctx context.Context) ([]InterfaceRaw, error) {
	if m.interfacesErr != nil {
		return nil, m.interfacesErr
	}
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
		interfaces: []InterfaceRaw{
			{Name: "eth0"},
			{Name: "eth1"},
			{Name: "eth2"},
		},
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

	if env.UUID != "" {
		t.Fatalf("expected empty UUID when topology degraded, got %q", env.UUID)
	}
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
		t.Fatalf("expected exactly 2 interfaces (upv0 & downv0), got %d: %+v", len(payload.Interfaces), payload.Interfaces)
	}

	if payload.Interfaces[0].Name != "upv0" {
		t.Fatalf("expected first interface to be 'upv0', got %q", payload.Interfaces[0].Name)
	}
	if payload.Interfaces[0].Counters.RxBytes != 1000 || payload.Interfaces[0].Counters.TxBytes != 2000 {
		t.Fatalf("expected upstream counters from eth0 (1000/2000), got %+v", payload.Interfaces[0].Counters)
	}

	if payload.Interfaces[1].Name != "downv0" {
		t.Fatalf("expected second interface to be 'downv0', got %q", payload.Interfaces[1].Name)
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

func TestPortMap_DeltaPortMembershipChange(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{Name: "eth0", Counters: InterfaceCounters{RxBytes: 1000, TxBytes: 1000}},
			{Name: "eth1", Counters: InterfaceCounters{RxBytes: 50000, TxBytes: 50000}},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "100",
			PhysicalPortRoles: map[string]string{
				"eth0": "upstream",
			},
		},
	}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	// 1. Initial sample establishes baseline for eth0 as upstream
	env1, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect 1: %v", err)
	}
	var p1 StatePayload
	_ = json.Unmarshal(env1.Payload, &p1)
	if len(p1.Interfaces) != 1 || p1.Interfaces[0].DeltaCounters.RxBytes != 0 {
		t.Fatalf("expected 0 delta on initial baseline, got %+v", p1.Interfaces[0].DeltaCounters)
	}

	// 2. Reconfiguration: upstream moves from eth0 to eth1
	store.st.PhysicalPortRoles = map[string]string{
		"eth1": "upstream",
	}

	env2, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect 2: %v", err)
	}
	var p2 StatePayload
	_ = json.Unmarshal(env2.Payload, &p2)
	if len(p2.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(p2.Interfaces))
	}
	// CRITICAL CHECK: delta must be 0, NOT 49,000 (50,000 - 1,000)
	if p2.Interfaces[0].DeltaCounters.RxBytes != 0 {
		t.Fatalf("expected 0 delta on upstream port membership change, got %d (leakage from previous baseline)", p2.Interfaces[0].DeltaCounters.RxBytes)
	}

	// 3. Subsequent sample on eth1: incremental traffic from 50,000 to 50,300
	reader.interfaces = []InterfaceRaw{
		{Name: "eth0", Counters: InterfaceCounters{RxBytes: 1000, TxBytes: 1000}},
		{Name: "eth1", Counters: InterfaceCounters{RxBytes: 50300, TxBytes: 50200}},
	}
	env3, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("collect 3: %v", err)
	}
	var p3 StatePayload
	_ = json.Unmarshal(env3.Payload, &p3)
	if p3.Interfaces[0].DeltaCounters.RxBytes != 300 || p3.Interfaces[0].DeltaCounters.TxBytes != 200 {
		t.Fatalf("expected 300/200 deltas from eth1 baseline, got %+v", p3.Interfaces[0].DeltaCounters)
	}
}

func TestPortMap_MultiPortDeltaAggregationAndCounterReset(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{Name: "eth1", Counters: InterfaceCounters{RxBytes: 1000, TxBytes: 1000}},
			{Name: "eth2", Counters: InterfaceCounters{RxBytes: 2000, TxBytes: 2000}},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "100",
			PhysicalPortRoles: map[string]string{
				"eth1": "downstream",
				"eth2": "downstream",
			},
		},
	}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	// 1. Initial sample establishes baselines for both eth1 and eth2
	env1, _ := c.Collect(context.Background(), true)
	var p1 StatePayload
	_ = json.Unmarshal(env1.Payload, &p1)
	if p1.Interfaces[0].DeltaCounters.RxBytes != 0 {
		t.Fatalf("expected 0 delta on initial sample, got %+v", p1.Interfaces[0].DeltaCounters)
	}

	// 2. eth1 increases by 300 (to 1300), eth2 resets to 50 (e.g. driver reset)
	reader.interfaces = []InterfaceRaw{
		{Name: "eth1", Counters: InterfaceCounters{RxBytes: 1300, TxBytes: 1200}},
		{Name: "eth2", Counters: InterfaceCounters{RxBytes: 50, TxBytes: 50}},
	}
	env2, _ := c.Collect(context.Background(), true)
	var p2 StatePayload
	_ = json.Unmarshal(env2.Payload, &p2)

	// eth1 delta is 300/200, eth2 delta is 0 (reset). Aggregated delta must be 300/200!
	if p2.Interfaces[0].DeltaCounters.RxBytes != 300 || p2.Interfaces[0].DeltaCounters.TxBytes != 200 {
		t.Fatalf("expected 300/200 deltas preserving eth1 despite eth2 reset, got %+v", p2.Interfaces[0].DeltaCounters)
	}

	// 3. Subsequent sample: eth1 increases by 100 (to 1400), eth2 increases by 25 (to 75)
	reader.interfaces = []InterfaceRaw{
		{Name: "eth1", Counters: InterfaceCounters{RxBytes: 1400, TxBytes: 1250}},
		{Name: "eth2", Counters: InterfaceCounters{RxBytes: 75, TxBytes: 70}},
	}
	env3, _ := c.Collect(context.Background(), true)
	var p3 StatePayload
	_ = json.Unmarshal(env3.Payload, &p3)

	// Aggregated delta: eth1 (100) + eth2 (25) = 125 RX, eth1 (50) + eth2 (20) = 70 TX
	if p3.Interfaces[0].DeltaCounters.RxBytes != 125 || p3.Interfaces[0].DeltaCounters.TxBytes != 70 {
		t.Fatalf("expected 125/70 deltas from both ports, got %+v", p3.Interfaces[0].DeltaCounters)
	}
}

func TestPortMap_VLANDiscoveryAndClientMerging(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{
				Name:     "eth1",
				Master:   "br1",
				Counters: InterfaceCounters{RxBytes: 10000, TxBytes: 5000},
			},
			{
				Name:     "br1",
				Counters: InterfaceCounters{RxBytes: 10000, TxBytes: 5000},
			},
			{
				Name:          "br1.100",
				Link:          "br1",
				IPv4Addresses: []string{"192.168.100.1/24"},
				IPv6Addresses: []InterfaceIPv6Address{{Address: "fd00:100::1/64", Valid: 3600}},
				Counters:      InterfaceCounters{RxBytes: 6000, TxBytes: 3000},
			},
			{
				Name:          "br1.200",
				Link:          "br1",
				IPv4Addresses: []string{"192.168.200.1/24"},
				IPv6Addresses: []InterfaceIPv6Address{{Address: "fd00:200::1/64", Valid: 3600}},
				Counters:      InterfaceCounters{RxBytes: 4000, TxBytes: 2000},
			},
		},
		neighbors: []NeighborRaw{
			{DST: "192.168.100.10", Dev: "br1.100", LLAdr: "aa:bb:cc:dd:ee:01", State: []string{"REACHABLE"}},
			{DST: "192.168.200.10", Dev: "br1.200", LLAdr: "AA:BB:CC:DD:EE:01", State: []string{"REACHABLE"}},
			{DST: "fd00:200::10", Dev: "br1.200", LLAdr: "aa:bb:cc:dd:ee:01", State: []string{"REACHABLE"}},
			{DST: "192.168.200.20", Dev: "br1.200", LLAdr: "aa:bb:cc:dd:ee:02", State: []string{"STALE"}},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "100",
			PhysicalPortRoles: map[string]string{
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

	if len(payload.Interfaces) != 1 || payload.Interfaces[0].Name != "downv0" {
		t.Fatalf("expected single downv0 interface, got %+v", payload.Interfaces)
	}

	iface := payload.Interfaces[0]

	// 1. Counters must come strictly from physical wire port eth1 (no double-counting br1/VLANs)
	if iface.Counters.RxBytes != 10000 || iface.Counters.TxBytes != 5000 {
		t.Fatalf("expected counters 10000/5000 strictly from eth1, got %+v", iface.Counters)
	}

	// 2. IPv4 and IPv6 addresses from both br1.100 and br1.200 must be present
	if iface.IPv4 == nil || len(iface.IPv4.Addresses) != 2 ||
		iface.IPv4.Addresses[0] != "192.168.100.1/24" || iface.IPv4.Addresses[1] != "192.168.200.1/24" {
		t.Fatalf("expected both VLAN IPv4 addresses, got %+v", iface.IPv4)
	}
	if len(iface.IPv6Addresses) != 2 ||
		iface.IPv6Addresses[0].Address != "fd00:100::1/64" || iface.IPv6Addresses[1].Address != "fd00:200::1/64" {
		t.Fatalf("expected both VLAN IPv6 addresses, got %+v", iface.IPv6Addresses)
	}

	// 3. Clients must be discovered from VLANs and merged by MAC
	if len(iface.Clients) != 2 {
		t.Fatalf("expected 2 merged clients, got %d: %+v", len(iface.Clients), iface.Clients)
	}
	cl1 := iface.Clients[0]
	if cl1.MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("expected first client MAC aa:bb:cc:dd:ee:01, got %s", cl1.MAC)
	}
	if len(cl1.IPv4Addresses) != 2 || cl1.IPv4Addresses[0] != "192.168.100.10" || cl1.IPv4Addresses[1] != "192.168.200.10" {
		t.Fatalf("expected merged IPv4 addresses for client 1, got %+v", cl1.IPv4Addresses)
	}
	if len(cl1.IPv6Addresses) != 1 || cl1.IPv6Addresses[0] != "fd00:200::10" {
		t.Fatalf("expected merged IPv6 address for client 1, got %+v", cl1.IPv6Addresses)
	}
	if len(cl1.Ports) != 2 || cl1.Ports[0] != "br1.100" || cl1.Ports[1] != "br1.200" {
		t.Fatalf("expected merged ports [br1.100, br1.200] for client 1, got %+v", cl1.Ports)
	}
}

func TestPortMap_DirectPortVLANPrefixDiscovery(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{
				Name:     "eth1",
				Counters: InterfaceCounters{RxBytes: 2500, TxBytes: 1500},
			},
			{
				Name:          "eth1.10",
				IPv4Addresses: []string{"10.0.10.1/24"},
				Counters:      InterfaceCounters{RxBytes: 2500, TxBytes: 1500},
			},
		},
		neighbors: []NeighborRaw{
			{DST: "10.0.10.50", Dev: "eth1.10", LLAdr: "11:22:33:44:55:66", State: []string{"REACHABLE"}},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "101",
			PhysicalPortRoles: map[string]string{
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

	iface := payload.Interfaces[0]
	if iface.Counters.RxBytes != 2500 || iface.Counters.TxBytes != 1500 {
		t.Fatalf("expected 2500/1500 without double counting eth1.10, got %+v", iface.Counters)
	}
	if iface.IPv4 == nil || len(iface.IPv4.Addresses) != 1 || iface.IPv4.Addresses[0] != "10.0.10.1/24" {
		t.Fatalf("expected 10.0.10.1/24 from eth1.10, got %+v", iface.IPv4)
	}
	if len(iface.Clients) != 1 || iface.Clients[0].MAC != "11:22:33:44:55:66" {
		t.Fatalf("expected client from eth1.10, got %+v", iface.Clients)
	}
}

func TestCollect_ReadInterfacesFailurePreservesBaselines(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{Name: "eth0", Counters: InterfaceCounters{RxBytes: 1000, TxBytes: 2000}},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "100",
			PhysicalPortRoles: map[string]string{
				"eth0": "upstream",
			},
		},
	}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	// 1. Initial periodic sample establishes baseline at 1000/2000
	if _, err := c.Collect(context.Background(), true); err != nil {
		t.Fatalf("initial collect: %v", err)
	}

	// 2. ReadInterfaces fails: both periodic Collect and on-demand HandleStateGet must return an error
	reader.interfacesErr = errors.New("proc/net/dev unavailable")
	if _, err := c.Collect(context.Background(), true); err == nil {
		t.Fatal("expected Collect to fail when ReadInterfaces fails")
	}
	if _, err := c.HandleStateGet(context.Background()); err == nil {
		t.Fatal("expected HandleStateGet to fail when ReadInterfaces fails")
	}

	// 3. Recovery: ReadInterfaces succeeds with incremented counters (1500/2300)
	// Delta must be 500/300 from the preserved baseline, not reset to 0 or spiked
	reader.interfacesErr = nil
	reader.interfaces = []InterfaceRaw{
		{Name: "eth0", Counters: InterfaceCounters{RxBytes: 1500, TxBytes: 2300}},
	}
	env, err := c.Collect(context.Background(), true)
	if err != nil {
		t.Fatalf("recovery collect: %v", err)
	}
	var payload StatePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(payload.Interfaces))
	}
	deltas := payload.Interfaces[0].DeltaCounters
	if deltas.RxBytes != 500 || deltas.TxBytes != 300 {
		t.Fatalf("expected preserved baseline delta 500/300, got %+v", deltas)
	}
}

func TestCollect_CancelledContext(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{Name: "eth0", Counters: InterfaceCounters{RxBytes: 1000, TxBytes: 2000}},
		},
	}
	store := &mockStateStore{st: state.State{AppliedUUID: "100"}}
	c, err := New(Config{Target: "vyos", StateStore: store, Reader: reader})
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Collect(ctx, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from Collect, got %v", err)
	}
	if _, err := c.HandleStateGet(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from HandleStateGet, got %v", err)
	}
}

func TestLinuxSystemReader_CPULoadBaselineProtection(t *testing.T) {
	r := NewLinuxSystemReader()

	// Seed baseline with real CPU count from /proc/stat
	loads, err := r.ReadCPULoad(true)
	if err != nil {
		t.Skipf("skipping on non-procfs environment: %v", err)
	}
	if len(loads) == 0 {
		t.Fatal("expected at least one CPU entry")
	}

	// Set a known deterministic baseline
	for i := range r.prevCPULoad {
		r.prevCPULoad[i] = cpuSample{total: 1, idle: 1}
	}

	// On-demand sample (updateBaseline=false) must NOT overwrite prevCPULoad
	if _, err := r.ReadCPULoad(false); err != nil {
		t.Fatalf("ReadCPULoad(false): %v", err)
	}
	for i, s := range r.prevCPULoad {
		if s.total != 1 || s.idle != 1 {
			t.Fatalf("expected prevCPULoad[%d] preserved at {1,1} on non-periodic read, got %+v", i, s)
		}
	}

	// Periodic sample (updateBaseline=true) MUST update prevCPULoad
	if _, err := r.ReadCPULoad(true); err != nil {
		t.Fatalf("ReadCPULoad(true): %v", err)
	}
	if r.prevCPULoad[0].total <= 1 {
		t.Fatalf("expected prevCPULoad[0].total updated from /proc/stat, got %+v", r.prevCPULoad[0])
	}
}

func TestPortMap_CustomLinuxInterfaceNames(t *testing.T) {
	reader := &mockSystemReader{
		interfaces: []InterfaceRaw{
			{Name: "enp3s0", Counters: InterfaceCounters{RxBytes: 4000, TxBytes: 2000}},
			{Name: "ens18", Counters: InterfaceCounters{RxBytes: 8000, TxBytes: 6000}},
		},
		linkStates: map[string]PortLinkState{
			"enp3s0": {Carrier: true, Speed: 1000, Duplex: "full"},
			"ens18":  {Carrier: true, Speed: 2500, Duplex: "full"},
		},
		lldpPeers: map[string]interface{}{
			"enp3s0": map[string]string{"name": "core-sw"},
			"ens18":  map[string]string{"name": "access-sw"},
		},
	}
	store := &mockStateStore{
		st: state.State{
			AppliedUUID: "200",
			PhysicalPortRoles: map[string]string{
				"enp3s0":      "upstream",
				"ens18":       "downstream",
				"nonexistent": "downstream",
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
		t.Fatalf("expected 2 interfaces (upv0, downv0), got %d: %+v", len(payload.Interfaces), payload.Interfaces)
	}
	if payload.Interfaces[0].Name != "upv0" || payload.Interfaces[0].Counters.RxBytes != 4000 {
		t.Fatalf("expected upv0 with enp3s0 counters (4000), got %+v", payload.Interfaces[0])
	}
	if payload.Interfaces[1].Name != "downv0" || payload.Interfaces[1].Counters.RxBytes != 8000 {
		t.Fatalf("expected downv0 with ens18 counters (8000), got %+v", payload.Interfaces[1])
	}
	if payload.LinkState.Upstream["enp3s0"].Speed != 1000 {
		t.Fatalf("expected enp3s0 in link_state.upstream, got %+v", payload.LinkState.Upstream)
	}
	if payload.LinkState.Downstream["ens18"].Speed != 2500 {
		t.Fatalf("expected ens18 in link_state.downstream, got %+v", payload.LinkState.Downstream)
	}
	if _, ok := payload.LinkState.Downstream["nonexistent"]; ok {
		t.Fatal("nonexistent interface must be omitted from link_state")
	}
	if _, ok := payload.LLDPPeers.Upstream["enp3s0"]; !ok {
		t.Fatalf("expected enp3s0 in lldp_peers.upstream, got %+v", payload.LLDPPeers.Upstream)
	}
	if _, ok := payload.LLDPPeers.Downstream["ens18"]; !ok {
		t.Fatalf("expected ens18 in lldp_peers.downstream, got %+v", payload.LLDPPeers.Downstream)
	}
}
