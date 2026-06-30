package agent

import (
	"testing"

	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/apply"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/applyvyos"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/config"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderer"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/renderervyos"
)

/*
TC-CONFIG-CONVERT-002
Type: Safety
Title: Debug flags do not change engine selection
Summary:
Builds configure engines with debug logging enabled in both supported
configure modes. Debug flags should only control logging detail and
must not switch placeholder mode to real mode or real mode to placeholder.

Validates:
  - placeholder mode still selects placeholder engines
  - real mode still selects real adapters
  - debug flags do not change backend mode selection
*/
func TestConfigDebugFlagsDoNotChangeEngineSelection(t *testing.T) {
	cases := []struct {
		name      string
		mode      string
		wantRendr any
		wantApply any
	}{
		{
			name:      "placeholder",
			mode:      "placeholder",
			wantRendr: (*renderer.Placeholder)(nil),
			wantApply: (*apply.Placeholder)(nil),
		},
		{
			name:      "real",
			mode:      "real",
			wantRendr: (*renderervyos.Adapter)(nil),
			wantApply: (*applyvyos.Adapter)(nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultAppConfig()
			cfg.Agent.Configure.Mode = tc.mode
			cfg.Agent.Logging.Level = "debug"
			cfg.Agent.Debug.LogPayloads = true
			cfg.Agent.Debug.LogRendered = true
			cfg.Agent.Debug.LogApplyPlan = true

			rndr, applier, err := newConfigureEngines(&cfg, nil)
			if err != nil {
				t.Fatalf("new configure engines: %v", err)
			}
			switch tc.wantRendr.(type) {
			case *renderer.Placeholder:
				if _, ok := rndr.(*renderer.Placeholder); !ok {
					t.Fatalf("renderer type got=%T want *renderer.Placeholder", rndr)
				}
			case *renderervyos.Adapter:
				if _, ok := rndr.(*renderervyos.Adapter); !ok {
					t.Fatalf("renderer type got=%T want *renderervyos.Adapter", rndr)
				}
			}
			switch tc.wantApply.(type) {
			case *apply.Placeholder:
				if _, ok := applier.(*apply.Placeholder); !ok {
					t.Fatalf("apply type got=%T want *apply.Placeholder", applier)
				}
			case *applyvyos.Adapter:
				if _, ok := applier.(*applyvyos.Adapter); !ok {
					t.Fatalf("apply type got=%T want *applyvyos.Adapter", applier)
				}
			}
		})
	}
}
