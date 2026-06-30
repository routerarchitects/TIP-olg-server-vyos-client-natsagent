package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
TC-CONFIG-LOAD-001
Type: Positive
Title: Config load valid YAML returns success
Summary:
Loads a valid YAML file with a small set of explicit fields.
The loader should overlay the YAML onto defaults, validate the result,
and return a usable config.

Validates:
  - valid YAML loads without error
  - provided fields are preserved
  - omitted required runtime fields are filled from defaults
*/
func TestConfigLoadValidYAMLReturnsSuccess(t *testing.T) {
	path := writeConfigForTest(t, `
agent:
  name: vyos-agent-test
  target: vyos-lab
agentcore:
  nats:
    servers:
      - nats://127.0.0.1:4222
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Agent.Name != "vyos-agent-test" {
		t.Fatalf("agent name got=%q want=vyos-agent-test", cfg.Agent.Name)
	}
	if cfg.Agent.Target != "vyos-lab" {
		t.Fatalf("target got=%q want=vyos-lab", cfg.Agent.Target)
	}
	if cfg.Agent.StateFile == "" {
		t.Fatal("state file should be populated from defaults")
	}
	if cfg.AgentCore.KV.Bucket == "" {
		t.Fatal("kv bucket should be populated from defaults")
	}
}

/*
TC-CONFIG-LOAD-002
Type: Negative
Title: Config load missing file returns error
Summary:
Attempts to load a config file path that does not exist.
The loader should return a clear read error and must not panic.

Validates:
  - missing file returns error
  - error includes read config context
*/
func TestConfigLoadMissingFileReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read config file") {
		t.Fatalf("error %q does not contain read config file", err.Error())
	}
}

/*
TC-CONFIG-LOAD-003
Type: Negative
Title: Config load invalid YAML returns error
Summary:
Attempts to load malformed YAML.
The loader should fail during YAML parsing and must not fall back to
unsafe defaults.

Validates:
  - malformed YAML returns error
  - error includes unmarshal context
*/
func TestConfigLoadInvalidYAMLReturnsError(t *testing.T) {
	path := writeConfigForTest(t, "agent:\n  logging: [")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal config yaml") {
		t.Fatalf("error %q does not contain unmarshal config yaml", err.Error())
	}
}

/*
TC-CONFIG-LOAD-004
Type: Positive
Title: Config load partial YAML applies defaults
Summary:
Loads a partial YAML file that overrides only a few fields.
Missing optional fields should be filled from defaults while explicit
YAML values remain unchanged.

Validates:
  - partial YAML loads successfully
  - defaults fill omitted fields
  - provided fields survive overlay
*/
func TestConfigLoadPartialYAMLAppliesDefaults(t *testing.T) {
	path := writeConfigForTest(t, `
agent:
  name: partial-agent
  logging:
    level: debug
agentcore:
  nats:
    servers:
      - nats://10.0.0.1:4222
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Agent.Name != "partial-agent" {
		t.Fatalf("agent name got=%q want=partial-agent", cfg.Agent.Name)
	}
	if cfg.Agent.Logging.Level != "debug" {
		t.Fatalf("logging level got=%q want=debug", cfg.Agent.Logging.Level)
	}
	if cfg.Agent.Logging.Format != "text" {
		t.Fatalf("logging format got=%q want=text", cfg.Agent.Logging.Format)
	}
	if cfg.Agent.Configure.Mode != "placeholder" {
		t.Fatalf("configure mode got=%q want=placeholder", cfg.Agent.Configure.Mode)
	}
	if got := cfg.AgentCore.NATS.Servers; len(got) != 1 || got[0] != "nats://10.0.0.1:4222" {
		t.Fatalf("nats servers got=%v want=[nats://10.0.0.1:4222]", got)
	}
}

func writeConfigForTest(t *testing.T, data string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
