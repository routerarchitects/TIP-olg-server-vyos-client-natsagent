package config

import (
	"testing"
	"time"
)

/*
TC-CONFIG-CONVERT-001
Type: Positive
Title: Config converts to agentcore config correctly
Summary:
Converts a populated app config into agentcore.Config.
The conversion should preserve shared-library runtime settings and
parse duration strings into duration values.

Validates:
  - agent identity is mapped
  - NATS and TLS fields are mapped
  - subject and KV fields are mapped
  - timeout, retry, and execution fields are mapped
*/
func TestConfigConvertsToAgentCoreConfigCorrectly(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.Agent.Name = "convert-agent"
	cfg.Agent.Version = "2.3.4"
	cfg.Agent.Actions.Enabled = []string{"trace"}
	cfg.AgentCore.NATS.Servers = []string{"nats://nats-a:4222", "tls://nats-b:4222"}
	cfg.AgentCore.NATS.ClientName = "convert-client"
	cfg.AgentCore.NATS.CredentialsFile = "/secrets/creds"
	cfg.AgentCore.NATS.NKeySeedFile = "/secrets/nkey"
	cfg.AgentCore.NATS.UserJWTFile = "/secrets/user.jwt"
	cfg.AgentCore.NATS.Username = "user"
	cfg.AgentCore.NATS.Password = "pass"
	cfg.AgentCore.NATS.Token = "token"
	cfg.AgentCore.NATS.ConnectTimeout = "3s"
	cfg.AgentCore.NATS.MaxReconnects = 5
	cfg.AgentCore.NATS.ReconnectWait = "750ms"
	cfg.AgentCore.NATS.ReconnectBufSize = 128
	cfg.AgentCore.NATS.TLS.Enabled = true
	cfg.AgentCore.NATS.TLS.InsecureSkipVerify = true
	cfg.AgentCore.NATS.TLS.CAFile = "/ca.pem"
	cfg.AgentCore.NATS.TLS.CertFile = "/cert.pem"
	cfg.AgentCore.NATS.TLS.KeyFile = "/key.pem"
	cfg.AgentCore.NATS.TLS.ServerName = "nats.example.test"
	cfg.AgentCore.JetStream.Domain = "domain-a"
	cfg.AgentCore.JetStream.APIPrefix = "$JS.API"
	cfg.AgentCore.JetStream.DefaultTimeout = "4s"
	cfg.AgentCore.Subjects.ConfigurePattern = "cfg.%s"
	cfg.AgentCore.Subjects.ActionPattern = "act.%s.%s"
	cfg.AgentCore.Subjects.ResultPattern = "res.%s"
	cfg.AgentCore.Subjects.StatusPattern = "stat.%s"
	cfg.AgentCore.Subjects.HealthPattern = "healthz.%s"
	cfg.AgentCore.KV.Bucket = "desired_bucket"
	cfg.AgentCore.KV.KeyPattern = "desired.config.%s"
	cfg.AgentCore.KV.AutoCreateBucket = false
	cfg.AgentCore.KV.History = 3
	cfg.AgentCore.KV.TTL = "1m"
	cfg.AgentCore.KV.MaxValueSize = 4096
	cfg.AgentCore.KV.Storage = "memory"
	cfg.AgentCore.KV.Replicas = 2
	cfg.AgentCore.Timeouts.PublishTimeout = "5s"
	cfg.AgentCore.Timeouts.SubscribeTimeout = "6s"
	cfg.AgentCore.Timeouts.KVTimeout = "7s"
	cfg.AgentCore.Timeouts.ShutdownTimeout = "8s"
	cfg.AgentCore.Timeouts.HandlerWarnAfter = "9s"
	cfg.AgentCore.Retry.PublishAttempts = 4
	cfg.AgentCore.Retry.PublishBackoff = "250ms"
	cfg.AgentCore.Execution.HandlerMode = "sync"

	got, err := cfg.ToAgentCoreConfig()
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if got.AgentName != "convert-agent" || got.Version != "2.3.4" {
		t.Fatalf("identity mapping mismatch: %+v", got)
	}
	if len(got.NATS.Servers) != 2 || got.NATS.Servers[0] != "nats://nats-a:4222" || got.NATS.Servers[1] != "tls://nats-b:4222" {
		t.Fatalf("nats servers got=%v", got.NATS.Servers)
	}
	if got.NATS.ClientName != "convert-client" ||
		got.NATS.CredentialsFile != "/secrets/creds" ||
		got.NATS.NKeySeedFile != "/secrets/nkey" ||
		got.NATS.UserJWTFile != "/secrets/user.jwt" ||
		got.NATS.Username != "user" ||
		got.NATS.Password != "pass" ||
		got.NATS.Token != "token" {
		t.Fatalf("nats auth/client mapping mismatch: %+v", got.NATS)
	}
	if got.NATS.ConnectTimeout != 3*time.Second ||
		got.NATS.MaxReconnects != 5 ||
		got.NATS.ReconnectWait != 750*time.Millisecond ||
		got.NATS.ReconnectBufSize != 128 {
		t.Fatalf("nats timing mapping mismatch: %+v", got.NATS)
	}
	if got.NATS.TLS == nil ||
		!got.NATS.TLS.Enabled ||
		!got.NATS.TLS.InsecureSkipVerify ||
		got.NATS.TLS.CAFile != "/ca.pem" ||
		got.NATS.TLS.CertFile != "/cert.pem" ||
		got.NATS.TLS.KeyFile != "/key.pem" ||
		got.NATS.TLS.ServerName != "nats.example.test" {
		t.Fatalf("tls mapping mismatch: %+v", got.NATS.TLS)
	}
	if got.JetStream.Domain != "domain-a" ||
		got.JetStream.APIPrefix != "$JS.API" ||
		got.JetStream.DefaultTimeout != 4*time.Second {
		t.Fatalf("jetstream mapping mismatch: %+v", got.JetStream)
	}
	if got.Subjects.ConfigurePattern != "cfg.%s" ||
		got.Subjects.ActionPattern != "act.%s.%s" ||
		got.Subjects.ResultPattern != "res.%s" ||
		got.Subjects.StatusPattern != "stat.%s" ||
		got.Subjects.HealthPattern != "healthz.%s" {
		t.Fatalf("subject mapping mismatch: %+v", got.Subjects)
	}
	if got.KV.Bucket != "desired_bucket" ||
		got.KV.KeyPattern != "desired.config.%s" ||
		got.KV.AutoCreateBucket ||
		got.KV.History != 3 ||
		got.KV.TTL != time.Minute ||
		got.KV.MaxValueSize != 4096 ||
		got.KV.Storage != "memory" ||
		got.KV.Replicas != 2 {
		t.Fatalf("kv mapping mismatch: %+v", got.KV)
	}
	if got.Timeouts.PublishTimeout != 5*time.Second ||
		got.Timeouts.SubscribeTimeout != 6*time.Second ||
		got.Timeouts.KVTimeout != 7*time.Second ||
		got.Timeouts.ShutdownTimeout != 8*time.Second {
		t.Fatalf("timeout mapping mismatch: %+v", got.Timeouts)
	}
	if got.Retry.PublishAttempts != 4 || got.Retry.PublishBackoff != 250*time.Millisecond {
		t.Fatalf("retry mapping mismatch: %+v", got.Retry)
	}
	if len(cfg.Agent.Actions.Enabled) != 1 || cfg.Agent.Actions.Enabled[0] != "trace" {
		t.Fatalf("action settings should remain available on app config, got=%v", cfg.Agent.Actions.Enabled)
	}
}
