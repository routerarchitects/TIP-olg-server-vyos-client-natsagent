package configure

import (
	"context"
	"errors"
	"testing"

	"github.com/routerarchitects/nats-agent-core/agentcore"
)

/*
TC-IDEMPOTENCY-002
Type: Recovery
Title: Same UUID after renderer failure retries
Summary:
Runs configure once with renderer failure, then retries the same UUID after
the renderer succeeds. Since renderer failure does not checkpoint state, the
retry should render again and complete successfully.

Validates:
  - failed render does not save state
  - retry renders again
  - retry applies and saves requested UUID
*/
func TestSameUUIDAfterRendererFailureRetries(t *testing.T) {
	fixture := newPhase3WorkflowFixture(t, "cfg-idemp-render-retry")
	fixture.renderer.Err = errors.New("first render failed")

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected first attempt error, got nil")
	}
	if fixture.renderer.Calls() != 1 || fixture.apply.Calls() != 0 || fixture.store.SaveCalls() != 0 {
		t.Fatalf("first attempt calls got renderer=%d apply=%d save=%d want renderer=1 apply=0 save=0", fixture.renderer.Calls(), fixture.apply.Calls(), fixture.store.SaveCalls())
	}

	fixture.renderer.Err = nil
	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("retry handle: %v", err)
	}
	if fixture.renderer.Calls() != 2 || fixture.apply.Calls() != 1 || fixture.store.SaveCalls() != 1 {
		t.Fatalf("retry calls got renderer=%d apply=%d save=%d want renderer=2 apply=1 save=1", fixture.renderer.Calls(), fixture.apply.Calls(), fixture.store.SaveCalls())
	}
	saved, ok := fixture.store.LastSavedState()
	if !ok || saved.AppliedUUID != fixture.msg.UUID {
		t.Fatalf("saved state got=%+v ok=%v want uuid=%q", saved, ok, fixture.msg.UUID)
	}
}

/*
TC-IDEMPOTENCY-005
Type: Safety
Title: Duplicate configure events do not double apply after success
Summary:
Handles the same configure notification twice. After the first success
checkpoints the UUID, the duplicate event should publish already-in-sync
without rendering, applying, or saving a second time.

Validates:
  - apply call count remains one
  - state save count remains one
  - second event reports already_in_sync
*/
func TestDuplicateConfigureEventsDoNotDoubleApplyAfterSuccess(t *testing.T) {
	fixture := newPhase3WorkflowFixture(t, "cfg-idemp-duplicate")

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("duplicate handle: %v", err)
	}

	if fixture.apply.Calls() != 1 {
		t.Fatalf("apply calls got=%d want=1", fixture.apply.Calls())
	}
	if fixture.store.SaveCalls() != 1 {
		t.Fatalf("save calls got=%d want=1", fixture.store.SaveCalls())
	}
	if !fixture.client.ContainsStatus("success", "already_in_sync") {
		t.Fatal("expected already_in_sync status for duplicate event")
	}
}

/*
TC-IDEMPOTENCY-008
Type: Safety
Title: Retry does not publish duplicate success for same attempt
Summary:
Checks result stream behavior for a single successful attempt and for a
failure followed by retry. Each Handle attempt should publish at most one
success result, and the failed attempt should publish none.

Validates:
  - one success result for one successful attempt
  - failed attempt publishes no success result
  - retry publishes exactly one success result
*/
func TestRetryDoesNotPublishDuplicateSuccessForSameAttempt(t *testing.T) {
	t.Run("single success", func(t *testing.T) {
		fixture := newPhase3WorkflowFixture(t, "cfg-idemp-single-success")
		if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if got := countConfigureResults(fixture.client.Results(), "success"); got != 1 {
			t.Fatalf("success result count got=%d want=1", got)
		}
	})

	t.Run("failure then retry", func(t *testing.T) {
		fixture := newPhase3WorkflowFixture(t, "cfg-idemp-retry-success")
		fixture.apply.Errs = []error{errors.New("first apply failed"), nil}

		err := fixture.service.Handle(context.Background(), fixture.msg)
		if err == nil {
			t.Fatal("expected first attempt error, got nil")
		}
		if got := countConfigureResults(fixture.client.Results(), "success"); got != 0 {
			t.Fatalf("success result count after failure got=%d want=0", got)
		}

		if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
			t.Fatalf("retry handle: %v", err)
		}
		if got := countConfigureResults(fixture.client.Results(), "success"); got != 1 {
			t.Fatalf("success result count after retry got=%d want=1", got)
		}
	})
}

func countConfigureResults(results []agentcore.ResultEnvelope, result string) int {
	count := 0
	for _, got := range results {
		if got.Result == result && got.CommandType == "configure" {
			count++
		}
	}
	return count
}
