package apply

import (
	"context"

	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
)

// Engine applies rendered config for a target.
type Engine interface {
	Apply(ctx context.Context, rendered renderer.Output) error
}
