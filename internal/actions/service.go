package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/wire"
)

type Service struct {
	client    AgentCoreClient
	logger    agentcore.Logger
	now       func() time.Time
	enabled   map[string]struct{}
	executors map[string]Executor
}

type Dependencies struct {
	Client    AgentCoreClient
	Logger    agentcore.Logger
	Now       func() time.Time
	Enabled   []string
	Executors map[string]Executor
}

func NewService(deps Dependencies) (*Service, error) {
	if deps.Client == nil {
		return nil, errors.New("action service: client is required")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}

	enabled := make(map[string]struct{}, len(deps.Enabled))
	for _, action := range deps.Enabled {
		trimmed := strings.TrimSpace(action)
		if trimmed == "" {
			continue
		}
		enabled[trimmed] = struct{}{}
	}

	executors := make(map[string]Executor, len(deps.Executors))
	for action, exec := range deps.Executors {
		if strings.TrimSpace(action) == "" {
			continue
		}
		if exec == nil {
			return nil, fmt.Errorf("action service: executor %q is nil", action)
		}
		executors[action] = exec
	}
	if len(executors) == 0 {
		return nil, errors.New("action service: at least one executor is required")
	}

	return &Service{
		client:    deps.Client,
		logger:    deps.Logger,
		now:       deps.Now,
		enabled:   enabled,
		executors: executors,
	}, nil
}

func (s *Service) Handle(ctx context.Context, msg agentcore.ActionCommand) error {
	if ctx == nil {
		return errors.New("action handle: context is nil")
	}

	s.logInfo("action received", "target", msg.Target, "action", msg.Action, "rpc_id", msg.RPCID, "stage", "received", "status", "running")
	if err := s.publishStatus(ctx, msg, "running", "received", "action command received"); err != nil {
		return s.fail(ctx, msg, "status_publish_failed", "action processing failed", fmt.Errorf("publish action status received: %w", err))
	}

	if _, ok := s.enabled[msg.Action]; !ok {
		return s.fail(ctx, msg, "disabled_action", "action is not enabled", fmt.Errorf("action %q is not enabled", msg.Action))
	}

	executor, ok := s.executors[msg.Action]
	if !ok {
		return s.fail(ctx, msg, "unsupported_action", "action is not supported", fmt.Errorf("action %q has no executor", msg.Action))
	}

	s.logInfo("action executing", "target", msg.Target, "action", msg.Action, "rpc_id", msg.RPCID, "stage", "executing", "status", "running")
	if err := s.publishStatus(ctx, msg, "running", "executing", fmt.Sprintf("executing %s action", msg.Action)); err != nil {
		return s.fail(ctx, msg, "status_publish_failed", "action processing failed", fmt.Errorf("publish action status executing: %w", err))
	}

	output, err := executor.Execute(ctx, msg)
	if err != nil {
		code := "action_execute_failed"
		safeMessage := "action execution failed"
		if errors.Is(err, ErrInvalidActionPayload) {
			code = "invalid_action_payload"
			safeMessage = "invalid action payload"
		}
		return s.fail(ctx, msg, code, safeMessage, fmt.Errorf("execute action %q: %w", msg.Action, err))
	}

	s.logInfo("action completed", "target", msg.Target, "action", msg.Action, "rpc_id", msg.RPCID, "stage", "completed", "status", "success")
	if err := s.publishStatus(ctx, msg, "success", "completed", fmt.Sprintf("%s action completed", msg.Action)); err != nil {
		return s.fail(ctx, msg, "status_publish_failed", "action processing failed", fmt.Errorf("publish action status completed: %w", err))
	}

	s.logInfo("action result publishing", "target", msg.Target, "action", msg.Action, "rpc_id", msg.RPCID, "status", "success")
	if err := s.client.PublishResult(ctx, wire.BuildResult(
		msg.RPCID,
		msg.Target,
		"action",
		"",
		msg.Action,
		"success",
		"",
		output.Message,
		output.Payload,
		s.now().UTC(),
	)); err != nil {
		return s.fail(ctx, msg, "result_publish_failed", "failed to publish action result", fmt.Errorf("publish action success result: %w", err))
	}
	s.logInfo("action result published", "target", msg.Target, "action", msg.Action, "rpc_id", msg.RPCID, "status", "success")

	return nil
}

func (s *Service) fail(ctx context.Context, msg agentcore.ActionCommand, code, safeMessage string, originalErr error) error {
	s.logError("action failed", "target", msg.Target, "action", msg.Action, "rpc_id", msg.RPCID, "stage", "failed", "status", "failure", "error_code", code, "message", safeMessage)

	var statusErr error
	if err := s.publishStatus(ctx, msg, "failure", "failed", "action processing failed"); err != nil {
		statusErr = fmt.Errorf("publish action failure status: %w", err)
	}

	var resultErr error
	if err := s.client.PublishResult(ctx, wire.BuildResult(
		msg.RPCID,
		msg.Target,
		"action",
		"",
		msg.Action,
		"failure",
		code,
		safeMessage,
		nil,
		s.now().UTC(),
	)); err != nil {
		resultErr = fmt.Errorf("publish action failure result: %w", err)
	}

	return errors.Join(originalErr, statusErr, resultErr)
}

func (s *Service) publishStatus(ctx context.Context, msg agentcore.ActionCommand, status, stage, message string) error {
	return s.client.PublishStatus(ctx, wire.BuildStatus(
		msg.RPCID,
		msg.Target,
		"",
		status,
		stage,
		message,
		s.now().UTC(),
	))
}

func (s *Service) logInfo(msg string, kv ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Info(msg, kv...)
}

func (s *Service) logError(msg string, kv ...any) {
	if s.logger == nil {
		return
	}
	s.logger.Error(msg, kv...)
}
