package agent

import (
	"testing"

	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/config"
)

/*
TC-LOGGING-006
Type: Safety
Title: Payload flag without debug does not log payload
Summary:
Builds configure debug logging config from app settings where the payload
flag is enabled but the logger level remains info. Runtime wiring should
not pass payload logging through unless the logger level is debug.

Validates:
  - payload flag alone is not enough
  - rendered/apply plan debug flags are also suppressed at non-debug level
*/
func TestLoggingPayloadFlagWithoutDebugDoesNotLogPayload(t *testing.T) {
	cfg := config.DefaultAppConfig()
	cfg.Agent.Logging.Level = "info"
	cfg.Agent.Debug.LogPayloads = true
	cfg.Agent.Debug.LogRendered = true
	cfg.Agent.Debug.LogApplyPlan = true

	got := configureDebugConfig(&cfg)
	if got.LogPayloads || got.LogRendered || got.LogApplyPlan {
		t.Fatalf("debug config got=%+v want all false when logging level is info", got)
	}
}

/*
TC-LOGGING-007
Type: Safety
Title: Partial debug config does not emit debug logs
Summary:
Builds configure debug logging config with debug level enabled but no
explicit debug data flags. Runtime wiring should keep all raw-data logging
disabled unless each flag is explicitly enabled.

Validates:
  - debug level alone is not enough for payload logging
  - debug level alone is not enough for rendered/apply plan logging
*/
func TestLoggingPartialDebugConfigDoesNotEmitDebugLogs(t *testing.T) {
	cfg := config.DefaultAppConfig()
	cfg.Agent.Logging.Level = "debug"

	got := configureDebugConfig(&cfg)
	if got.LogPayloads || got.LogRendered || got.LogApplyPlan {
		t.Fatalf("debug config got=%+v want all false without explicit debug flags", got)
	}
}
