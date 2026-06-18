package renderer

import (
	"context"
	"errors"
	"fmt"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

// Placeholder is a deterministic mock renderer for Phase 3.
type Placeholder struct{}

// NewPlaceholder creates a placeholder renderer.
func NewPlaceholder() *Placeholder {
	return &Placeholder{}
}

// Render returns deterministic mock config text for workflow validation.
func (p *Placeholder) Render(ctx context.Context, desired agentcore.StoredDesiredConfig) (Output, error) {
	if ctx == nil {
		return Output{}, errors.New("render: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, fmt.Errorf("render: %w", err)
	}
	if desired.Record.Target == "" {
		return Output{}, errors.New("render: desired target is empty")
	}
	if desired.Record.UUID == "" {
		return Output{}, errors.New("render: desired uuid is empty")
	}

	return Output{
		Target: desired.Record.Target,
		UUID:   desired.Record.UUID,
		Text: "# placeholder vyos config\n" +
			"# target: " + desired.Record.Target + "\n" +
			"# uuid: " + desired.Record.UUID + "\n",
	}, nil
}
