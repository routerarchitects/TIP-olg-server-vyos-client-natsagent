package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

type PlaceholderRTTYExecutor struct{}

func NewPlaceholderRTTYExecutor() *PlaceholderRTTYExecutor {
	return &PlaceholderRTTYExecutor{}
}

func (e *PlaceholderRTTYExecutor) Execute(ctx context.Context, msg agentcore.ActionCommand) (Output, error) {
	if ctx == nil {
		return Output{}, errors.New("execute action: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, fmt.Errorf("execute action: %w", err)
	}
	if msg.Target == "" {
		return Output{}, errors.New("execute action: target is empty")
	}
	if msg.Action != ActionRTTY {
		return Output{}, fmt.Errorf("execute action: unsupported action %q", msg.Action)
	}
	if msg.RPCID == "" {
		return Output{}, errors.New("execute action: rpc id is empty")
	}
	if len(msg.Payload) == 0 || !json.Valid(msg.Payload) {
		return Output{}, fmt.Errorf("%w: payload must be valid json", ErrInvalidActionPayload)
	}

	var payload RTTYPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return Output{}, fmt.Errorf("%w: parse rtty payload: %w", ErrInvalidActionPayload, err)
	}
	if err := payload.Validate(); err != nil {
		return Output{}, err
	}

	resPayload, err := json.Marshal(map[string]any{
		"error":            0,
		"text":             "Success",
		"executor":         "placeholder_rtty",
		"action":           ActionRTTY,
		"target":           msg.Target,
		"status":           "completed",
		"placeholder":      true,
		"received_payload": true,
		"id":               payload.ID,
	})
	if err != nil {
		return Output{}, fmt.Errorf("build placeholder rtty payload: %w", err)
	}

	return Output{
		Payload: resPayload,
		Message: "placeholder rtty action completed",
	}, nil
}
