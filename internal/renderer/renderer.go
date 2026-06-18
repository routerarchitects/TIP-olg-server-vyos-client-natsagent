package renderer

import (
	"context"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

// Output is rendered configure content for apply.
type Output struct {
	Target string
	UUID   string
	Text   string
}

// Renderer converts desired config into rendered target-specific text.
type Renderer interface {
	Render(ctx context.Context, desired agentcore.StoredDesiredConfig) (Output, error)
}
