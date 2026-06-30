package wire

import (
	"encoding/json"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

// Version is the wire protocol version used across envelopes.
const Version = "1.0"

// BuildStatus creates a standard StatusEnvelope.
func BuildStatus(rpcID, target, uuid, status, stage, message string, timestamp time.Time) agentcore.StatusEnvelope {
	return agentcore.StatusEnvelope{
		Version:   Version,
		RPCID:     rpcID,
		Target:    target,
		UUID:      uuid,
		Status:    status,
		Stage:     stage,
		Message:   message,
		Timestamp: timestamp,
	}
}

// BuildResult creates a standard ResultEnvelope.
func BuildResult(rpcID, target, cmdType, uuid, action, result, errCode, message string, payload json.RawMessage, timestamp time.Time) agentcore.ResultEnvelope {
	return agentcore.ResultEnvelope{
		Version:     Version,
		RPCID:       rpcID,
		Target:      target,
		CommandType: cmdType,
		UUID:        uuid,
		Action:      action,
		Result:      result,
		ErrorCode:   errCode,
		Message:     message,
		Payload:     payload,
		Timestamp:   timestamp,
	}
}
