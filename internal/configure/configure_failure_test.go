package configure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/state"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/testutil"
)

/*
TC-CONFIGURE-FAILURE-001
Type: Negative
Title: Renderer failure stops before apply
Summary:
Runs configure with a renderer that returns an error.
The service should stop before apply and state save, publish failure,
and avoid publishing success.

Validates:
  - renderer is called exactly once
  - apply is not called
  - state save is not called
  - failure result uses render_failed
  - failure status is published
  - no success result/status is published
*/
func TestConfigureRendererFailureStopsBeforeApply(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-render-fail")
	fixture.renderer.Err = errors.New("renderer exploded")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := fixture.renderer.Calls(); got != 1 {
		t.Fatalf("renderer calls got=%d want=1", got)
	}
	if got := fixture.apply.Calls(); got != 0 {
		t.Fatalf("apply calls got=%d want=0", got)
	}
	if got := fixture.store.SaveCalls(); got != 0 {
		t.Fatalf("save calls got=%d want=0", got)
	}
	assertConfigureFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.UUID, fixture.msg.RPCID, "render_failed")
	assertConfigureFailurePublishedWithoutSuccess(t, fixture.client)
}

/*
TC-CONFIGURE-FAILURE-002
Type: Safety
Title: Renderer failure preserves previous state
Summary:
Seeds local state with an existing applied UUID and makes rendering fail
for a new desired UUID. The failed render must not modify persisted state
or record the new UUID as saved.

Validates:
  - state save is not called
  - current state remains the previous UUID
  - new UUID is not recorded as a saved state
*/
func TestConfigureRendererFailurePreservesPreviousState(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-render-new")
	fixture.store.Current = state.State{Target: fixture.msg.Target, AppliedUUID: "cfg-previous"}
	fixture.renderer.Err = errors.New("render failed")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	assertStatePreserved(t, fixture.store, "cfg-previous", fixture.msg.UUID)
}

/*
TC-CONFIGURE-FAILURE-003
Type: Negative
Title: Renderer failure publishes clear failure
Summary:
Runs a renderer failure and inspects the published result.
The failure result should be observable, identify render failure, and
preserve correlation data without leaking raw payload/rendered commands.

Validates:
  - result is failure
  - command_type is configure
  - error_code is render_failed
  - target, uuid, and rpc_id are preserved
  - failure message stays safe
*/
func TestConfigureRendererFailurePublishesClearFailure(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-render-clear")
	fixture.renderer.Err = errors.New("renderer failed with secret payload value")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	result := assertConfigureFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.UUID, fixture.msg.RPCID, "render_failed")
	if result.CommandType != "configure" {
		t.Fatalf("command_type got=%q want=configure", result.CommandType)
	}
	if strings.Contains(result.Message, "secret payload value") {
		t.Fatalf("failure message leaked dependency detail: %q", result.Message)
	}
}

/*
TC-CONFIGURE-FAILURE-004
Type: Negative
Title: Apply failure does not save state
Summary:
Runs configure with successful render and failed apply.
The service must report apply failure and must not checkpoint the UUID.

Validates:
  - renderer is called exactly once
  - apply is called exactly once
  - state save is not called
  - failure result uses apply_failed
  - no success result/status is published
*/
func TestConfigureApplyFailureDoesNotSaveState(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-apply-fail")
	fixture.apply.Err = errors.New("commit failed")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := fixture.renderer.Calls(); got != 1 {
		t.Fatalf("renderer calls got=%d want=1", got)
	}
	if got := fixture.apply.Calls(); got != 1 {
		t.Fatalf("apply calls got=%d want=1", got)
	}
	if got := fixture.store.SaveCalls(); got != 0 {
		t.Fatalf("save calls got=%d want=0", got)
	}
	assertConfigureFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.UUID, fixture.msg.RPCID, "apply_failed")
	assertConfigureFailurePublishedWithoutSuccess(t, fixture.client)
}

/*
TC-CONFIGURE-FAILURE-005
Type: Recovery
Title: Apply failure allows retry same UUID
Summary:
Runs configure twice with the same UUID while apply fails once and then
succeeds. The first failure must not checkpoint the UUID, allowing the
second attempt to render/apply again and save state.

Validates:
  - failed first attempt does not save state
  - second attempt renders and applies again
  - second attempt saves requested UUID
  - both failure and success results are observable
*/
func TestConfigureApplyFailureAllowsRetrySameUUID(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-apply-retry")
	fixture.apply.Errs = []error{errors.New("first apply failed"), nil}

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected first attempt error, got nil")
	}
	if got := fixture.renderer.Calls(); got != 1 {
		t.Fatalf("renderer calls after first attempt got=%d want=1", got)
	}
	if got := fixture.apply.Calls(); got != 1 {
		t.Fatalf("apply calls after first attempt got=%d want=1", got)
	}
	if got := fixture.store.SaveCalls(); got != 0 {
		t.Fatalf("save calls after first attempt got=%d want=0", got)
	}
	assertConfigureFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.UUID, fixture.msg.RPCID, "apply_failed")

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if got := fixture.renderer.Calls(); got != 2 {
		t.Fatalf("renderer calls after second attempt got=%d want=2", got)
	}
	if got := fixture.apply.Calls(); got != 2 {
		t.Fatalf("apply calls after second attempt got=%d want=2", got)
	}
	if got := fixture.store.SaveCalls(); got != 1 {
		t.Fatalf("save calls after second attempt got=%d want=1", got)
	}
	saved, ok := fixture.store.LastSavedState()
	if !ok {
		t.Fatal("expected saved state after retry")
	}
	if saved.AppliedUUID != fixture.msg.UUID {
		t.Fatalf("saved uuid got=%q want=%q", saved.AppliedUUID, fixture.msg.UUID)
	}
	if !fixture.client.ContainsResult("success", "configure") {
		t.Fatal("expected success result after retry")
	}
}

/*
TC-CONFIGURE-FAILURE-006
Type: Safety
Title: Apply failure preserves previous UUID
Summary:
Seeds local state with a previous UUID and makes apply fail for a new
desired UUID. The failed apply must not mutate current state or save
the new UUID.

Validates:
  - state save is not called
  - current state remains previous UUID
  - new UUID is not saved
  - failure result uses apply_failed
*/
func TestConfigureApplyFailurePreservesPreviousUUID(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-apply-new")
	fixture.store.Current = state.State{Target: fixture.msg.Target, AppliedUUID: "cfg-previous"}
	fixture.apply.Err = errors.New("apply failed")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	assertStatePreserved(t, fixture.store, "cfg-previous", fixture.msg.UUID)
	assertConfigureFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.UUID, fixture.msg.RPCID, "apply_failed")
}

/*
TC-CONFIGURE-FAILURE-007
Type: Negative
Title: State save failure marks configure failed
Summary:
Runs configure with successful render/apply and failing state save.
The handler should return error and publish configure failure without
publishing success.

Validates:
  - renderer is called exactly once
  - apply is called exactly once
  - state save is attempted once
  - failure result uses state_save_failed
  - no success result/status is published
*/
func TestConfigureStateSaveFailureMarksConfigureFailed(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-save-fail")
	fixture.store.SaveErr = errors.New("disk full")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := fixture.renderer.Calls(); got != 1 {
		t.Fatalf("renderer calls got=%d want=1", got)
	}
	if got := fixture.apply.Calls(); got != 1 {
		t.Fatalf("apply calls got=%d want=1", got)
	}
	if got := fixture.store.SaveCalls(); got != 1 {
		t.Fatalf("save calls got=%d want=1", got)
	}
	assertConfigureFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.UUID, fixture.msg.RPCID, "state_save_failed")
	assertConfigureFailurePublishedWithoutSuccess(t, fixture.client)
}

/*
TC-CONFIGURE-FAILURE-008
Type: Safety
Title: State save failure does not persist UUID
Summary:
Seeds local state with a previous UUID and makes checkpoint save fail
after successful render/apply. Attempted saves are observable, but the
fake store must not treat failed saves as persisted current state.

Validates:
  - state save is attempted once
  - current state remains previous UUID
  - requested UUID is not treated as applied
  - failure result uses state_save_failed
*/
func TestConfigureStateSaveFailureDoesNotPersistUUID(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-save-new")
	fixture.store.Current = state.State{Target: fixture.msg.Target, AppliedUUID: "cfg-previous"}
	fixture.store.SaveErr = errors.New("save failed")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := fixture.store.SaveCalls(); got != 1 {
		t.Fatalf("save calls got=%d want=1", got)
	}
	if current := fixture.store.CurrentState(); current.AppliedUUID != "cfg-previous" {
		t.Fatalf("current uuid got=%q want=cfg-previous", current.AppliedUUID)
	}
	if current := fixture.store.CurrentState(); current.AppliedUUID == fixture.msg.UUID {
		t.Fatalf("failed save persisted requested uuid %q", fixture.msg.UUID)
	}
	assertConfigureFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.UUID, fixture.msg.RPCID, "state_save_failed")
}

/*
TC-CONFIGURE-FAILURE-009
Type: Recovery
Title: State save failure retries safely
Summary:
Runs configure twice with the same UUID while state save fails once and
then succeeds. The failed checkpoint must not create already-in-sync
behavior; retry should render/apply/save again.

Validates:
  - first attempt renders and applies but fails save
  - second attempt renders and applies again
  - second attempt saves requested UUID
  - retry publishes success
*/
func TestConfigureStateSaveFailureRetriesSafely(t *testing.T) {
	fixture := newFailureWorkflowFixture(t, "cfg-save-retry")
	fixture.store.SaveErr = errors.New("first save failed")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected first attempt error, got nil")
	}
	if got := fixture.renderer.Calls(); got != 1 {
		t.Fatalf("renderer calls after first attempt got=%d want=1", got)
	}
	if got := fixture.apply.Calls(); got != 1 {
		t.Fatalf("apply calls after first attempt got=%d want=1", got)
	}
	if got := fixture.store.SaveCalls(); got != 1 {
		t.Fatalf("save calls after first attempt got=%d want=1", got)
	}
	if current := fixture.store.CurrentState(); current.AppliedUUID == fixture.msg.UUID {
		t.Fatalf("failed save persisted requested uuid %q", fixture.msg.UUID)
	}

	fixture.store.SaveErr = nil
	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if got := fixture.renderer.Calls(); got != 2 {
		t.Fatalf("renderer calls after second attempt got=%d want=2", got)
	}
	if got := fixture.apply.Calls(); got != 2 {
		t.Fatalf("apply calls after second attempt got=%d want=2", got)
	}
	if got := fixture.store.SaveCalls(); got != 2 {
		t.Fatalf("save calls after second attempt got=%d want=2", got)
	}
	if current := fixture.store.CurrentState(); current.AppliedUUID != fixture.msg.UUID {
		t.Fatalf("current uuid got=%q want=%q", current.AppliedUUID, fixture.msg.UUID)
	}
	if !fixture.client.ContainsResult("success", "configure") {
		t.Fatal("expected success result after retry")
	}
}

/*
TC-CONFIGURE-FAILURE-010
Type: Safety
Title: Configure failure does not publish success
Summary:
Exercises renderer, apply, and state-save failure modes.
Each failed configure attempt should publish failure output without any
success result or success status.

Validates:
  - renderer failure publishes no success
  - apply failure publishes no success
  - state-save failure publishes no success
*/
func TestConfigureFailureDoesNotPublishSuccess(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*phase3WorkflowFixture)
	}{
		{
			name: "renderer failure",
			mutate: func(f *phase3WorkflowFixture) {
				f.renderer.Err = errors.New("render failed")
			},
		},
		{
			name: "apply failure",
			mutate: func(f *phase3WorkflowFixture) {
				f.apply.Err = errors.New("apply failed")
			},
		},
		{
			name: "state save failure",
			mutate: func(f *phase3WorkflowFixture) {
				f.store.SaveErr = errors.New("save failed")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFailureWorkflowFixture(t, "cfg-no-success")
			tc.mutate(&fixture)

			err := fixture.service.Handle(context.Background(), fixture.msg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			assertConfigureFailurePublishedWithoutSuccess(t, fixture.client)
		})
	}
}

/*
TC-CONFIGURE-FAILURE-011
Type: Safety
Title: Configure failure includes correlation data
Summary:
Exercises renderer, apply, and state-save failure modes.
Failure results should preserve target, UUID, and RPC ID so operators
can trace failed workflows.

Validates:
  - renderer failure preserves correlation data
  - apply failure preserves correlation data
  - state-save failure preserves correlation data
*/
func TestConfigureFailureIncludesCorrelationData(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		mutate func(*phase3WorkflowFixture)
	}{
		{
			name: "renderer failure",
			code: "render_failed",
			mutate: func(f *phase3WorkflowFixture) {
				f.renderer.Err = errors.New("render failed")
			},
		},
		{
			name: "apply failure",
			code: "apply_failed",
			mutate: func(f *phase3WorkflowFixture) {
				f.apply.Err = errors.New("apply failed")
			},
		},
		{
			name: "state save failure",
			code: "state_save_failed",
			mutate: func(f *phase3WorkflowFixture) {
				f.store.SaveErr = errors.New("save failed")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFailureWorkflowFixture(t, "cfg-correlation")
			tc.mutate(&fixture)

			err := fixture.service.Handle(context.Background(), fixture.msg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			assertConfigureFailureResult(t, fixture.client, fixture.msg.Target, fixture.msg.UUID, fixture.msg.RPCID, tc.code)
		})
	}
}

func newFailureWorkflowFixture(t *testing.T, uuid string) phase3WorkflowFixture {
	t.Helper()
	return newPhase3WorkflowFixture(t, uuid)
}

func assertConfigureFailurePublishedWithoutSuccess(t *testing.T, client *testutil.FakeConfigureClient) {
	t.Helper()

	if !client.ContainsStatus("failure", "failed") {
		t.Fatal("expected failure status")
	}
	assertNoSuccessResult(t, client)
	for _, status := range client.Statuses() {
		if status.Status == "success" {
			t.Fatalf("unexpected success status: %+v", status)
		}
	}
}

func assertConfigureFailureResult(t *testing.T, client *testutil.FakeConfigureClient, target, uuid, rpcID, code string) agentcore.ResultEnvelope {
	t.Helper()

	result, ok := client.LastResult()
	if !ok {
		t.Fatal("expected failure result")
	}
	if result.Result != "failure" {
		t.Fatalf("result got=%q want=failure: %+v", result.Result, result)
	}
	if result.CommandType != "configure" {
		t.Fatalf("command_type got=%q want=configure: %+v", result.CommandType, result)
	}
	if result.ErrorCode != code {
		t.Fatalf("error_code got=%q want=%q: %+v", result.ErrorCode, code, result)
	}
	if result.Target != target || result.UUID != uuid || result.RPCID != rpcID {
		t.Fatalf("correlation got target=%q uuid=%q rpc_id=%q want target=%q uuid=%q rpc_id=%q", result.Target, result.UUID, result.RPCID, target, uuid, rpcID)
	}
	return result
}

func assertStatePreserved(t *testing.T, store *testutil.FakeStateStore, previousUUID, rejectedUUID string) {
	t.Helper()

	if got := store.SaveCalls(); got != 0 {
		t.Fatalf("save calls got=%d want=0", got)
	}
	if current := store.CurrentState(); current.AppliedUUID != previousUUID {
		t.Fatalf("current uuid got=%q want=%q", current.AppliedUUID, previousUUID)
	}
	for _, saved := range store.SavedStates() {
		if saved.AppliedUUID == rejectedUUID {
			t.Fatalf("rejected uuid %q was saved: %+v", rejectedUUID, saved)
		}
	}
}
