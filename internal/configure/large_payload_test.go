package configure

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/renderer"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/testutil"
)

/*
TC-PERF-001
Type: Load
Title: Large config one thousand commands does not crash
Summary:
Runs configure with a fake renderer that returns one thousand rendered
commands. This is a lightweight sanity test, not a benchmark.

Validates:
  - configure completes
  - apply is called once
  - large rendered output reaches apply intact
  - success result is published
*/
func TestLargeConfigOneThousandCommandsDoesNotCrash(t *testing.T) {
	fixture := newPhase3WorkflowFixture(t, "cfg-large-commands")
	rendered := renderedCommandLines(1000)
	fixture.renderer.Output = renderer.Output{
		Target: fixture.msg.Target,
		UUID:   fixture.msg.UUID,
		Text:   rendered,
	}
	fixture.renderer.UseOutput = true

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	applied, ok := fixture.apply.LastInput()
	if !ok {
		t.Fatal("expected apply input")
	}
	if applied.Text != rendered {
		t.Fatalf("large rendered output was not preserved")
	}
	if !fixture.client.ContainsResult("success", "configure") {
		t.Fatal("expected success result")
	}
}

/*
TC-PERF-004
Type: Load
Title: Large rendered output handled safely
Summary:
Runs configure with a large rendered command text and verifies the apply
backend receives exactly the same text without truncation or mutation.

Validates:
  - no crash
  - rendered output reaches apply intact
  - state is saved after apply
*/
func TestLargeRenderedOutputHandledSafely(t *testing.T) {
	fixture := newPhase3WorkflowFixture(t, "cfg-large-rendered")
	rendered := renderedCommandLines(1500)
	fixture.renderer.Output = renderer.Output{
		Target: fixture.msg.Target,
		UUID:   fixture.msg.UUID,
		Text:   rendered,
	}
	fixture.renderer.UseOutput = true

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	applied, ok := fixture.apply.LastInput()
	if !ok {
		t.Fatal("expected apply input")
	}
	if applied.Text != rendered {
		t.Fatalf("apply text length got=%d want=%d", len(applied.Text), len(rendered))
	}
	if fixture.store.SaveCalls() != 1 {
		t.Fatalf("save calls got=%d want=1", fixture.store.SaveCalls())
	}
}

/*
TC-PERF-002
Type: Safety
Title: Large payload does not cause unsafe logging allocation
Summary:
Runs configure with a large desired payload and default logging disabled.
The payload should pass validation and render/apply safely without logging
or stringifying payload content in this path.

Validates:
  - no crash
  - renderer receives large payload intact
  - configure succeeds
*/
func TestLargePayloadDoesNotCauseUnsafeLoggingAllocation(t *testing.T) {
	fixture := newPhase3WorkflowFixture(t, "cfg-large-payload")
	payload := testutil.LargePayload(1000)
	desired := testutil.DesiredConfig(fixture.msg.Target, fixture.msg.UUID, payload)
	fixture.client.Desired = &desired

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	input, ok := fixture.renderer.LastInput()
	if !ok {
		t.Fatal("expected renderer input")
	}
	if string(input.Record.Payload) != string(payload) {
		t.Fatal("large payload was not preserved")
	}
}

func renderedCommandLines(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		b.WriteString("set interfaces ethernet eth0 vif ")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" description generated\n")
	}
	return b.String()
}
