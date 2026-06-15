package configure

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/actions"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/testutil"
)

/*
TC-CONCURRENCY-001
Type: Concurrency
Title: Concurrent configure events same UUID apply at most once
Summary:
Runs multiple goroutines against one configure service with the same UUID.
The service mutex and persisted state decision should allow only one apply
and one checkpoint; later duplicate calls should observe already-in-sync.

Validates:
  - no panic
  - apply count is at most one
  - save count is at most one
  - final state contains requested UUID
*/
func TestConcurrentConfigureEventsSameUUIDApplyAtMostOnce(t *testing.T) {
	fixture := newPhase3WorkflowFixture(t, "cfg-concurrent-same")
	runConcurrentConfigureHandles(t, fixture.service, fixture.msg, 8)

	if got := fixture.apply.Calls(); got != 1 {
		t.Fatalf("apply calls got=%d want=1", got)
	}
	if got := fixture.store.SaveCalls(); got != 1 {
		t.Fatalf("save calls got=%d want=1", got)
	}
	if got := fixture.store.CurrentState().AppliedUUID; got != fixture.msg.UUID {
		t.Fatalf("current uuid got=%q want=%q", got, fixture.msg.UUID)
	}
}

/*
TC-CONCURRENCY-003
Type: Concurrency
Title: Configure and action can overlap safely
Summary:
Runs configure and action services concurrently with independent fake
dependencies. The workflows should both complete without panic or race when
run under the race detector.

Validates:
  - configure succeeds
  - action succeeds
  - both publish success results
*/
func TestConfigureAndActionCanOverlapSafely(t *testing.T) {
	configureFixture := newPhase3WorkflowFixture(t, "cfg-concurrent-action")
	actionClient := &testutil.StatusResultRecorder{}
	actionExec := &concurrencyActionExecutor{
		output: actions.Output{
			Message: "concurrent action completed",
			Payload: json.RawMessage(`{"ok":true}`),
		},
	}
	actionService, err := actions.NewService(actions.Dependencies{
		Client:    actionClient,
		Enabled:   []string{actions.ActionTrace},
		Executors: map[string]actions.Executor{actions.ActionTrace: actionExec},
		Now: func() time.Time {
			return time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new action service: %v", err)
	}
	actionMsg := agentcore.ActionCommand{
		Version:     "1.0",
		RPCID:       "rpc-concurrent-action",
		Target:      testutil.MinimalTarget,
		CommandType: "action",
		Action:      actions.ActionTrace,
		Payload:     json.RawMessage(`{"host":"8.8.8.8"}`),
		Timestamp:   time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- configureFixture.service.Handle(context.Background(), configureFixture.msg)
	}()
	go func() {
		defer wg.Done()
		errs <- actionService.Handle(context.Background(), actionMsg)
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent workflow error: %v", err)
		}
	}

	if !configureFixture.client.ContainsResult("success", "configure") {
		t.Fatal("expected configure success result")
	}
	if !actionClient.ContainsResult("success", "action") {
		t.Fatal("expected action success result")
	}
}

/*
TC-CONCURRENCY-005
Type: Load
Title: Burst configure events no panic no race
Summary:
Runs a bounded burst of duplicate configure events against one service.
This keeps CI runtime small while exercising the serialized duplicate event
path under the race detector.

Validates:
  - no panic
  - no race under go test -race
  - final state remains valid
*/
func TestBurstConfigureEventsNoPanicNoRace(t *testing.T) {
	fixture := newPhase3WorkflowFixture(t, "cfg-burst-same")
	runConcurrentConfigureHandles(t, fixture.service, fixture.msg, 16)

	if got := fixture.store.CurrentState().AppliedUUID; got != fixture.msg.UUID {
		t.Fatalf("current uuid got=%q want=%q", got, fixture.msg.UUID)
	}
	if fixture.apply.Calls() != 1 {
		t.Fatalf("apply calls got=%d want=1", fixture.apply.Calls())
	}
}

func runConcurrentConfigureHandles(t *testing.T, svc *Service, msg agentcore.ConfigureNotification, workers int) {
	t.Helper()

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- svc.Handle(context.Background(), msg)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent handle error: %v", err)
		}
	}
}

type concurrencyActionExecutor struct {
	output actions.Output
	err    error

	mu    sync.Mutex
	calls int
}

func (e *concurrencyActionExecutor) Execute(ctx context.Context, msg agentcore.ActionCommand) (actions.Output, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.calls++
	if e.err != nil {
		return actions.Output{}, e.err
	}
	return e.output, nil
}
