package config

import (
	"testing"
)

/*
TC-CONFIG-REDACT-001
Type: Positive
Title: AppConfig Redacted masks credentials
Summary:
Verifies that AppConfig.Redacted() correctly hides username, password, token,
credentials files, TLS key/cert/CA files, and embedded credentials in server URLs.

Validates:
  - username, password, token, and other sensitive files are redacted
  - NATS server URLs with embedded credentials are masked
  - server URLs without credentials remain unchanged
*/
func TestAppConfigRedacted(t *testing.T) {
	cfg := AppConfig{}
	cfg.AgentCore.NATS.Username = "myuser"
	cfg.AgentCore.NATS.Password = "mypassword"
	cfg.AgentCore.NATS.Token = "mytoken"
	cfg.AgentCore.NATS.CredentialsFile = "/path/to/creds"
	cfg.AgentCore.NATS.NKeySeedFile = "/path/to/nkey"
	cfg.AgentCore.NATS.UserJWTFile = "/path/to/jwt"
	cfg.AgentCore.NATS.TLS.KeyFile = "/path/to/key"
	cfg.AgentCore.NATS.TLS.CertFile = "/path/to/cert"
	cfg.AgentCore.NATS.TLS.CAFile = "/path/to/ca"
	cfg.AgentCore.NATS.Servers = []string{
		"nats://user1:pass1@localhost:4222",
		"nats://localhost:4222",
		"nats://token1@localhost:4223",
	}

	redacted := cfg.Redacted()

	if redacted.AgentCore.NATS.Username != "********" {
		t.Errorf("expected Username to be redacted, got %q", redacted.AgentCore.NATS.Username)
	}
	if redacted.AgentCore.NATS.Password != "********" {
		t.Errorf("expected Password to be redacted, got %q", redacted.AgentCore.NATS.Password)
	}
	if redacted.AgentCore.NATS.Token != "********" {
		t.Errorf("expected Token to be redacted, got %q", redacted.AgentCore.NATS.Token)
	}
	if redacted.AgentCore.NATS.CredentialsFile != "********" {
		t.Errorf("expected CredentialsFile to be redacted, got %q", redacted.AgentCore.NATS.CredentialsFile)
	}
	if redacted.AgentCore.NATS.NKeySeedFile != "********" {
		t.Errorf("expected NKeySeedFile to be redacted, got %q", redacted.AgentCore.NATS.NKeySeedFile)
	}
	if redacted.AgentCore.NATS.UserJWTFile != "********" {
		t.Errorf("expected UserJWTFile to be redacted, got %q", redacted.AgentCore.NATS.UserJWTFile)
	}
	if redacted.AgentCore.NATS.TLS.KeyFile != "********" {
		t.Errorf("expected KeyFile to be redacted, got %q", redacted.AgentCore.NATS.TLS.KeyFile)
	}
	if redacted.AgentCore.NATS.TLS.CertFile != "********" {
		t.Errorf("expected CertFile to be redacted, got %q", redacted.AgentCore.NATS.TLS.CertFile)
	}
	if redacted.AgentCore.NATS.TLS.CAFile != "********" {
		t.Errorf("expected CAFile to be redacted, got %q", redacted.AgentCore.NATS.TLS.CAFile)
	}

	expectedServers := []string{
		"nats://redacted:redacted@localhost:4222",
		"nats://localhost:4222",
		"nats://redacted:redacted@localhost:4223",
	}
	if len(redacted.AgentCore.NATS.Servers) != len(expectedServers) {
		t.Fatalf("expected %d redacted servers, got %d", len(expectedServers), len(redacted.AgentCore.NATS.Servers))
	}
	for i, s := range redacted.AgentCore.NATS.Servers {
		if s != expectedServers[i] {
			t.Errorf("expected server %d to be %q, got %q", i, expectedServers[i], s)
		}
	}
}
