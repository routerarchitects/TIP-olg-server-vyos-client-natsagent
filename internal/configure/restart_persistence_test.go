package configure

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/renderer"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/state"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/testutil"
)

/*
TC-RESTART-001
Type: Recovery
Title: Restart with persisted state does not reapply same UUID
Summary:
Runs configure once with a file-backed state store, then constructs a new
service and state store instance pointing at the same state path. The second
service should detect the persisted UUID and skip render/apply.

Validates:
  - first service saves the requested UUID
  - second service loads persisted state after restart
  - second service skips render/apply/save
  - already_in_sync success is published
*/
func TestRestartWithPersistedStateDoesNotReapplySameUUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	target := testutil.MinimalTarget
	uuid := "cfg-restart-persisted"

	first := newRestartConfigureFixture(t, path, target, uuid)
	if err := first.service.Handle(context.Background(), first.msg); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if got := first.apply.Calls(); got != 1 {
		t.Fatalf("first apply calls got=%d want=1", got)
	}

	second := newRestartConfigureFixture(t, path, target, uuid)
	if err := second.service.Handle(context.Background(), second.msg); err != nil {
		t.Fatalf("second handle: %v", err)
	}

	if got := second.renderer.Calls(); got != 0 {
		t.Fatalf("renderer calls after restart got=%d want=0", got)
	}
	if got := second.apply.Calls(); got != 0 {
		t.Fatalf("apply calls after restart got=%d want=0", got)
	}
	if !second.client.ContainsStatus("success", "already_in_sync") {
		t.Fatal("expected already_in_sync status after restart")
	}
}

/*
TC-RESTART-002
Type: Recovery
Title: Restart with missing state reapplies config
Summary:
Starts a service with a state path that does not exist. Missing local
checkpoint state means no UUID is trusted as applied, so the service should
render/apply/save the desired config safely.

Validates:
  - missing state file is accepted
  - render/apply/save run once
  - state file is created with requested UUID
*/
func TestRestartWithMissingStateReappliesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "state.json")
	fixture := newRestartConfigureFixture(t, path, testutil.MinimalTarget, "cfg-restart-missing")

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.renderer.Calls() != 1 || fixture.apply.Calls() != 1 {
		t.Fatalf("calls got renderer=%d apply=%d want renderer=1 apply=1", fixture.renderer.Calls(), fixture.apply.Calls())
	}
	loaded, err := state.NewFileStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("load saved state: %v", err)
	}
	if loaded.AppliedUUID != fixture.msg.UUID {
		t.Fatalf("applied_uuid got=%q want=%q", loaded.AppliedUUID, fixture.msg.UUID)
	}
}

/*
TC-RESTART-003
Type: Negative
Title: Restart with corrupt state fails safely
Summary:
Writes malformed JSON to the state path and starts configure processing.
The corrupt state must not be trusted and the service should stop before
render/apply/save side effects.

Validates:
  - corrupt state returns state_load_failed
  - renderer/apply are not called
  - no new checkpoint is saved
*/
func TestRestartWithCorruptStateFailsSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}
	fixture := newRestartConfigureFixture(t, path, testutil.MinimalTarget, "cfg-restart-corrupt")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if fixture.renderer.Calls() != 0 || fixture.apply.Calls() != 0 {
		t.Fatalf("side effects got renderer=%d apply=%d want 0", fixture.renderer.Calls(), fixture.apply.Calls())
	}
	result, ok := fixture.client.LastResult()
	if !ok {
		t.Fatal("expected failure result")
	}
	if result.ErrorCode != "state_load_failed" {
		t.Fatalf("error_code got=%q want=state_load_failed", result.ErrorCode)
	}
}

/*
TC-RESTART-004
Type: Safety
Title: Restart reads state before configure decision
Summary:
Uses a state store that asserts render/apply have not happened when Load is
called. This proves the service reads checkpoint state before deciding to
skip or apply desired config.

Validates:
  - state Load happens before render
  - state Load happens before apply
  - same UUID skips side effects after state read
*/
func TestRestartReadsStateBeforeConfigureDecision(t *testing.T) {
	msg := testutil.MinimalConfigureNotification()
	msg.UUID = "cfg-restart-order"
	desired := testutil.DesiredConfig(msg.Target, msg.UUID, testutil.MinimalDesiredConfig().Record.Payload)
	client := &testutil.FakeConfigureClient{Desired: &desired}
	rndr := &testutil.FakeRenderer{}
	apply := &testutil.FakeApplyEngine{}
	store := &stateLoadOrderStore{
		current: state.State{Target: msg.Target, AppliedUUID: msg.UUID},
		check: func() error {
			if rndr.Calls() != 0 || apply.Calls() != 0 {
				t.Fatalf("state loaded after side effects: render=%d apply=%d", rndr.Calls(), apply.Calls())
			}
			return nil
		},
	}
	svc := newRestartService(t, client, store, rndr, apply)

	if err := svc.Handle(context.Background(), msg); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if rndr.Calls() != 0 || apply.Calls() != 0 {
		t.Fatalf("side effects got renderer=%d apply=%d want 0", rndr.Calls(), apply.Calls())
	}
}

type restartConfigureFixture struct {
	msg      agentcore.ConfigureNotification
	client   *testutil.FakeConfigureClient
	renderer *testutil.FakeRenderer
	apply    *testutil.FakeApplyEngine
	service  *Service
}

func newRestartConfigureFixture(t *testing.T, path, target, uuid string) restartConfigureFixture {
	t.Helper()

	msg := testutil.MinimalConfigureNotification()
	msg.Target = target
	msg.UUID = uuid
	msg.RPCID = "rpc-" + uuid
	desired := testutil.DesiredConfig(target, uuid, testutil.MinimalDesiredConfig().Record.Payload)
	client := &testutil.FakeConfigureClient{Desired: &desired}
	rndr := &testutil.FakeRenderer{
		Output:    renderer.Output{Target: target, UUID: uuid, Text: "set system host-name restart\n"},
		UseOutput: true,
	}
	apply := &testutil.FakeApplyEngine{}
	store := state.NewFileStore(path)
	svc := newRestartService(t, client, store, rndr, apply)

	return restartConfigureFixture{
		msg:      msg,
		client:   client,
		renderer: rndr,
		apply:    apply,
		service:  svc,
	}
}

func newRestartService(t *testing.T, client *testutil.FakeConfigureClient, store StateStore, rndr Renderer, apply ApplyEngine) *Service {
	t.Helper()

	svc, err := NewService(Dependencies{
		Client:      client,
		StateStore:  store,
		Renderer:    rndr,
		ApplyEngine: apply,
		Now: func() time.Time {
			return time.Date(2026, 6, 8, 13, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

type stateLoadOrderStore struct {
	current state.State
	check   func() error
}

func (s *stateLoadOrderStore) Load(ctx context.Context) (state.State, error) {
	if s.check != nil {
		if err := s.check(); err != nil {
			return state.State{}, err
		}
	}
	return s.current, nil
}

func (s *stateLoadOrderStore) Save(ctx context.Context, st state.State) error {
	s.current = st
	return nil
}
