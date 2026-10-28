package actions

import "errors"

const (
	ActionTrace = "trace"
	ActionRTTY  = "rtty"
)

var (
	ErrInvalidActionPayload = errors.New("invalid action payload")
)
