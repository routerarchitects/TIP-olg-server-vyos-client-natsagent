package applyvyos

import (
	"context"
	"errors"
	"strings"
	"testing"

	vyosapply "github.com/Telecominfraproject/olg-renderer-vyos/apply"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
)

type fakeApplyBackend struct {
	input vyosapply.Input
	err   error
	calls int
}

func (f *fakeApplyBackend) Apply(ctx context.Context, input vyosapply.Input) (vyosapply.Result, error) {
	f.calls++
	f.input = input
	if f.err != nil {
		return vyosapply.Result{}, f.err
	}
	return vyosapply.Result{Target: input.Target, ConfigUUID: input.ConfigUUID, Applied: true}, nil
}

/*
TC-APPLY-VYOS-001
Type: Positive
Title: Build input maps rendered output
Summary:
Builds apply input from internal rendered output.
The adapter should pass target, config UUID, and rendered command
text to the external apply backend.

Validates:
  - target and config UUID are mapped
  - rendered commands become desired commands
*/
func TestBuildInputMapsRenderedOutput(t *testing.T) {
	input := BuildInput(renderer.Output{
		Target: "vyos",
		UUID:   "cfg-1",
		Text:   "set interfaces ethernet eth0 address dhcp\n",
	})

	if input.Target != "vyos" || input.ConfigUUID != "cfg-1" {
		t.Fatalf("identity mapping mismatch: %+v", input)
	}
	if input.DesiredCommands != "set interfaces ethernet eth0 address dhcp\n" {
		t.Fatalf("commands got=%q", input.DesiredCommands)
	}
}

/*
TC-APPLY-VYOS-002
Type: Positive
Title: Apply success returns nil
Summary:
Runs the adapter against a fake successful apply backend.
The adapter should call the backend once and return nil when apply
completes without error.

Validates:
  - backend apply is called once
  - input target and config UUID are mapped
  - successful apply returns nil
*/
func TestApplySuccessReturnsNil(t *testing.T) {
	backend := &fakeApplyBackend{}
	adapter, err := NewWithBackend(backend)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	err = adapter.Apply(context.Background(), renderer.Output{
		Target: "vyos",
		UUID:   "cfg-1",
		Text:   "set interfaces ethernet eth0 address dhcp\n",
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if backend.calls != 1 {
		t.Fatalf("apply calls got=%d want=1", backend.calls)
	}
	if backend.input.Target != "vyos" || backend.input.ConfigUUID != "cfg-1" {
		t.Fatalf("input mapping mismatch: %+v", backend.input)
	}
}

/*
TC-APPLY-VYOS-003
Type: Negative
Title: Apply returns wrapped backend error
Summary:
Runs the adapter against a fake backend that fails apply.
The adapter should return an error that preserves backend context
and identifies the apply stage.

Validates:
  - backend apply error is returned
  - error includes vyos apply failed context
*/
func TestApplyReturnsWrappedBackendError(t *testing.T) {
	backend := &fakeApplyBackend{err: errors.New("commit failed")}
	adapter, err := NewWithBackend(backend)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	err = adapter.Apply(context.Background(), renderer.Output{
		Target: "vyos",
		UUID:   "cfg-1",
		Text:   "set interfaces ethernet eth0 address dhcp\n",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "vyos apply failed") || !strings.Contains(err.Error(), "commit failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

/*
TC-APPLY-VYOS-004
Type: Positive
Title: Command count ignores blank lines
Summary:
Counts desired command text with blank lines and whitespace.
The helper should count only non-empty command lines for safe
metadata logging.

Validates:
  - non-empty command lines are counted
  - blank and whitespace-only lines are ignored
*/
func TestCountNonEmptyLinesIgnoresBlankLines(t *testing.T) {
	got := countNonEmptyLines("\nset a\n  \nset b\n\t\n")
	if got != 2 {
		t.Fatalf("line count got=%d want=2", got)
	}
}
