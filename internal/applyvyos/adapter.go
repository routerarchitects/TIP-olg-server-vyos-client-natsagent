package applyvyos

import (
	"context"
	"errors"
	"fmt"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	vyosapply "github.com/Telecominfraproject/olg-renderer-vyos/apply"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
)

type Backend interface {
	Apply(ctx context.Context, input vyosapply.Input) (vyosapply.Result, error)
}

type Preparer interface {
	Prepare(ctx context.Context, input vyosapply.Input) (vyosapply.Plan, error)
}

type Adapter struct {
	backend Backend
	logger  agentcore.Logger
	debug   DebugLogging
}

type Option func(*Adapter)

type DebugLogging struct {
	LogRendered  bool
	LogApplyPlan bool
}

func WithLogger(logger agentcore.Logger) Option {
	return func(a *Adapter) {
		a.logger = logger
	}
}

func WithDebugLogging(debug DebugLogging) Option {
	return func(a *Adapter) {
		a.debug = debug
	}
}

func New(saveAfterCommit bool, opts ...Option) (*Adapter, error) {
	backend, err := vyosapply.New(vyosapply.WithSaveAfterCommit(saveAfterCommit))
	if err != nil {
		return nil, fmt.Errorf("create vyos apply engine: %w", err)
	}
	return NewWithBackend(backend, opts...)
}

func NewWithBackend(backend Backend, opts ...Option) (*Adapter, error) {
	if backend == nil {
		return nil, errors.New("vyos apply backend is required")
	}
	a := &Adapter{backend: backend}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	return a, nil
}

func (a *Adapter) Apply(ctx context.Context, rendered renderer.Output) error {
	if a == nil || a.backend == nil {
		return errors.New("vyos apply adapter is not initialized")
	}

	input := BuildInput(rendered)
	a.logInfo("vyos apply input prepared",
		"target", input.Target,
		"uuid", input.ConfigUUID,
		"desired_size_bytes", len(input.DesiredCommands),
		"desired_command_count", countNonEmptyLines(input.DesiredCommands),
	)
	if a.debug.LogRendered {
		a.logDebug("vyos apply desired commands prepared",
			"target", input.Target,
			"uuid", input.ConfigUUID,
			"desired_commands", input.DesiredCommands,
		)
	}
	if preparer, ok := a.backend.(Preparer); ok {
		plan, err := preparer.Prepare(ctx, input)
		if err != nil {
			a.logWarn("failed to prepare vyos apply plan", "error", err)
		} else {
			a.logInfo("vyos apply plan prepared",
				"target", plan.Target,
				"uuid", plan.ConfigUUID,
				"delete_count", len(plan.DeleteCommands),
				"set_count", len(plan.SetCommands),
				"commit", plan.Commit,
				"save", plan.Save,
			)
			if a.debug.LogApplyPlan {
				a.logDebug("vyos apply plan command arrays prepared",
					"target", plan.Target,
					"uuid", plan.ConfigUUID,
					"delete_commands", plan.DeleteCommands,
					"set_commands", plan.SetCommands,
					"commit", plan.Commit,
					"save", plan.Save,
				)
			}
		}
	}
	if _, err := a.backend.Apply(ctx, input); err != nil {
		return fmt.Errorf("vyos apply failed: %w", err)
	}
	return nil
}

func BuildInput(rendered renderer.Output) vyosapply.Input {
	return vyosapply.Input{
		Target:          rendered.Target,
		ConfigUUID:      rendered.UUID,
		DesiredCommands: rendered.Text,
	}
}

func (a *Adapter) logInfo(msg string, kv ...any) {
	if a == nil || a.logger == nil {
		return
	}
	a.logger.Info(msg, kv...)
}

func (a *Adapter) logDebug(msg string, kv ...any) {
	if a == nil || a.logger == nil {
		return
	}
	a.logger.Debug(msg, kv...)
}

func (a *Adapter) logWarn(msg string, kv ...any) {
	if a == nil || a.logger == nil {
		return
	}
	a.logger.Warn(msg, kv...)
}

func countNonEmptyLines(text string) int {
	count := 0
	inLine := false
	for _, r := range text {
		switch r {
		case '\n', '\r':
			if inLine {
				count++
				inLine = false
			}
		case ' ', '\t':
			continue
		default:
			inLine = true
		}
	}
	if inLine {
		count++
	}
	return count
}
