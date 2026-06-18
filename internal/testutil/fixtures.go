package testutil

import (
	"encoding/json"
	"strings"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/renderer"
)

const (
	MinimalTarget = "vyos"
	MinimalUUID   = "cfg-test-1"
	MinimalRPCID  = "rpc-test-1"
)

func MinimalDesiredConfig() agentcore.StoredDesiredConfig {
	return MinimalRenderableDesiredConfig()
}

func MinimalPlaceholderDesiredConfig() agentcore.StoredDesiredConfig {
	return DesiredConfig(MinimalTarget, MinimalUUID, json.RawMessage(`{"interfaces":[],"services":{}}`))
}

func MinimalRenderableDesiredConfig() agentcore.StoredDesiredConfig {
	return DesiredConfig(MinimalTarget, MinimalUUID, json.RawMessage(`{
		"schema_name": "olg-ucentral",
		"schema_version": "4.2.0",
		"config": {
			"services": {
				"ssh": {"port": 22}
			},
			"interfaces": [
				{
					"name": "OLG_APPLY_SMOKE_TEST_Upstream",
					"role": "upstream",
					"ethernet": [
						{"select-ports": ["WAN*"]}
					],
					"ipv4": {
						"addressing": "dynamic"
					}
				}
			]
		}
	}`))
}

func DesiredConfig(target, uuid string, payload json.RawMessage) agentcore.StoredDesiredConfig {
	return agentcore.StoredDesiredConfig{
		Record: agentcore.DesiredConfigRecord{
			Target:  target,
			UUID:    uuid,
			Payload: cloneRawMessage(payload),
		},
	}
}

func MinimalConfigureNotification() agentcore.ConfigureNotification {
	return agentcore.ConfigureNotification{
		Version: "1.0",
		RPCID:   MinimalRPCID,
		Target:  MinimalTarget,
		UUID:    MinimalUUID,
	}
}

func MinimalActionCommand(action string) agentcore.ActionCommand {
	return agentcore.ActionCommand{
		Version: "1.0",
		RPCID:   MinimalRPCID,
		Target:  MinimalTarget,
		Action:  action,
	}
}

func MinimalRenderedOutput() renderer.Output {
	return renderer.Output{
		Target: MinimalTarget,
		UUID:   MinimalUUID,
		Text:   "# rendered by test fixture\n",
	}
}

func LargePayload(repetitions int) json.RawMessage {
	if repetitions < 1 {
		repetitions = 1
	}
	return json.RawMessage(`{"interfaces":[` + strings.TrimRight(strings.Repeat(`{"name":"LAN","role":"downstream"},`, repetitions), ",") + `],"services":{}}`)
}

func InvalidPayload() json.RawMessage {
	return json.RawMessage(`{"interfaces":`)
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	return append(json.RawMessage(nil), in...)
}
