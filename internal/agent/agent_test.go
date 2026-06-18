package agent

import (
	"strings"
	"testing"

	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/apply"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/applyvyos"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/config"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderervyos"
)

/*
TC-AGENT-CONFIGURE-001
Type: Positive
Title: Placeholder mode wires placeholder engines
Summary:
Builds configure engines with placeholder mode selected.
The runtime should use internal placeholder renderer and apply
implementations for safe local and CI execution.

Validates:
  - placeholder renderer is selected
  - placeholder apply engine is selected
  - engine construction succeeds
*/
func TestNewConfigureEnginesWiresPlaceholderMode(t *testing.T) {
	cfg := config.DefaultAppConfig()
	cfg.Agent.Configure.Mode = "placeholder"

	rndr, applier, err := newConfigureEngines(&cfg, nil)
	if err != nil {
		t.Fatalf("new configure engines: %v", err)
	}
	if _, ok := rndr.(*renderer.Placeholder); !ok {
		t.Fatalf("renderer type got=%T want *renderer.Placeholder", rndr)
	}
	if _, ok := applier.(*apply.Placeholder); !ok {
		t.Fatalf("apply type got=%T want *apply.Placeholder", applier)
	}
}

/*
TC-AGENT-CONFIGURE-002
Type: Positive
Title: Real mode wires VyOS adapters
Summary:
Builds configure engines with real mode selected.
The runtime should create adapters around the olg-renderer-vyos
renderer and apply packages.

Validates:
  - real renderer adapter is selected
  - real apply adapter is selected
  - engine construction succeeds
*/
func TestNewConfigureEnginesWiresRealMode(t *testing.T) {
	cfg := config.DefaultAppConfig()
	cfg.Agent.Configure.Mode = "real"

	rndr, applier, err := newConfigureEngines(&cfg, nil)
	if err != nil {
		t.Fatalf("new configure engines: %v", err)
	}
	if _, ok := rndr.(*renderervyos.Adapter); !ok {
		t.Fatalf("renderer type got=%T want *renderervyos.Adapter", rndr)
	}
	if _, ok := applier.(*applyvyos.Adapter); !ok {
		t.Fatalf("apply type got=%T want *applyvyos.Adapter", applier)
	}
}

/*
TC-AGENT-CONFIGURE-003
Type: Positive
Title: Real mode accepts debug logging config
Summary:
Builds configure engines with real mode and debug log flags enabled.
Debug settings should be accepted without changing the selected
renderer and apply adapter implementations.

Validates:
  - real renderer adapter is still selected
  - real apply adapter is still selected
  - debug flags do not break construction
*/
func TestNewConfigureEnginesAcceptsDebugLoggingConfig(t *testing.T) {
	cfg := config.DefaultAppConfig()
	cfg.Agent.Configure.Mode = "real"
	cfg.Agent.Logging.Level = "debug"
	cfg.Agent.Debug.LogPayloads = true
	cfg.Agent.Debug.LogRendered = true
	cfg.Agent.Debug.LogApplyPlan = true

	rndr, applier, err := newConfigureEngines(&cfg, nil)
	if err != nil {
		t.Fatalf("new configure engines: %v", err)
	}
	if _, ok := rndr.(*renderervyos.Adapter); !ok {
		t.Fatalf("renderer type got=%T want *renderervyos.Adapter", rndr)
	}
	if _, ok := applier.(*applyvyos.Adapter); !ok {
		t.Fatalf("apply type got=%T want *applyvyos.Adapter", applier)
	}
}

/*
TC-AGENT-CONFIGURE-004
Type: Negative
Title: Invalid configure mode fails wiring
Summary:
Builds configure engines with an unsupported mode.
Runtime construction should fail before the agent starts with a
clear configure mode error.

Validates:
  - invalid mode returns an error
  - error mentions agent.configure.mode
*/
func TestNewConfigureEnginesRejectsInvalidMode(t *testing.T) {
	cfg := config.DefaultAppConfig()
	cfg.Agent.Configure.Mode = "invalid"

	_, _, err := newConfigureEngines(&cfg, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "agent.configure.mode") {
		t.Fatalf("error %q does not mention agent.configure.mode", err.Error())
	}
}
