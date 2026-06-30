package actions

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/testutil"
)

/*
TC-ACTION-WORKFLOW-001
Type: Positive
Title: Trace happy path publishes received executing completed
Summary:
Runs an enabled trace action with a successful fake executor.
The action service should publish the deterministic happy-path status
sequence and should not publish failed status.

Validates:
  - received status is first
  - executing status is second
  - completed status is third
  - no failed status is published
*/
func TestActionTraceHappyPathPublishesReceivedExecutingCompleted(t *testing.T) {
	fixture := newActionWorkflowFixture(t)

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	assertStatusStages(t, fixture.client, []string{"received", "executing", "completed"})
	if fixture.client.ContainsStatus("failure", "failed") {
		t.Fatal("unexpected failed status")
	}
}

/*
TC-ACTION-WORKFLOW-002
Type: Positive
Title: Trace happy path publishes final result
Summary:
Runs an enabled trace action with a fake executor that returns a known
message and payload. The service should publish a final success result
with action metadata and executor output.

Validates:
  - result is success
  - command_type is action
  - action is trace
  - target and rpc_id are preserved
  - executor payload and message are propagated
*/
func TestActionTraceHappyPathPublishesFinalResult(t *testing.T) {
	fixture := newActionWorkflowFixture(t)

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	result, ok := fixture.client.LastResult()
	if !ok {
		t.Fatal("expected result")
	}
	if result.Result != "success" || result.CommandType != "action" || result.Action != ActionTrace {
		t.Fatalf("unexpected result metadata: %+v", result)
	}
	if result.Target != fixture.msg.Target || result.RPCID != fixture.msg.RPCID {
		t.Fatalf("result correlation got target=%q rpc_id=%q", result.Target, result.RPCID)
	}
	if result.Message != fixture.executor.Output.Message {
		t.Fatalf("message got=%q want=%q", result.Message, fixture.executor.Output.Message)
	}
	if string(result.Payload) != string(fixture.executor.Output.Payload) {
		t.Fatalf("payload got=%s want=%s", string(result.Payload), string(fixture.executor.Output.Payload))
	}
}

/*
TC-ACTION-WORKFLOW-003
Type: Negative
Title: Unsupported action fails safely
Summary:
Enables an action name but does not provide an executor for that action.
The service should publish unsupported_action failure and avoid completed
or success output.

Validates:
  - service returns error
  - failure result uses unsupported_action
  - failed status is published
  - known executor is not called
  - completed/success output is absent
*/
func TestActionUnsupportedActionFails(t *testing.T) {
	fixture := newActionWorkflowFixture(t)
	fixture.msg.Action = "ping"
	service := newActionServiceWorkflowForTest(
		t,
		fixture.client,
		map[string]Executor{ActionTrace: fixture.executor},
		[]string{ActionTrace, "ping"},
	)

	err := service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := fixture.executor.Calls(); got != 0 {
		t.Fatalf("executor calls got=%d want=0", got)
	}
	assertActionFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.Action, fixture.msg.RPCID, "unsupported_action")
	assertActionFailureWithoutCompleted(t, fixture.client)
}

/*
TC-ACTION-WORKFLOW-004
Type: Negative
Title: Disabled action fails safely
Summary:
Provides a trace executor but omits trace from the enabled action list.
The service should publish disabled_action failure and must not call the
executor.

Validates:
  - service returns error
  - failure result uses disabled_action
  - failed status is published
  - executor is not called
  - completed/success output is absent
*/
func TestActionDisabledActionFails(t *testing.T) {
	fixture := newActionWorkflowFixture(t)
	service := newActionServiceWorkflowForTest(
		t,
		fixture.client,
		map[string]Executor{ActionTrace: fixture.executor},
		[]string{"ping"},
	)

	err := service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := fixture.executor.Calls(); got != 0 {
		t.Fatalf("executor calls got=%d want=0", got)
	}
	assertActionFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.Action, fixture.msg.RPCID, "disabled_action")
	assertActionFailureWithoutCompleted(t, fixture.client)
}

/*
TC-ACTION-WORKFLOW-005
Type: Negative
Title: Execution failure publishes failed
Summary:
Runs an enabled trace action whose executor returns a generic error.
The service should publish received, executing, and failed statuses, then
publish an action_execute_failed result.

Validates:
  - executor is called once
  - status order is received, executing, failed
  - failure result uses action_execute_failed
  - completed/success output is absent
*/
func TestActionExecutionFailurePublishesFailed(t *testing.T) {
	fixture := newActionWorkflowFixture(t)
	fixture.executor.Err = errors.New("executor failed")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := fixture.executor.Calls(); got != 1 {
		t.Fatalf("executor calls got=%d want=1", got)
	}
	assertStatusStages(t, fixture.client, []string{"received", "executing", "failed"})
	assertActionFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.Action, fixture.msg.RPCID, "action_execute_failed")
	assertActionFailureWithoutCompleted(t, fixture.client)
}

/*
TC-ACTION-WORKFLOW-006
Type: Negative
Title: Missing or invalid payload fails safely
Summary:
Runs trace through the placeholder executor with empty and invalid JSON
payloads. The placeholder executor owns payload validation today, so it
is called and returns ErrInvalidActionPayload.

Validates:
  - empty payload fails with invalid_action_payload
  - malformed JSON payload fails with invalid_action_payload
  - failure status/result is published
  - completed/success output is absent
*/
func TestActionMissingRequiredPayloadFails(t *testing.T) {
	cases := []struct {
		name    string
		payload json.RawMessage
	}{
		{name: "empty payload", payload: nil},
		{name: "invalid json", payload: json.RawMessage(`{"host":`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &testutil.StatusResultRecorder{}
			svc := newActionServiceWorkflowForTest(
				t,
				client,
				map[string]Executor{ActionTrace: NewPlaceholderTraceExecutor()},
				[]string{ActionTrace},
			)
			msg := minimalTraceActionCommand()
			msg.Payload = tc.payload

			err := svc.Handle(context.Background(), msg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			assertActionFailureResult(t, client, msg.Target, msg.Action, msg.RPCID, "invalid_action_payload")
			assertActionFailureWithoutCompleted(t, client)
		})
	}
}

/*
TC-ACTION-WORKFLOW-007
Type: Safety
Title: Status sequence order is stable
Summary:
Checks success and executor-failure status sequences.
The service should publish deterministic ordered stages on both paths.

Validates:
  - success status order is received, executing, completed
  - executor failure status order is received, executing, failed
*/
func TestActionStatusSequenceOrderIsStable(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fixture := newActionWorkflowFixture(t)
		if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
			t.Fatalf("handle: %v", err)
		}
		assertStatusStages(t, fixture.client, []string{"received", "executing", "completed"})
	})

	t.Run("executor failure", func(t *testing.T) {
		fixture := newActionWorkflowFixture(t)
		fixture.executor.Err = errors.New("executor failed")
		err := fixture.service.Handle(context.Background(), fixture.msg)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		assertStatusStages(t, fixture.client, []string{"received", "executing", "failed"})
	})
}

/*
TC-ACTION-WORKFLOW-008
Type: Safety
Title: Failure does not publish completed
Summary:
Exercises unsupported, disabled, executor-failure, and invalid-payload
paths. Failed action workflows must not publish completed status or
success result.

Validates:
  - unsupported action publishes no completed/success
  - disabled action publishes no completed/success
  - executor failure publishes no completed/success
  - invalid payload publishes no completed/success
*/
func TestActionFailureDoesNotPublishCompleted(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T) *testutil.StatusResultRecorder
	}{
		{
			name: "unsupported action",
			run: func(t *testing.T) *testutil.StatusResultRecorder {
				fixture := newActionWorkflowFixture(t)
				fixture.msg.Action = "ping"
				service := newActionServiceWorkflowForTest(t, fixture.client, map[string]Executor{ActionTrace: fixture.executor}, []string{ActionTrace, "ping"})
				if err := service.Handle(context.Background(), fixture.msg); err == nil {
					t.Fatal("expected error, got nil")
				}
				return fixture.client
			},
		},
		{
			name: "disabled action",
			run: func(t *testing.T) *testutil.StatusResultRecorder {
				fixture := newActionWorkflowFixture(t)
				service := newActionServiceWorkflowForTest(t, fixture.client, map[string]Executor{ActionTrace: fixture.executor}, []string{"ping"})
				if err := service.Handle(context.Background(), fixture.msg); err == nil {
					t.Fatal("expected error, got nil")
				}
				return fixture.client
			},
		},
		{
			name: "executor failure",
			run: func(t *testing.T) *testutil.StatusResultRecorder {
				fixture := newActionWorkflowFixture(t)
				fixture.executor.Err = errors.New("executor failed")
				if err := fixture.service.Handle(context.Background(), fixture.msg); err == nil {
					t.Fatal("expected error, got nil")
				}
				return fixture.client
			},
		},
		{
			name: "invalid payload",
			run: func(t *testing.T) *testutil.StatusResultRecorder {
				client := &testutil.StatusResultRecorder{}
				service := newActionServiceWorkflowForTest(t, client, map[string]Executor{ActionTrace: NewPlaceholderTraceExecutor()}, []string{ActionTrace})
				msg := minimalTraceActionCommand()
				msg.Payload = json.RawMessage(`not-json`)
				if err := service.Handle(context.Background(), msg); err == nil {
					t.Fatal("expected error, got nil")
				}
				return client
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.run(t)
			assertActionFailureWithoutCompleted(t, client)
		})
	}
}

/*
TC-ACTION-WORKFLOW-009
Type: Positive
Title: Action preserves correlation data
Summary:
Checks both success and failure action results for correlation metadata.
Result envelopes should preserve target, action, and RPC ID.

Validates:
  - success result preserves target, action, and rpc_id
  - failure result preserves target, action, and rpc_id
  - result command_type is action
*/
func TestActionPreservesCorrelationData(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fixture := newActionWorkflowFixture(t)
		if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
			t.Fatalf("handle: %v", err)
		}

		result, ok := fixture.client.LastResult()
		if !ok {
			t.Fatal("expected result")
		}
		assertActionResultCorrelation(t, result, fixture.msg.Target, fixture.msg.Action, fixture.msg.RPCID)
	})

	t.Run("failure", func(t *testing.T) {
		fixture := newActionWorkflowFixture(t)
		fixture.executor.Err = errors.New("executor failed")
		if err := fixture.service.Handle(context.Background(), fixture.msg); err == nil {
			t.Fatal("expected error, got nil")
		}

		result, ok := fixture.client.LastResult()
		if !ok {
			t.Fatal("expected result")
		}
		assertActionResultCorrelation(t, result, fixture.msg.Target, fixture.msg.Action, fixture.msg.RPCID)
		if result.ErrorCode != "action_execute_failed" {
			t.Fatalf("error code got=%q want=action_execute_failed", result.ErrorCode)
		}
	})
}

type actionWorkflowFixture struct {
	msg      agentcore.ActionCommand
	client   *testutil.StatusResultRecorder
	executor *workflowFakeActionExecutor
	service  *Service
}

type workflowFakeActionExecutor struct {
	Output   Output
	Err      error
	Validate func(agentcore.ActionCommand) error
	Events   *testutil.EventRecorder

	calls  int
	inputs []agentcore.ActionCommand
}

func (f *workflowFakeActionExecutor) Execute(ctx context.Context, msg agentcore.ActionCommand) (Output, error) {
	if f.Events != nil {
		f.Events.Record("execute_action")
	}
	f.calls++
	f.inputs = append(f.inputs, cloneActionCommand(msg))
	if f.Validate != nil {
		if err := f.Validate(msg); err != nil {
			return Output{}, err
		}
	}
	if f.Err != nil {
		return Output{}, f.Err
	}
	return f.Output, nil
}

func (f *workflowFakeActionExecutor) Calls() int {
	return f.calls
}

func newActionWorkflowFixture(t *testing.T) actionWorkflowFixture {
	t.Helper()

	client := &testutil.StatusResultRecorder{}
	exec := &workflowFakeActionExecutor{
		Output: Output{
			Message: "fake trace completed",
			Payload: json.RawMessage(`{"executor":"fake_trace","ok":true}`),
		},
	}
	svc := newActionServiceWorkflowForTest(
		t,
		client,
		map[string]Executor{ActionTrace: exec},
		[]string{ActionTrace},
	)

	return actionWorkflowFixture{
		msg:      minimalTraceActionCommand(),
		client:   client,
		executor: exec,
		service:  svc,
	}
}

func newActionServiceWorkflowForTest(t *testing.T, client *testutil.StatusResultRecorder, execs map[string]Executor, enabled []string) *Service {
	t.Helper()

	svc, err := NewService(Dependencies{
		Client:    client,
		Enabled:   enabled,
		Executors: execs,
		Now: func() time.Time {
			return time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new action service: %v", err)
	}
	return svc
}

func minimalTraceActionCommand() agentcore.ActionCommand {
	return agentcore.ActionCommand{
		Version:     "1.0",
		RPCID:       "rpc-action-workflow",
		Target:      "vyos",
		CommandType: "action",
		Action:      ActionTrace,
		Payload:     json.RawMessage(`{"host":"8.8.8.8"}`),
	}
}

func assertStatusStages(t *testing.T, recorder *testutil.StatusResultRecorder, want []string) {
	t.Helper()

	got := recorder.StatusStages()
	if len(got) != len(want) {
		t.Fatalf("status stages got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("status stage[%d] got=%q want=%q; all stages=%v", i, got[i], want[i], got)
		}
	}
}

func assertActionFailureResult(t *testing.T, recorder *testutil.StatusResultRecorder, target, action, rpcID, code string) agentcore.ResultEnvelope {
	t.Helper()

	result, ok := recorder.LastResult()
	if !ok {
		t.Fatal("expected failure result")
	}
	if result.Result != "failure" {
		t.Fatalf("result got=%q want=failure: %+v", result.Result, result)
	}
	if result.ErrorCode != code {
		t.Fatalf("error_code got=%q want=%q: %+v", result.ErrorCode, code, result)
	}
	assertActionResultCorrelation(t, result, target, action, rpcID)
	return result
}

func assertActionFailureWithoutCompleted(t *testing.T, recorder *testutil.StatusResultRecorder) {
	t.Helper()

	if !recorder.ContainsStatus("failure", "failed") {
		t.Fatal("expected failed status")
	}
	if recorder.ContainsStatus("success", "completed") {
		t.Fatal("unexpected completed status")
	}
	for _, result := range recorder.Results() {
		if result.Result == "success" {
			t.Fatalf("unexpected success result: %+v", result)
		}
	}
}

func assertActionResultCorrelation(t *testing.T, result agentcore.ResultEnvelope, target, action, rpcID string) {
	t.Helper()

	if result.CommandType != "action" {
		t.Fatalf("command_type got=%q want=action: %+v", result.CommandType, result)
	}
	if result.Target != target || result.Action != action || result.RPCID != rpcID {
		t.Fatalf("correlation got target=%q action=%q rpc_id=%q want target=%q action=%q rpc_id=%q", result.Target, result.Action, result.RPCID, target, action, rpcID)
	}
}

func cloneActionCommand(in agentcore.ActionCommand) agentcore.ActionCommand {
	in.Payload = append(json.RawMessage(nil), in.Payload...)
	return in
}
