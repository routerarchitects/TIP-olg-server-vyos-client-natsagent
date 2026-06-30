package config

import "testing"

/*
TC-CONFIG-DEFAULTS-001
Type: Positive
Title: Config default configure mode is placeholder
Summary:
Loads YAML that omits agent.configure.mode.
The final config should explicitly contain placeholder mode rather than
relying on hidden runtime selection.

Validates:
  - omitted configure mode resolves to placeholder
  - loaded config remains valid
*/
func TestConfigDefaultConfigureModeIsPlaceholder(t *testing.T) {
	path := writeConfigForTest(t, `
agent:
  name: default-mode-agent
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Agent.Configure.Mode != "placeholder" {
		t.Fatalf("configure mode got=%q want=placeholder", cfg.Agent.Configure.Mode)
	}
}

/*
TC-CONFIG-DEFAULTS-002
Type: Positive
Title: YAML overrides defaults correctly
Summary:
Loads YAML that overrides several default values.
The provided YAML values should win while unrelated omitted values
remain defaulted.

Validates:
  - explicit YAML values are preserved
  - defaults fill unrelated fields
*/
func TestConfigYAMLOverridesDefaultsCorrectly(t *testing.T) {
	path := writeConfigForTest(t, `
agent:
  name: custom-agent
  version: 9.9.9
  target: vyos-custom
  state_file: /tmp/custom-state.json
  configure:
    mode: real
  apply:
    save_after_commit: true
agentcore:
  kv:
    bucket: custom_bucket
    storage: memory
  retry:
    publish_attempts: 3
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Agent.Name != "custom-agent" {
		t.Fatalf("name got=%q want=custom-agent", cfg.Agent.Name)
	}
	if cfg.Agent.Version != "9.9.9" {
		t.Fatalf("version got=%q want=9.9.9", cfg.Agent.Version)
	}
	if cfg.Agent.Target != "vyos-custom" {
		t.Fatalf("target got=%q want=vyos-custom", cfg.Agent.Target)
	}
	if cfg.Agent.StateFile != "/tmp/custom-state.json" {
		t.Fatalf("state file got=%q want=/tmp/custom-state.json", cfg.Agent.StateFile)
	}
	if cfg.Agent.Configure.Mode != "real" {
		t.Fatalf("configure mode got=%q want=real", cfg.Agent.Configure.Mode)
	}
	if !cfg.Agent.Apply.SaveAfterCommit {
		t.Fatal("save_after_commit got=false want=true")
	}
	if cfg.AgentCore.KV.Bucket != "custom_bucket" {
		t.Fatalf("kv bucket got=%q want=custom_bucket", cfg.AgentCore.KV.Bucket)
	}
	if cfg.AgentCore.KV.Storage != "memory" {
		t.Fatalf("kv storage got=%q want=memory", cfg.AgentCore.KV.Storage)
	}
	if cfg.AgentCore.Retry.PublishAttempts != 3 {
		t.Fatalf("publish attempts got=%d want=3", cfg.AgentCore.Retry.PublishAttempts)
	}
	if cfg.AgentCore.Subjects.ConfigurePattern != "cmd.configure.%s" {
		t.Fatalf("configure subject default got=%q", cfg.AgentCore.Subjects.ConfigurePattern)
	}
}

/*
TC-CONFIG-DEFAULTS-003
Type: Safety
Title: Defaults are not reapplied after overlay
Summary:
Loads YAML values that differ from defaults.
The final config should preserve those values and should not drift back
to defaults after validation.

Validates:
  - overlay values are stable
  - validation does not mutate overridden values
*/
func TestConfigDefaultsAreNotReappliedAfterOverlay(t *testing.T) {
	path := writeConfigForTest(t, `
agent:
  logging:
    level: debug
    format: json
  actions:
    enabled:
      - trace
agentcore:
  nats:
    servers:
      - nats://192.0.2.10:4222
    client_name: custom-client
    max_reconnects: 7
  timeouts:
    publish_timeout: 11s
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Agent.Logging.Level != "debug" {
		t.Fatalf("logging level got=%q want=debug", cfg.Agent.Logging.Level)
	}
	if cfg.Agent.Logging.Format != "json" {
		t.Fatalf("logging format got=%q want=json", cfg.Agent.Logging.Format)
	}
	if got := cfg.AgentCore.NATS.Servers; len(got) != 1 || got[0] != "nats://192.0.2.10:4222" {
		t.Fatalf("nats servers got=%v want=[nats://192.0.2.10:4222]", got)
	}
	if cfg.AgentCore.NATS.ClientName != "custom-client" {
		t.Fatalf("client name got=%q want=custom-client", cfg.AgentCore.NATS.ClientName)
	}
	if cfg.AgentCore.NATS.MaxReconnects != 7 {
		t.Fatalf("max reconnects got=%d want=7", cfg.AgentCore.NATS.MaxReconnects)
	}
	if cfg.AgentCore.Timeouts.PublishTimeout != "11s" {
		t.Fatalf("publish timeout got=%q want=11s", cfg.AgentCore.Timeouts.PublishTimeout)
	}
}
