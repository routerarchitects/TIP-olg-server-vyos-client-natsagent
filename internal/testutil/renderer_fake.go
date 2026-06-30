package testutil

import (
	"context"
	"sync"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
)

// FakeRenderer is a controllable renderer test double for configure tests.
type FakeRenderer struct {
	Output    renderer.Output
	UseOutput bool
	Err       error
	Validate  func(agentcore.StoredDesiredConfig) error
	Events    *EventRecorder

	mu     sync.Mutex
	calls  int
	inputs []agentcore.StoredDesiredConfig
}

func (f *FakeRenderer) Render(ctx context.Context, desired agentcore.StoredDesiredConfig) (renderer.Output, error) {
	if f.Events != nil {
		f.Events.Record("render")
	}

	f.mu.Lock()
	f.calls++
	f.inputs = append(f.inputs, cloneDesired(desired))
	validate := f.Validate
	err := f.Err
	out := f.Output
	useOutput := f.UseOutput || out != (renderer.Output{})
	f.mu.Unlock()

	if validate != nil {
		if err := validate(desired); err != nil {
			return renderer.Output{}, err
		}
	}
	if err != nil {
		return renderer.Output{}, err
	}
	if useOutput {
		return out, nil
	}
	return renderer.Output{
		Target: desired.Record.Target,
		UUID:   desired.Record.UUID,
		Text:   "# rendered by FakeRenderer\n",
	}, nil
}

func (f *FakeRenderer) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *FakeRenderer) Inputs() []agentcore.StoredDesiredConfig {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]agentcore.StoredDesiredConfig, len(f.inputs))
	for i, input := range f.inputs {
		out[i] = cloneDesired(input)
	}
	return out
}

func (f *FakeRenderer) LastInput() (agentcore.StoredDesiredConfig, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.inputs) == 0 {
		return agentcore.StoredDesiredConfig{}, false
	}
	return cloneDesired(f.inputs[len(f.inputs)-1]), true
}

func (f *FakeRenderer) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = 0
	f.inputs = nil
}

func cloneDesired(in agentcore.StoredDesiredConfig) agentcore.StoredDesiredConfig {
	in.Record.Payload = cloneRawMessage(in.Record.Payload)
	return in
}
