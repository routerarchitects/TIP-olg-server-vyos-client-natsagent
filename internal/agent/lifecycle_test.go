package agent

import (
	"bytes"
	"context"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/config"
	"github.com/nats-io/nats.go"
)

func startNatsServer(t *testing.T) (string, func()) {
	t.Helper()
	bin, err := exec.LookPath("nats-server")
	if err != nil {
		t.Skip("nats-server binary not found, skipping test")
	}

	dataDir := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()

	cmd := exec.Command(bin, "-js", "-a", "127.0.0.1", "-p", port, "-sd", dataDir)
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start nats-server: %v", err)
	}

	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}

	url := "nats://127.0.0.1:" + port
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		nc, err := nats.Connect(url, nats.Name("ready-check"), nats.Timeout(200*time.Millisecond))
		if err == nil {
			nc.Close()
			return url, stop
		}
		time.Sleep(50 * time.Millisecond)
	}

	stop()
	t.Fatalf("nats-server on %s did not become ready: %s", url, logs.String())
	return "", nil
}

/*
TC-AGENT-LIFECYCLE-001
Type: Positive / Recovery
Title: Runtime close triggered on startup status publication failure
Summary:
Verifies that if publishStartupStatus fails during Start(), a best-effort
Close is triggered to clean up resources before returning the error.

Validates:
  - client Start succeeds
  - NATS connection failure during timestamp retrieval causes status publish failure
  - Runtime Close is called and closed state is set to true
*/
func TestRuntimeStartFailureCloses(t *testing.T) {
	url, stopNats := startNatsServer(t)
	// We do not defer stopNats here because we will call it manually to simulate a connection error.

	cfg := config.DefaultAppConfig()
	cfg.Agent.Configure.Mode = "placeholder"
	cfg.Agent.StateFile = t.TempDir() + "/state.json"
	cfg.AgentCore.NATS.Servers = []string{url}
	cfg.AgentCore.NATS.ConnectTimeout = "500ms"

	coreCfg, err := cfg.ToAgentCoreConfig()
	if err != nil {
		t.Fatalf("failed to build core config: %v", err)
	}

	ctx := context.Background()

	customClock := func() time.Time {
		stopNats()
		return time.Now()
	}

	rt, err := New(&cfg, coreCfg, WithClock(customClock))
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}

	err = rt.Start(ctx)
	if err == nil {
		t.Fatal("expected Start() to fail due to NATS connection error, got nil")
	}

	rt.mu.Lock()
	closed := rt.closed
	rt.mu.Unlock()

	if !closed {
		t.Error("expected runtime to be closed after startup failure")
	}
}

/*
TC-AGENT-LIFECYCLE-002
Type: Positive / Recovery
Title: Runtime handles SIGTERM during startup and shuts down gracefully
Summary:
Verifies that if a signal-derived context is cancelled during startup (after client Start
but before status publish), the status is still published using a fresh context, and the
runtime shuts down gracefully returning nil.

Validates:
  - client Start succeeds
  - context cancellation during startup does not fail status publish
  - rt.Run(ctx) returns nil (exit code 0) on graceful shutdown
*/
func TestRuntimeSIGTERMDuringStartupGracefulShutdown(t *testing.T) {
	url, stopNats := startNatsServer(t)
	defer stopNats()

	cfg := config.DefaultAppConfig()
	cfg.Agent.Configure.Mode = "placeholder"
	cfg.Agent.StateFile = t.TempDir() + "/state.json"
	cfg.AgentCore.NATS.Servers = []string{url}
	cfg.AgentCore.NATS.ConnectTimeout = "500ms"

	coreCfg, err := cfg.ToAgentCoreConfig()
	if err != nil {
		t.Fatalf("failed to build core config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	customClock := func() time.Time {
		cancel() // Cancel the signal-derived context during startup status timestamp generation
		return time.Now()
	}

	rt, err := New(&cfg, coreCfg, WithClock(customClock))
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}

	err = rt.Run(ctx)
	if err != nil {
		t.Fatalf("expected Run() to return nil for graceful shutdown, got: %v", err)
	}

	rt.mu.Lock()
	closed := rt.closed
	rt.mu.Unlock()

	if !closed {
		t.Error("expected runtime to be closed after Run() completed")
	}
}
