package applyvyos

import (
	"context"
	"errors"
	"strings"
	"testing"

	vyosapply "github.com/routerarchitects/olg-renderer-vyos/apply"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/renderer"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/testutil"
)

type applyBoundaryBackend struct {
	err error

	calls  int
	inputs []vyosapply.Input
}

func (f *applyBoundaryBackend) Apply(ctx context.Context, input vyosapply.Input) (vyosapply.Result, error) {
	f.calls++
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return vyosapply.Result{}, f.err
	}
	return vyosapply.Result{Target: input.Target, ConfigUUID: input.ConfigUUID, Applied: true}, nil
}

/*
TC-APPLY-ADAPTER-001
Type: Positive
Title: Rendered output maps to apply input
Summary:
Runs the VyOS apply adapter against a fake apply backend.
The adapter should map internal rendered output to the external
vyosapply input without changing target, UUID, or rendered commands.

Validates:
  - backend Apply is called once
  - target, UUID, and rendered commands are preserved
*/
func TestApplyAdapterRenderedOutputMapsToApplyInput(t *testing.T) {
	backend := &applyBoundaryBackend{}
	adapter := newApplyBoundaryAdapter(t, backend)

	rendered := applyBoundaryRendered()
	if err := adapter.Apply(context.Background(), rendered); err != nil {
		t.Fatalf("apply: %v", err)
	}

	input := onlyApplyBoundaryInput(t, backend)
	assertApplyInputMatchesRendered(t, input, rendered)
}

/*
TC-APPLY-ADAPTER-002
Type: Positive
Title: Calls Prepare when supported
Summary:
Runs the adapter with a backend that supports both Prepare and Apply.
The adapter should call Prepare once before Apply.

Validates:
  - Prepare is called exactly once
  - Apply is called exactly once
  - order is prepare then apply
*/
func TestApplyAdapterCallsPrepareWhenSupported(t *testing.T) {
	events := &testutil.EventRecorder{}
	backend := &testutil.FakeApplyBackend{Events: events}
	adapter := newApplyBoundaryAdapter(t, backend)

	if err := adapter.Apply(context.Background(), applyBoundaryRendered()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if backend.PrepareCalls() != 1 || backend.ApplyCalls() != 1 {
		t.Fatalf("calls got prepare=%d apply=%d want prepare=1 apply=1", backend.PrepareCalls(), backend.ApplyCalls())
	}
	assertApplyEvents(t, events, []string{"prepare", "apply"})
}

/*
TC-APPLY-ADAPTER-003
Type: Safety
Title: Logs plan fields safely
Summary:
Runs the adapter with a logger and a fake prepared plan containing raw
command strings. Default info logs should include only safe plan summary
fields, not rendered command arrays.

Validates:
  - plan summary fields are logged
  - raw delete/set commands are not logged by default
*/
func TestApplyAdapterLogsPlanFieldsSafely(t *testing.T) {
	logs := &testutil.LogCapture{}
	backend := &testutil.FakeApplyBackend{
		UsePlan: true,
		Plan: vyosapply.Plan{
			Target:         "vyos",
			ConfigUUID:     "cfg-apply",
			DeleteCommands: []string{"delete interfaces ethernet eth9"},
			SetCommands:    []string{"set system login user admin authentication plaintext-password secret"},
			Commit:         true,
			Save:           true,
		},
	}
	adapter := newApplyBoundaryAdapter(t, backend, WithLogger(logs))

	if err := adapter.Apply(context.Background(), applyBoundaryRendered()); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if !logs.Contains("delete_count") || !logs.Contains("set_count") || !logs.Contains("commit") || !logs.Contains("save") {
		t.Fatalf("expected safe plan summary fields in logs: %+v", logs.Entries())
	}
	assertLogDoesNotContain(t, logs, "delete interfaces ethernet eth9")
	assertLogDoesNotContain(t, logs, "plaintext-password secret")
}

/*
TC-APPLY-ADAPTER-004
Type: Positive
Title: Calls Apply exactly once
Summary:
Runs a successful apply through a fake backend. The adapter should not
duplicate backend Apply calls.

Validates:
  - Apply is called exactly once
*/
func TestApplyAdapterCallsApplyExactlyOnce(t *testing.T) {
	backend := &applyBoundaryBackend{}
	adapter := newApplyBoundaryAdapter(t, backend)

	if err := adapter.Apply(context.Background(), applyBoundaryRendered()); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if backend.calls != 1 {
		t.Fatalf("apply calls got=%d want=1", backend.calls)
	}
}

/*
TC-APPLY-ADAPTER-005
Type: Negative
Title: Propagates Prepare error
Summary:
Runs the adapter with a preparer that fails before apply.
The adapter should return the prepare error and must not call Apply.

Validates:
  - prepare error is returned with adapter context
  - Apply is not called
*/
func TestApplyAdapterPropagatesPrepareError(t *testing.T) {
	prepareErr := errors.New("prepare failed")
	backend := &testutil.FakeApplyBackend{PrepareErr: prepareErr}
	adapter := newApplyBoundaryAdapter(t, backend)

	err := adapter.Apply(context.Background(), applyBoundaryRendered())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "prepare vyos apply plan") || !strings.Contains(err.Error(), prepareErr.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.ApplyCalls() != 0 {
		t.Fatalf("apply calls got=%d want=0", backend.ApplyCalls())
	}
}

/*
TC-APPLY-ADAPTER-006
Type: Negative
Title: Propagates Apply error
Summary:
Runs the adapter with a backend whose Apply step fails after Prepare
succeeds. The adapter should return the apply error with context.

Validates:
  - Prepare is called once
  - Apply is called once
  - apply error is returned
*/
func TestApplyAdapterPropagatesApplyError(t *testing.T) {
	applyErr := errors.New("commit failed")
	backend := &testutil.FakeApplyBackend{ApplyErr: applyErr}
	adapter := newApplyBoundaryAdapter(t, backend)

	err := adapter.Apply(context.Background(), applyBoundaryRendered())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "vyos apply failed") || !strings.Contains(err.Error(), applyErr.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.PrepareCalls() != 1 || backend.ApplyCalls() != 1 {
		t.Fatalf("calls got prepare=%d apply=%d want prepare=1 apply=1", backend.PrepareCalls(), backend.ApplyCalls())
	}
}

/*
TC-APPLY-ADAPTER-007
Type: Safety
Title: Prepare does not mutate input for Apply
Summary:
Runs a backend whose Prepare path records a mutated version of its input.
Apply should still receive the original intended adapter input, and the
internal rendered output should remain unchanged.

Validates:
  - Apply receives the expected rendered commands
  - rendered output remains unchanged after Prepare
*/
func TestApplyAdapterPrepareDoesNotMutateInputForApply(t *testing.T) {
	backend := &testutil.FakeApplyBackend{MutateOnPrepare: true}
	adapter := newApplyBoundaryAdapter(t, backend)
	rendered := applyBoundaryRendered()
	originalText := rendered.Text

	if err := adapter.Apply(context.Background(), rendered); err != nil {
		t.Fatalf("apply: %v", err)
	}

	input, ok := backend.LastApplyInput()
	if !ok {
		t.Fatal("expected apply input")
	}
	assertApplyInputMatchesRendered(t, input, rendered)
	if rendered.Text != originalText {
		t.Fatalf("rendered text mutated got=%q want=%q", rendered.Text, originalText)
	}
}

/*
TC-APPLY-ADAPTER-008
Type: Safety
Title: Backend with Prepare and Apply uses correct input
Summary:
Runs a backend supporting both Prepare and Apply with validators on both
steps. The adapter should pass the same intended input to each step.

Validates:
  - Prepare receives the expected input
  - Apply receives the expected input
  - Prepare and Apply are each called once
*/
func TestApplyAdapterBackendWithBothPrepareAndApplyUsesCorrectInput(t *testing.T) {
	rendered := applyBoundaryRendered()
	backend := &testutil.FakeApplyBackend{
		ValidatePrepare: func(input vyosapply.Input) error {
			return validateApplyInputMatchesRendered(input, rendered)
		},
		ValidateApply: func(input vyosapply.Input) error {
			return validateApplyInputMatchesRendered(input, rendered)
		},
	}
	adapter := newApplyBoundaryAdapter(t, backend)

	if err := adapter.Apply(context.Background(), rendered); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if backend.PrepareCalls() != 1 || backend.ApplyCalls() != 1 {
		t.Fatalf("calls got prepare=%d apply=%d want prepare=1 apply=1", backend.PrepareCalls(), backend.ApplyCalls())
	}
	assertApplyInputMatchesRendered(t, mustLastPrepareInput(t, backend), rendered)
	assertApplyInputMatchesRendered(t, mustLastApplyInput(t, backend), rendered)
}

/*
TC-APPLY-ADAPTER-009
Type: Safety
Title: Does not apply when Prepare fails
Summary:
Runs a backend whose Prepare step fails. The adapter should stop before
the unsafe Apply step.

Validates:
  - Apply is not called after Prepare failure
  - prepare error is returned
*/
func TestApplyAdapterDoesNotApplyWhenPrepareFails(t *testing.T) {
	backend := &testutil.FakeApplyBackend{PrepareErr: errors.New("plan rejected")}
	adapter := newApplyBoundaryAdapter(t, backend)

	err := adapter.Apply(context.Background(), applyBoundaryRendered())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if backend.ApplyCalls() != 0 {
		t.Fatalf("apply calls got=%d want=0", backend.ApplyCalls())
	}
}

/*
TC-APPLY-ADAPTER-010
Type: Negative
Title: Handles empty plan safely
Summary:
Runs a preparer that returns the zero-value plan. The adapter should not
panic or stop solely because the plan contains no command arrays; Apply
still receives the rendered input.

Validates:
  - empty plan does not panic
  - Apply is still called with the expected input
*/
func TestApplyAdapterHandlesNilOrEmptyPlanSafely(t *testing.T) {
	backend := &testutil.FakeApplyBackend{
		UsePlan: true,
		Plan:    vyosapply.Plan{},
	}
	adapter := newApplyBoundaryAdapter(t, backend)
	rendered := applyBoundaryRendered()

	if err := adapter.Apply(context.Background(), rendered); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if backend.PrepareCalls() != 1 || backend.ApplyCalls() != 1 {
		t.Fatalf("calls got prepare=%d apply=%d want prepare=1 apply=1", backend.PrepareCalls(), backend.ApplyCalls())
	}
	assertApplyInputMatchesRendered(t, mustLastApplyInput(t, backend), rendered)
}

/*
TC-APPLY-ADAPTER-011
Type: Safety
Title: Does not log rendered commands by default
Summary:
Runs the adapter with a logger and sensitive-looking rendered commands.
Default logging should report safe metadata only and should not include
the raw rendered command text.

Validates:
  - rendered commands are absent from default logs
  - no debug rendered logging is enabled
*/
func TestApplyAdapterDoesNotLogRenderedCommandsByDefault(t *testing.T) {
	logs := &testutil.LogCapture{}
	backend := &applyBoundaryBackend{}
	adapter := newApplyBoundaryAdapter(t, backend, WithLogger(logs))
	rendered := renderer.Output{
		Target: "vyos",
		UUID:   "cfg-secret",
		Text:   "set system login user admin authentication plaintext-password secret\n",
	}

	if err := adapter.Apply(context.Background(), rendered); err != nil {
		t.Fatalf("apply: %v", err)
	}

	assertLogDoesNotContain(t, logs, rendered.Text)
	assertLogDoesNotContain(t, logs, "plaintext-password secret")
}

func newApplyBoundaryAdapter(t *testing.T, backend Backend, opts ...Option) *Adapter {
	t.Helper()

	adapter, err := NewWithBackend(backend, opts...)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return adapter
}

func applyBoundaryRendered() renderer.Output {
	return renderer.Output{
		Target: "vyos",
		UUID:   "cfg-apply",
		Text:   "delete interfaces ethernet eth9\nset interfaces ethernet eth0 address dhcp\n",
	}
}

func onlyApplyBoundaryInput(t *testing.T, backend *applyBoundaryBackend) vyosapply.Input {
	t.Helper()

	if backend.calls != 1 {
		t.Fatalf("apply calls got=%d want=1", backend.calls)
	}
	if len(backend.inputs) != 1 {
		t.Fatalf("input count got=%d want=1", len(backend.inputs))
	}
	return backend.inputs[0]
}

func assertApplyEvents(t *testing.T, events *testutil.EventRecorder, want []string) {
	t.Helper()

	got := events.Events()
	if len(got) != len(want) {
		t.Fatalf("events got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event[%d] got=%q want=%q; all events=%v", i, got[i], want[i], got)
		}
	}
}

func assertApplyInputMatchesRendered(t *testing.T, input vyosapply.Input, rendered renderer.Output) {
	t.Helper()

	if err := validateApplyInputMatchesRendered(input, rendered); err != nil {
		t.Fatal(err)
	}
}

func validateApplyInputMatchesRendered(input vyosapply.Input, rendered renderer.Output) error {
	if input.Target != rendered.Target {
		return errors.New("target mismatch")
	}
	if input.ConfigUUID != rendered.UUID {
		return errors.New("uuid mismatch")
	}
	if input.DesiredCommands != rendered.Text {
		return errors.New("desired commands mismatch")
	}
	return nil
}

func mustLastPrepareInput(t *testing.T, backend *testutil.FakeApplyBackend) vyosapply.Input {
	t.Helper()

	input, ok := backend.LastPrepareInput()
	if !ok {
		t.Fatal("expected prepare input")
	}
	return input
}

func mustLastApplyInput(t *testing.T, backend *testutil.FakeApplyBackend) vyosapply.Input {
	t.Helper()

	input, ok := backend.LastApplyInput()
	if !ok {
		t.Fatal("expected apply input")
	}
	return input
}

func assertLogDoesNotContain(t *testing.T, logs *testutil.LogCapture, text string) {
	t.Helper()

	if logs.Contains(text) {
		t.Fatalf("unexpected log content %q in entries: %+v", text, logs.Entries())
	}
}
