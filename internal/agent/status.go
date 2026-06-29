package agent

import (
	"context"
	"fmt"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

func (r *Runtime) publishStartupStatus(ctx context.Context) error {
	r.logInfo(
		"startup status publishing",
		"target", r.appConfig.Agent.Target,
		"stage", "startup",
		"status", "running",
	)

	msg := agentcore.StatusEnvelope{
		Version:   wireVersion,
		Target:    r.appConfig.Agent.Target,
		Status:    "running",
		Stage:     "startup",
		Message:   "vyos-nats-agent started",
		Timestamp: r.now().UTC(),
	}
	publishCtx, cancel := context.WithTimeout(context.Background(), startupCloseTimeout)
	defer cancel()
	if err := r.client.PublishStatus(publishCtx, msg); err != nil {
		return fmt.Errorf("publish startup status: %w", err)
	}

	r.logInfo(
		"startup status published",
		"target", r.appConfig.Agent.Target,
		"stage", "startup",
		"status", "running",
	)
	return nil
}
