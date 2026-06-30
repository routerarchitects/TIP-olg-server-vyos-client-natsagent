package configure

import (
	"context"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/state"
)

type AgentCoreClient interface {
	LoadDesiredConfig(ctx context.Context, target string) (*agentcore.StoredDesiredConfig, error)
	PublishStatus(ctx context.Context, msg agentcore.StatusEnvelope) error
	PublishResult(ctx context.Context, msg agentcore.ResultEnvelope) error
}

type StateStore interface {
	Load(ctx context.Context) (state.State, error)
	Save(ctx context.Context, st state.State) error
}

type Renderer interface {
	Render(ctx context.Context, desired agentcore.StoredDesiredConfig) (renderer.Output, error)
}

type ApplyEngine interface {
	Apply(ctx context.Context, rendered renderer.Output) error
}
