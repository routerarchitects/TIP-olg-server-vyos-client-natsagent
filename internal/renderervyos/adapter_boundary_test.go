package renderervyos

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	vyosrenderer "github.com/routerarchitects/olg-renderer-vyos/renderer"
	internalrenderer "github.com/routerarchitects/olg-server-vyos-client-natagent/internal/renderer"
)

type adapterBoundaryRendererBackend struct {
	out         vyosrenderer.Output
	err         error
	mutateInput bool

	calls  int
	inputs []vyosrenderer.Input
}

func (f *adapterBoundaryRendererBackend) Render(ctx context.Context, input vyosrenderer.Input) (vyosrenderer.Output, error) {
	f.calls++
	if f.mutateInput && len(input.PayloadJSON) > 0 {
		input.PayloadJSON[0] = '['
	}
	f.inputs = append(f.inputs, cloneRendererInput(input))
	if f.err != nil {
		return vyosrenderer.Output{}, f.err
	}
	return f.out, nil
}

/*
TC-RENDERER-ADAPTER-001
Type: Positive
Title: Minimal payload maps correctly
Summary:
Runs the VyOS renderer adapter against a fake renderer backend with a
minimal desired config payload. The adapter should pass target, UUID,
and payload bytes through to the backend and map backend output back to
the internal renderer output shape.

Validates:
  - renderer backend is called once
  - target, UUID, and payload are preserved
  - rendered output text is returned
*/
func TestRendererAdapterMinimalPayloadMapsCorrectly(t *testing.T) {
	backend := &adapterBoundaryRendererBackend{
		out: vyosrenderer.Output{
			Target:       "vyos",
			ConfigUUID:   "cfg-minimal",
			RenderedText: "set interfaces ethernet eth0 address dhcp\n",
		},
	}
	adapter := newRendererBoundaryAdapter(t, backend)

	payload := json.RawMessage(`{"interfaces":[]}`)
	out, err := adapter.Render(context.Background(), rendererBoundaryDesired("vyos", "cfg-minimal", payload))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	input := onlyRendererInput(t, backend)
	if input.Target != "vyos" || input.ConfigUUID != "cfg-minimal" {
		t.Fatalf("input identity mismatch: %+v", input)
	}
	if string(input.PayloadJSON) != string(payload) {
		t.Fatalf("payload got=%s want=%s", string(input.PayloadJSON), string(payload))
	}
	if out.Target != "vyos" || out.UUID != "cfg-minimal" || out.Text != backend.out.RenderedText {
		t.Fatalf("output mapping mismatch: %+v", out)
	}
}

/*
TC-RENDERER-ADAPTER-002
Type: Load
Title: Large payload maps correctly
Summary:
Passes a deterministic large JSON payload through the adapter. The
adapter should not truncate or corrupt the payload before handing it to
the renderer backend.

Validates:
  - large payload is passed intact
  - renderer backend is called once
  - rendered output is returned
*/
func TestRendererAdapterLargePayloadMapsCorrectly(t *testing.T) {
	backend := &adapterBoundaryRendererBackend{
		out: vyosrenderer.Output{
			Target:       "vyos-large",
			ConfigUUID:   "cfg-large",
			RenderedText: "set system host-name large\n",
		},
	}
	adapter := newRendererBoundaryAdapter(t, backend)

	payload := largeRendererPayload()
	out, err := adapter.Render(context.Background(), rendererBoundaryDesired("vyos-large", "cfg-large", payload))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	input := onlyRendererInput(t, backend)
	if string(input.PayloadJSON) != string(payload) {
		t.Fatalf("large payload was not preserved")
	}
	if out.Text != backend.out.RenderedText {
		t.Fatalf("rendered output got=%q want=%q", out.Text, backend.out.RenderedText)
	}
}

/*
TC-RENDERER-ADAPTER-003
Type: Negative
Title: Invalid payload returns error
Summary:
Passes malformed desired config JSON to the adapter. BuildInput owns
payload decoding before the backend is called, so invalid JSON should
fail immediately and return no rendered output.

Validates:
  - adapter returns an error
  - renderer backend is not called
  - successful rendered output is empty
*/
func TestRendererAdapterInvalidPayloadReturnsError(t *testing.T) {
	backend := &adapterBoundaryRendererBackend{}
	adapter := newRendererBoundaryAdapter(t, backend)

	out, err := adapter.Render(context.Background(), rendererBoundaryDesired("vyos", "cfg-invalid", json.RawMessage(`{"interfaces":`)))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if backend.calls != 0 {
		t.Fatalf("backend calls got=%d want=0", backend.calls)
	}
	if out != (rendererOutputZero()) {
		t.Fatalf("rendered output got=%+v want zero", out)
	}
}

/*
TC-RENDERER-ADAPTER-004
Type: Positive
Title: Preserves UUID
Summary:
Checks that desired config UUID is passed to the renderer backend and
that backend output UUID is mapped back to internal renderer output.

Validates:
  - input UUID is unchanged
  - output UUID is unchanged
*/
func TestRendererAdapterPreservesUUID(t *testing.T) {
	backend := &adapterBoundaryRendererBackend{
		out: vyosrenderer.Output{
			Target:       "vyos",
			ConfigUUID:   "cfg-preserved",
			RenderedText: "set system host-name uuid\n",
		},
	}
	adapter := newRendererBoundaryAdapter(t, backend)

	out, err := adapter.Render(context.Background(), rendererBoundaryDesired("vyos", "cfg-preserved", json.RawMessage(`{"interfaces":[]}`)))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if got := onlyRendererInput(t, backend).ConfigUUID; got != "cfg-preserved" {
		t.Fatalf("input UUID got=%q want=cfg-preserved", got)
	}
	if out.UUID != "cfg-preserved" {
		t.Fatalf("output UUID got=%q want=cfg-preserved", out.UUID)
	}
}

/*
TC-RENDERER-ADAPTER-005
Type: Positive
Title: Preserves target
Summary:
Checks that desired config target is passed to the renderer backend and
that backend output target is mapped back to internal renderer output.

Validates:
  - input target is unchanged
  - output target is unchanged
*/
func TestRendererAdapterPreservesTarget(t *testing.T) {
	backend := &adapterBoundaryRendererBackend{
		out: vyosrenderer.Output{
			Target:       "vyos-edge-01",
			ConfigUUID:   "cfg-target",
			RenderedText: "set system host-name target\n",
		},
	}
	adapter := newRendererBoundaryAdapter(t, backend)

	out, err := adapter.Render(context.Background(), rendererBoundaryDesired("vyos-edge-01", "cfg-target", json.RawMessage(`{"interfaces":[]}`)))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if got := onlyRendererInput(t, backend).Target; got != "vyos-edge-01" {
		t.Fatalf("input target got=%q want=vyos-edge-01", got)
	}
	if out.Target != "vyos-edge-01" {
		t.Fatalf("output target got=%q want=vyos-edge-01", out.Target)
	}
}

/*
TC-RENDERER-ADAPTER-006
Type: Negative
Title: Propagates renderer error
Summary:
Runs the adapter with a fake renderer backend that returns an error.
The adapter should preserve the backend failure and return no successful
rendered output.

Validates:
  - backend render error is returned
  - error is wrapped with adapter context
  - output is empty
*/
func TestRendererAdapterPropagatesRendererError(t *testing.T) {
	backendErr := errors.New("renderer backend failed")
	backend := &adapterBoundaryRendererBackend{err: backendErr}
	adapter := newRendererBoundaryAdapter(t, backend)

	out, err := adapter.Render(context.Background(), rendererBoundaryDesired("vyos", "cfg-error", json.RawMessage(`{"interfaces":[]}`)))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "vyos render failed") || !strings.Contains(err.Error(), backendErr.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != (rendererOutputZero()) {
		t.Fatalf("rendered output got=%+v want zero", out)
	}
	if backend.calls != 1 {
		t.Fatalf("backend calls got=%d want=1", backend.calls)
	}
}

/*
TC-RENDERER-ADAPTER-007
Type: Safety
Title: Does not mutate payload
Summary:
Runs the adapter against a fake backend that mutates the payload buffer
it receives. The original desired config payload should remain unchanged
because the adapter clones payload bytes while building backend input.

Validates:
  - original desired payload bytes remain unchanged
  - adapter survives backend-side input mutation
*/
func TestRendererAdapterDoesNotMutatePayload(t *testing.T) {
	backend := &adapterBoundaryRendererBackend{
		mutateInput: true,
		out: vyosrenderer.Output{
			Target:       "vyos",
			ConfigUUID:   "cfg-mutation",
			RenderedText: "set system host-name mutation\n",
		},
	}
	adapter := newRendererBoundaryAdapter(t, backend)

	payload := json.RawMessage(`{"interfaces":[]}`)
	before := append(json.RawMessage(nil), payload...)
	if _, err := adapter.Render(context.Background(), rendererBoundaryDesired("vyos", "cfg-mutation", payload)); err != nil {
		t.Fatalf("render: %v", err)
	}

	if string(payload) != string(before) {
		t.Fatalf("payload mutated got=%s want=%s", string(payload), string(before))
	}
}

func newRendererBoundaryAdapter(t *testing.T, backend Backend) *Adapter {
	t.Helper()

	adapter, err := NewWithBackend(backend)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return adapter
}

func rendererBoundaryDesired(target, uuid string, payload json.RawMessage) agentcore.StoredDesiredConfig {
	return agentcore.StoredDesiredConfig{
		Record: agentcore.DesiredConfigRecord{
			Target:  target,
			UUID:    uuid,
			Payload: payload,
		},
	}
}

func onlyRendererInput(t *testing.T, backend *adapterBoundaryRendererBackend) vyosrenderer.Input {
	t.Helper()

	if backend.calls != 1 {
		t.Fatalf("backend calls got=%d want=1", backend.calls)
	}
	if len(backend.inputs) != 1 {
		t.Fatalf("backend input count got=%d want=1", len(backend.inputs))
	}
	return backend.inputs[0]
}

func cloneRendererInput(in vyosrenderer.Input) vyosrenderer.Input {
	in.PayloadJSON = append(json.RawMessage(nil), in.PayloadJSON...)
	return in
}

func largeRendererPayload() json.RawMessage {
	var b strings.Builder
	b.WriteString(`{"interfaces":[`)
	for i := 0; i < 256; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"name":"eth`)
		b.WriteString(string(rune('0' + (i % 10))))
		b.WriteString(`","description":"`)
		b.WriteString(strings.Repeat("x", 16))
		b.WriteString(`"}`)
	}
	b.WriteString(`]}`)
	return json.RawMessage(b.String())
}

func rendererOutputZero() internalrenderer.Output {
	return internalrenderer.Output{}
}
