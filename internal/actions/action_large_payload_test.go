package actions

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/testutil"
)

/*
TC-PERF-005
Type: Load
Title: Large action payload rejected or handled safely
Summary:
Runs the placeholder trace executor through the action service with a large
but valid JSON payload. Current placeholder policy accepts valid JSON, so
the action should complete without panic and publish one success result.

Validates:
  - large valid action payload is handled safely
  - completed status is published
  - success result is published
*/
func TestLargeActionPayloadRejectedOrHandledSafely(t *testing.T) {
	client := &testutil.StatusResultRecorder{}
	svc := newActionLoadService(t, client, NewPlaceholderTraceExecutor())
	msg := actionLoadCommand("rpc-large-action")
	msg.Payload = largeActionPayload(512)

	if err := svc.Handle(context.Background(), msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if !client.ContainsStatus("success", "completed") {
		t.Fatal("expected completed status")
	}
	if !client.ContainsResult("success", "action") {
		t.Fatal("expected action success result")
	}
}

/*
TC-CONCURRENCY-006
Type: Load
Title: Burst action events no panic no race
Summary:
Runs a bounded burst of action commands through one action service with a
thread-safe fake executor. This keeps CI fast while exercising shared
service reads and recorder writes under the race detector.

Validates:
  - no panic
  - no race under go test -race
  - executor is called once per action
  - all action attempts publish success results
*/
func TestBurstActionEventsNoPanicNoRace(t *testing.T) {
	client := &testutil.StatusResultRecorder{}
	executor := &actionLoadExecutor{
		output: Output{
			Message: "burst action completed",
			Payload: json.RawMessage(`{"ok":true}`),
		},
	}
	svc := newActionLoadService(t, client, executor)

	const workers = 12
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs <- svc.Handle(context.Background(), actionLoadCommand("rpc-burst-action-"+strconv.Itoa(i)))
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("burst action error: %v", err)
		}
	}
	if executor.Calls() != workers {
		t.Fatalf("executor calls got=%d want=%d", executor.Calls(), workers)
	}
	if got := countActionResults(client.Results(), "success"); got != workers {
		t.Fatalf("success result count got=%d want=%d", got, workers)
	}
}

type actionLoadExecutor struct {
	output Output
	err    error

	mu    sync.Mutex
	calls int
}

func (e *actionLoadExecutor) Execute(ctx context.Context, msg agentcore.ActionCommand) (Output, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.calls++
	if e.err != nil {
		return Output{}, e.err
	}
	return e.output, nil
}

func (e *actionLoadExecutor) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func newActionLoadService(t *testing.T, client *testutil.StatusResultRecorder, executor Executor) *Service {
	t.Helper()

	svc, err := NewService(Dependencies{
		Client:    client,
		Enabled:   []string{ActionTrace},
		Executors: map[string]Executor{ActionTrace: executor},
		Now: func() time.Time {
			return time.Date(2026, 6, 8, 15, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func actionLoadCommand(rpcID string) agentcore.ActionCommand {
	return agentcore.ActionCommand{
		Version:     "1.0",
		RPCID:       rpcID,
		Target:      testutil.MinimalTarget,
		CommandType: "action",
		Action:      ActionTrace,
		Payload:     json.RawMessage(`{"host":"8.8.8.8"}`),
		Timestamp:   time.Date(2026, 6, 8, 15, 0, 0, 0, time.UTC),
	}
}

func largeActionPayload(repetitions int) json.RawMessage {
	var b strings.Builder
	b.WriteString(`{"hosts":[`)
	for i := 0; i < repetitions; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"203.0.113.`)
		b.WriteString(strconv.Itoa(i % 255))
		b.WriteString(`"`)
	}
	b.WriteString(`]}`)
	return json.RawMessage(b.String())
}

func countActionResults(results []agentcore.ResultEnvelope, result string) int {
	count := 0
	for _, got := range results {
		if got.Result == result && got.CommandType == "action" {
			count++
		}
	}
	return count
}
