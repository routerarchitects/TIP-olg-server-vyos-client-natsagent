package config

import (
	"strings"
	"testing"
)

/*
TC-CONFIG-VALIDATE-007
Type: Negative
Title: Invalid NATS config fails validation
Summary:
Checks representative invalid NATS settings.
Validation should reject unusable NATS configuration before runtime
startup is attempted.

Validates:
  - empty server list is rejected
  - whitespace-only server list is rejected
  - invalid reconnect settings are rejected
*/
func TestConfigInvalidNATSConfigFailsValidation(t *testing.T) {
	cases := []struct {
		name          string
		mutate        func(*AppConfig)
		errorContains string
	}{
		{
			name: "empty servers",
			mutate: func(cfg *AppConfig) {
				cfg.AgentCore.NATS.Servers = nil
			},
			errorContains: "agentcore.nats.servers",
		},
		{
			name: "whitespace servers",
			mutate: func(cfg *AppConfig) {
				cfg.AgentCore.NATS.Servers = []string{"  ", "\t"}
			},
			errorContains: "agentcore.nats.servers",
		},
		{
			name: "unsupported retry on failed connect",
			mutate: func(cfg *AppConfig) {
				cfg.AgentCore.NATS.RetryOnFailedConnect = true
			},
			errorContains: "retry_on_failed_connect",
		},
		{
			name: "negative reconnect buffer",
			mutate: func(cfg *AppConfig) {
				cfg.AgentCore.NATS.ReconnectBufSize = -1
			},
			errorContains: "reconnect_buf_size",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultAppConfig()
			tc.mutate(&cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errorContains) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errorContains)
			}
		})
	}
}

/*
TC-CONFIG-VALIDATE-008
Type: Negative
Title: Invalid subject pattern fails validation
Summary:
Checks malformed subject patterns.
Validation should reject wildcard tokens, whitespace, and incorrect
format placeholders before runtime startup.

Validates:
  - wildcard subjects are rejected
  - subject whitespace is rejected
  - incorrect placeholder count is rejected
*/
func TestConfigInvalidSubjectPatternFailsValidation(t *testing.T) {
	cases := []struct {
		name          string
		mutate        func(*AppConfig)
		errorContains string
	}{
		{
			name: "wildcard configure subject",
			mutate: func(cfg *AppConfig) {
				cfg.AgentCore.Subjects.ConfigurePattern = "cmd.configure.>"
			},
			errorContains: "configure_pattern",
		},
		{
			name: "whitespace action subject",
			mutate: func(cfg *AppConfig) {
				cfg.AgentCore.Subjects.ActionPattern = "cmd action %s %s"
			},
			errorContains: "action_pattern",
		},
		{
			name: "missing status placeholder",
			mutate: func(cfg *AppConfig) {
				cfg.AgentCore.Subjects.StatusPattern = "status"
			},
			errorContains: "status_pattern",
		},
		{
			name: "unsupported result format directive",
			mutate: func(cfg *AppConfig) {
				cfg.AgentCore.Subjects.ResultPattern = "result.%d"
			},
			errorContains: "result_pattern",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultAppConfig()
			tc.mutate(&cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errorContains) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errorContains)
			}
		})
	}
}

/*
TC-CONFIG-VALIDATE-009
Type: Negative
Title: Unsupported action fails validation
Summary:
Enables an unsupported action name.
Validation should fail clearly instead of silently accepting an action
that has no registered executor.

Validates:
  - unsupported action returns error
  - error identifies the unsupported action
*/
func TestConfigUnsupportedActionFailsValidation(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.Agent.Actions.Enabled = []string{"trace", "reboot"}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported action") || !strings.Contains(err.Error(), "reboot") {
		t.Fatalf("unexpected error: %v", err)
	}
}

/*
TC-CONFIG-VALIDATE-010
Type: Negative
Title: Invalid configure mode fails at parse level
Summary:
Loads YAML with unsupported configure modes.
The loader validates the mode immediately after parsing, before any
runtime engine construction can be reached.

Validates:
  - unsupported configure modes fail Load
  - empty explicit configure mode fails Load
  - error identifies agent.configure.mode
*/
func TestConfigInvalidConfigureModeFailsAtParseLevel(t *testing.T) {
	for _, mode := range []string{"invalid", "real-mode", ""} {
		t.Run("mode_"+mode, func(t *testing.T) {
			path := writeConfigForTest(t, `
agent:
  configure:
    mode: "`+mode+`"
`)

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "agent.configure.mode") {
				t.Fatalf("error %q does not mention agent.configure.mode", err.Error())
			}
		})
	}
}

/*
TC-CONFIG-VALIDATE-011
Type: Negative
Title: Duplicate action fails validation
Summary:
Asserts that configurations containing duplicate action names fail validation.

Validates:
  - duplicate action returns error
  - error identifies duplicate action
*/
func TestConfigDuplicateActionFailsValidation(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.Agent.Actions.Enabled = []string{"trace", "trace"}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate action") || !strings.Contains(err.Error(), "trace") {
		t.Fatalf("unexpected error: %v", err)
	}
}
