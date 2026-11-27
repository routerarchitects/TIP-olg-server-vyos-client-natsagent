package configure

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/state"
)

func TestPortMap_ApplySuccess_Checkpoint(t *testing.T) {
	desired := newDesired("vyos", "cfg-1")
	client := &fakeConfigureClient{desired: desired}
	store := &fakeStateStore{}
	rndr := &fakeRenderer{
		output: renderer.Output{
			Target: "vyos",
			UUID:   "cfg-1",
			Text:   "set ...",
			PhysicalPortRoles: map[string]string{
				"eth0": "upstream",
				"eth1": "downstream",
			},
		},
	}
	apply := &fakeApplyEngine{}

	svc := newConfigureServiceForTest(t, client, store, rndr, apply, time.Now)
	msg := agentcore.ConfigureNotification{
		Target: "vyos",
		UUID:   "cfg-1",
		RPCID:  "rpc-1",
	}

	if err := svc.Handle(context.Background(), msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(store.saved) == 0 {
		t.Fatal("expected saved state checkpoint")
	}
	saved := store.saved[len(store.saved)-1]
	if saved.AppliedUUID != "cfg-1" {
		t.Fatalf("applied uuid got=%s want=cfg-1", saved.AppliedUUID)
	}
	if saved.PhysicalPortRoles["eth0"] != "upstream" || saved.PhysicalPortRoles["eth1"] != "downstream" {
		t.Fatalf("unexpected port roles: %+v", saved.PhysicalPortRoles)
	}
	if saved.TopologyDegraded {
		t.Fatal("expected topology degraded false on success")
	}
}

func TestPortMap_RoleChange(t *testing.T) {
	desired1 := newDesired("vyos", "cfg-1")
	client := &fakeConfigureClient{desired: desired1}
	store := &fakeStateStore{}
	rndr := &fakeRenderer{
		output: renderer.Output{
			Target: "vyos",
			UUID:   "cfg-1",
			PhysicalPortRoles: map[string]string{
				"eth0": "upstream",
				"eth1": "downstream",
			},
		},
	}
	apply := &fakeApplyEngine{}
	svc := newConfigureServiceForTest(t, client, store, rndr, apply, time.Now)

	_ = svc.Handle(context.Background(), agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-1", RPCID: "rpc-1"})

	// Now apply cfg-2 swapping port roles
	desired2 := newDesired("vyos", "cfg-2")
	client.desired = desired2
	rndr.output = renderer.Output{
		Target: "vyos",
		UUID:   "cfg-2",
		PhysicalPortRoles: map[string]string{
			"eth0": "downstream",
			"eth1": "upstream",
		},
	}
	if err := svc.Handle(context.Background(), agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-2", RPCID: "rpc-2"}); err != nil {
		t.Fatalf("handle cfg-2: %v", err)
	}

	if len(store.saved) == 0 {
		t.Fatal("expected saved state")
	}
	saved := store.saved[len(store.saved)-1]
	if saved.AppliedUUID != "cfg-2" {
		t.Fatalf("expected uuid cfg-2, got %s", saved.AppliedUUID)
	}
	if saved.PhysicalPortRoles["eth0"] != "downstream" || saved.PhysicalPortRoles["eth1"] != "upstream" {
		t.Fatalf("expected swapped roles, got %+v", saved.PhysicalPortRoles)
	}
}

func TestPortMap_FailedApply_RollbackPreservation(t *testing.T) {
	initialRoles := map[string]string{"eth0": "upstream", "eth1": "downstream"}
	store := &fakeStateStore{
		loadState: state.State{
			Target:            "vyos",
			AppliedUUID:       "cfg-good",
			PhysicalPortRoles: initialRoles,
		},
	}
	desired := newDesired("vyos", "cfg-bad")
	client := &fakeConfigureClient{desired: desired}
	rndr := &fakeRenderer{
		output: renderer.Output{
			Target:            "vyos",
			UUID:              "cfg-bad",
			PhysicalPortRoles: map[string]string{"eth0": "downstream"},
		},
	}
	apply := &fakeApplyEngine{err: errors.New("commit failed")}

	svc := newConfigureServiceForTest(t, client, store, rndr, apply, time.Now)
	msg := agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-bad", RPCID: "rpc-fail"}

	err := svc.Handle(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error on failed apply")
	}

	// Verify store was not overwritten with cfg-bad
	if len(store.saved) != 0 {
		t.Fatalf("expected no save on failed apply, got %d saves", len(store.saved))
	}
	if svc.IsTopologyDegraded() {
		t.Fatal("normal rollback must not set topology degraded")
	}
}

func TestPortMap_CheckpointSaveFailure_Degraded(t *testing.T) {
	desired := newDesired("vyos", "cfg-1")
	client := &fakeConfigureClient{desired: desired}
	store := &fakeStateStore{saveErr: errors.New("disk full")}
	rndr := &fakeRenderer{
		output: renderer.Output{
			Target:            "vyos",
			UUID:              "cfg-1",
			PhysicalPortRoles: map[string]string{"eth0": "upstream"},
		},
	}
	apply := &fakeApplyEngine{}

	svc := newConfigureServiceForTest(t, client, store, rndr, apply, time.Now)
	msg := agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-1", RPCID: "rpc-1"}

	err := svc.Handle(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error on save failure")
	}
	if !svc.IsTopologyDegraded() {
		t.Fatal("checkpoint save failure must flag topology as degraded")
	}
}

func TestPortMap_RollbackFailure_PersistsDegradedAndClearsOnSuccess(t *testing.T) {
	store := &fakeStateStore{
		loadState: state.State{
			Target:            "vyos",
			AppliedUUID:       "cfg-good",
			PhysicalPortRoles: map[string]string{"eth0": "upstream"},
		},
	}
	desired := newDesired("vyos", "cfg-bad")
	client := &fakeConfigureClient{desired: desired}
	rndr := &fakeRenderer{
		output: renderer.Output{
			Target:            "vyos",
			UUID:              "cfg-bad",
			PhysicalPortRoles: map[string]string{"eth0": "downstream"},
		},
	}
	apply := &fakeApplyEngine{err: errors.New("commit failed; rollback failed")}

	svc := newConfigureServiceForTest(t, client, store, rndr, apply, time.Now)
	if err := svc.Handle(context.Background(), agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-bad", RPCID: "rpc-1"}); err == nil {
		t.Fatal("expected error on rollback failure")
	}

	if !svc.IsTopologyDegraded() {
		t.Fatal("rollback failure must flag topology degraded in memory")
	}
	if len(store.saved) == 0 || !store.saved[len(store.saved)-1].TopologyDegraded {
		t.Fatalf("expected TopologyDegraded=true persisted to state store, got %+v", store.saved)
	}

	// Recovery on subsequent successful configure clears TopologyDegraded
	apply.err = nil
	client.desired = newDesired("vyos", "cfg-recover")
	rndr.output.UUID = "cfg-recover"
	if err := svc.Handle(context.Background(), agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-recover", RPCID: "rpc-2"}); err != nil {
		t.Fatalf("recovery handle failed: %v", err)
	}
	if svc.IsTopologyDegraded() {
		t.Fatal("expected TopologyDegraded cleared after successful configure and save")
	}
	last := store.saved[len(store.saved)-1]
	if last.AppliedUUID != "cfg-recover" || last.TopologyDegraded {
		t.Fatalf("expected clean recovered checkpoint, got %+v", last)
	}
}

type blockingApplyEngine struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingApplyEngine) Apply(ctx context.Context, _ renderer.Output) error {
	close(b.started)
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPortMap_ApplyInProgress_MarksTopologyDegraded(t *testing.T) {
	store := &fakeStateStore{}
	client := &fakeConfigureClient{desired: newDesired("vyos", "cfg-1")}
	rndr := &fakeRenderer{output: renderer.Output{Target: "vyos", UUID: "cfg-1"}}
	apply := &blockingApplyEngine{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	svc := newConfigureServiceForTest(t, client, store, rndr, apply, time.Now)

	done := make(chan error, 1)
	go func() {
		done <- svc.Handle(context.Background(), agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-1", RPCID: "rpc-1"})
	}()

	<-apply.started
	if !svc.IsTopologyDegraded() {
		t.Fatal("expected IsTopologyDegraded() true while Apply is in progress")
	}
	close(apply.release)

	if err := <-done; err != nil {
		t.Fatalf("handle failed: %v", err)
	}
	if svc.IsTopologyDegraded() {
		t.Fatal("expected IsTopologyDegraded() false after Apply completes")
	}
}

func TestPortMap_LegacyCheckpointUpgrade(t *testing.T) {
	desired := newDesired("vyos", "cfg-legacy")
	client := &fakeConfigureClient{desired: desired}
	store := &fakeStateStore{
		loadState: state.State{
			Target:            "vyos",
			AppliedUUID:       "cfg-legacy",
			PhysicalPortRoles: nil, // legacy checkpoint lacking port roles
		},
	}
	rndr := &fakeRenderer{
		output: renderer.Output{
			Target: "vyos",
			UUID:   "cfg-legacy",
			PhysicalPortRoles: map[string]string{
				"eth0": "upstream",
				"eth1": "downstream",
			},
		},
	}
	apply := &fakeApplyEngine{}

	svc := newConfigureServiceForTest(t, client, store, rndr, apply, time.Now)

	if err := svc.UpgradeLegacyCheckpoint(context.Background(), "vyos"); err != nil {
		t.Fatalf("upgrade legacy checkpoint: %v", err)
	}

	if apply.calls != 0 {
		t.Fatalf("legacy checkpoint upgrade must not call VyOS CLI apply, got %d calls", apply.calls)
	}
	if rndr.calls != 1 {
		t.Fatalf("legacy checkpoint upgrade should render normalization once, got %d calls", rndr.calls)
	}

	if len(store.saved) == 0 {
		t.Fatal("expected upgraded checkpoint saved")
	}
	saved := store.saved[len(store.saved)-1]
	if saved.PhysicalPortRoles["eth0"] != "upstream" || saved.PhysicalPortRoles["eth1"] != "downstream" {
		t.Fatalf("unexpected upgraded roles: %+v", saved.PhysicalPortRoles)
	}
}

func TestInterval_DynamicAdjustments(t *testing.T) {
	var reportedInterval int
	onIntervalChanged := func(sec int) {
		reportedInterval = sec
	}

	desiredWithInterval := func(interval int) *agentcore.StoredDesiredConfig {
		raw := map[string]interface{}{}
		if interval > 0 {
			raw["metrics"] = map[string]interface{}{
				"statistics": map[string]interface{}{
					"interval": interval,
				},
			}
		}
		b, _ := json.Marshal(raw)
		return &agentcore.StoredDesiredConfig{
			Record: agentcore.DesiredConfigRecord{
				Version:   "1.0",
				RPCID:     "rpc-1",
				Target:    "vyos",
				UUID:      "cfg-int",
				Payload:   b,
				Timestamp: time.Now().UTC(),
			},
			Bucket: "cfg_desired",
			Key:    "desired.vyos",
		}
	}

	// 1. Valid custom interval: 120s
	client := &fakeConfigureClient{desired: desiredWithInterval(120)}
	store := &fakeStateStore{}
	rndr := &fakeRenderer{}
	apply := &fakeApplyEngine{}

	svc, _ := NewService(Dependencies{
		Client:            client,
		StateStore:        store,
		Renderer:          rndr,
		ApplyEngine:       apply,
		OnIntervalChanged: onIntervalChanged,
		Now:               time.Now,
	})

	_ = svc.Handle(context.Background(), agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-int", RPCID: "rpc-1"})
	if reportedInterval != 120 {
		t.Fatalf("expected reported interval 120, got %d", reportedInterval)
	}

	// 2. Missing interval: reports 60 (declarative 60s fallback)
	client.desired = desiredWithInterval(0)
	client.desired.Record.UUID = "cfg-no-int"
	_ = svc.Handle(context.Background(), agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-no-int", RPCID: "rpc-2"})
	if reportedInterval != 60 {
		t.Fatalf("expected reported interval 60 on omission, got %d", reportedInterval)
	}

	// 3. Failed apply preserves previous interval without invoking callback
	reportedInterval = 999
	apply.err = errors.New("apply error")
	client.desired = desiredWithInterval(300)
	client.desired.Record.UUID = "cfg-fail"
	_ = svc.Handle(context.Background(), agentcore.ConfigureNotification{Target: "vyos", UUID: "cfg-fail", RPCID: "rpc-3"})
	if reportedInterval != 999 {
		t.Fatalf("failed apply must not invoke interval change callback, got %d", reportedInterval)
	}
}

func TestExtractStatisticsInterval(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		expected int
	}{
		{
			name:     "empty payload",
			payload:  "",
			expected: 60,
		},
		{
			name:     "invalid json",
			payload:  "not-json",
			expected: 60,
		},
		{
			name:     "unwrapped metrics with 300s interval",
			payload:  `{"metrics":{"statistics":{"interval":300}}}`,
			expected: 300,
		},
		{
			name: "wrapped in config envelope with 300s interval",
			payload: `{
				"uuid": 12345,
				"config": {
					"metrics": {
						"statistics": {
							"interval": 300
						}
					}
				}
			}`,
			expected: 300,
		},
		{
			name: "wrapped in config envelope with sub-60 interval falls back to 60",
			payload: `{
				"uuid": 12345,
				"config": {
					"metrics": {
						"statistics": {
							"interval": 15
						}
					}
				}
			}`,
			expected: 60,
		},
		{
			name: "wrapped in config envelope without metrics falls back to 60",
			payload: `{
				"uuid": 12345,
				"config": {
					"interfaces": []
				}
			}`,
			expected: 60,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractStatisticsInterval(json.RawMessage(tc.payload))
			if got != tc.expected {
				t.Errorf("extractStatisticsInterval() = %d, expected %d", got, tc.expected)
			}
		})
	}
}
