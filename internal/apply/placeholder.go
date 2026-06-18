package apply

import (
	"context"
	"errors"
	"fmt"

	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
)

// Placeholder is a no-op apply engine for Phase 3 workflow checks.
type Placeholder struct{}

// NewPlaceholder creates a placeholder apply engine.
func NewPlaceholder() *Placeholder {
	return &Placeholder{}
}

// Apply validates rendered output and returns success without touching VyOS.
func (p *Placeholder) Apply(ctx context.Context, rendered renderer.Output) error {
	if ctx == nil {
		return errors.New("apply: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if rendered.UUID == "" {
		return errors.New("apply: rendered uuid is empty")
	}
	if rendered.Text == "" {
		return errors.New("apply: rendered text is empty")
	}
	return nil
}
