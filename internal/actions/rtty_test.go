package actions

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

type fakeRTTYProcessRunner struct {
	binPath     string
	findErr     error
	killErr     error
	startErr    error
	killCalled  bool
	startCalled bool
	startBin    string
	startArgs   []string
}

func (f *fakeRTTYProcessRunner) FindBinary() (string, error) {
	if f.findErr != nil {
		return "", f.findErr
	}
	if f.binPath != "" {
		return f.binPath, nil
	}
	return "/usr/sbin/rtty", nil
}

func (f *fakeRTTYProcessRunner) KillExisting(ctx context.Context) error {
	f.killCalled = true
	return f.killErr
}

func (f *fakeRTTYProcessRunner) Start(bin string, args []string) error {
	f.startCalled = true
	f.startBin = bin
	f.startArgs = args
	return f.startErr
}

func validRTTYPayloadBytes() []byte {
	return []byte(`{
		"method": "rtty",
		"serial": "089798eeb450",
		"id": "conn123",
		"token": "tok456",
		"server": "gw.openwifi.net",
		"port": 5912,
		"timeout": 60,
		"user": "admin"
	}`)
}

func TestRTTYPayload_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*RTTYPayload)
		wantErr bool
	}{
		{
			name:    "valid payload",
			mutate:  func(p *RTTYPayload) {},
			wantErr: false,
		},
		{
			name:    "empty id",
			mutate:  func(p *RTTYPayload) { p.ID = "" },
			wantErr: true,
		},
		{
			name:    "whitespace id",
			mutate:  func(p *RTTYPayload) { p.ID = "   " },
			wantErr: true,
		},
		{
			name:    "invalid chars in id",
			mutate:  func(p *RTTYPayload) { p.ID = "id;rm -rf" },
			wantErr: true,
		},
		{
			name:    "empty token",
			mutate:  func(p *RTTYPayload) { p.Token = "" },
			wantErr: true,
		},
		{
			name:    "invalid chars in token",
			mutate:  func(p *RTTYPayload) { p.Token = "tok$var" },
			wantErr: true,
		},
		{
			name:    "empty server",
			mutate:  func(p *RTTYPayload) { p.Server = "" },
			wantErr: true,
		},
		{
			name:    "invalid chars in server",
			mutate:  func(p *RTTYPayload) { p.Server = "srv;bad" },
			wantErr: true,
		},
		{
			name:    "zero port",
			mutate:  func(p *RTTYPayload) { p.Port = 0 },
			wantErr: true,
		},
		{
			name:    "negative port",
			mutate:  func(p *RTTYPayload) { p.Port = -1 },
			wantErr: true,
		},
		{
			name:    "too high port",
			mutate:  func(p *RTTYPayload) { p.Port = 65536 },
			wantErr: true,
		},
		{
			name:    "negative timeout",
			mutate:  func(p *RTTYPayload) { p.Timeout = -5 },
			wantErr: true,
		},
		{
			name:    "zero timeout defaults to 60",
			mutate:  func(p *RTTYPayload) { p.Timeout = 0 },
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := RTTYPayload{
				ID:      "id1",
				Token:   "tok1",
				Server:  "127.0.0.1",
				Port:    5912,
				Timeout: 60,
			}
			tc.mutate(&p)
			err := p.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && p.Timeout == 0 {
				t.Fatalf("expected timeout to default to non-zero, got %d", p.Timeout)
			}
		})
	}
}

func TestPlaceholderRTTYExecutor(t *testing.T) {
	exec := NewPlaceholderRTTYExecutor()

	ctx := context.Background()
	msg := agentcore.ActionCommand{
		Target:  "vyos",
		Action:  ActionRTTY,
		RPCID:   "rpc-rtty-1",
		Payload: validRTTYPayloadBytes(),
	}

	out, err := exec.Execute(ctx, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Message != "placeholder rtty action completed" {
		t.Fatalf("unexpected message: %q", out.Message)
	}

	var m map[string]any
	if err := json.Unmarshal(out.Payload, &m); err != nil {
		t.Fatalf("payload should be valid json: %v", err)
	}
	if m["executor"] != "placeholder_rtty" || m["action"] != ActionRTTY {
		t.Fatalf("payload mismatch: %+v", m)
	}
}

func TestPlaceholderRTTYExecutor_Errors(t *testing.T) {
	exec := NewPlaceholderRTTYExecutor()
	ctx := context.Background()

	// 1. Nil context
	if _, err := exec.Execute(nil, agentcore.ActionCommand{}); err == nil {
		t.Fatal("expected error for nil context")
	}

	// 2. Canceled context
	cCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := exec.Execute(cCtx, agentcore.ActionCommand{}); err == nil {
		t.Fatal("expected error for canceled context")
	}

	// 3. Empty target
	if _, err := exec.Execute(ctx, agentcore.ActionCommand{Action: ActionRTTY, RPCID: "1"}); err == nil {
		t.Fatal("expected error for empty target")
	}

	// 4. Unsupported action
	if _, err := exec.Execute(ctx, agentcore.ActionCommand{Target: "vyos", Action: "unknown", RPCID: "1"}); err == nil {
		t.Fatal("expected error for unsupported action")
	}

	// 5. Empty RPCID
	if _, err := exec.Execute(ctx, agentcore.ActionCommand{Target: "vyos", Action: ActionRTTY}); err == nil {
		t.Fatal("expected error for empty rpc id")
	}

	// 6. Invalid JSON payload
	if _, err := exec.Execute(ctx, agentcore.ActionCommand{Target: "vyos", Action: ActionRTTY, RPCID: "1", Payload: []byte("{invalid")}); err == nil {
		t.Fatal("expected error for invalid json")
	}

	// 7. Invalid parameters
	if _, err := exec.Execute(ctx, agentcore.ActionCommand{Target: "vyos", Action: ActionRTTY, RPCID: "1", Payload: []byte(`{"id":""}`)}); err == nil {
		t.Fatal("expected error for invalid parameters")
	}
}

func TestVyOSRTTYExecutor_Success(t *testing.T) {
	runner := &fakeRTTYProcessRunner{
		binPath: "/usr/sbin/rtty",
	}
	exec := NewVyOSRTTYExecutor(runner).WithCertAndKey("/tmp/cert.pem", "/tmp/key.pem")

	ctx := context.Background()
	msg := agentcore.ActionCommand{
		Target:  "vyos",
		Action:  ActionRTTY,
		RPCID:   "rpc-rtty-real-1",
		Payload: validRTTYPayloadBytes(),
	}

	out, err := exec.Execute(ctx, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !runner.killCalled {
		t.Error("expected KillExisting to be called")
	}
	if !runner.startCalled {
		t.Error("expected Start to be called")
	}
	if runner.startBin != "/usr/sbin/rtty" {
		t.Errorf("startBin = %q, want /usr/sbin/rtty", runner.startBin)
	}

	expectedArgs := []string{
		"-I", "conn123",
		"-h", "gw.openwifi.net",
		"-p", "5912",
		"-t", "tok456",
		"-s",
		"-a",
		"-v",
		"-d", "VyOS",
		"-e", "60",
		"-c", "/tmp/cert.pem",
		"-k", "/tmp/key.pem",
	}
	if !reflect.DeepEqual(runner.startArgs, expectedArgs) {
		t.Errorf("startArgs = %v, want %v", runner.startArgs, expectedArgs)
	}

	if out.Message != "rtty session initiated" {
		t.Errorf("message = %q, want 'rtty session initiated'", out.Message)
	}

	var res map[string]any
	if err := json.Unmarshal(out.Payload, &res); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if res["text"] != "Success" || res["error"] != float64(0) || res["id"] != "conn123" {
		t.Fatalf("unexpected result payload: %+v", res)
	}
}

func TestVyOSRTTYExecutor_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("nil context", func(t *testing.T) {
		exec := NewVyOSRTTYExecutor(&fakeRTTYProcessRunner{})
		if _, err := exec.Execute(nil, agentcore.ActionCommand{}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		cCtx, cancel := context.WithCancel(ctx)
		cancel()
		exec := NewVyOSRTTYExecutor(&fakeRTTYProcessRunner{})
		if _, err := exec.Execute(cCtx, agentcore.ActionCommand{}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty target", func(t *testing.T) {
		exec := NewVyOSRTTYExecutor(&fakeRTTYProcessRunner{})
		if _, err := exec.Execute(ctx, agentcore.ActionCommand{Action: ActionRTTY, RPCID: "1"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unsupported action", func(t *testing.T) {
		exec := NewVyOSRTTYExecutor(&fakeRTTYProcessRunner{})
		if _, err := exec.Execute(ctx, agentcore.ActionCommand{Target: "vyos", Action: "trace", RPCID: "1"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty rpc id", func(t *testing.T) {
		exec := NewVyOSRTTYExecutor(&fakeRTTYProcessRunner{})
		if _, err := exec.Execute(ctx, agentcore.ActionCommand{Target: "vyos", Action: ActionRTTY}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid json payload", func(t *testing.T) {
		exec := NewVyOSRTTYExecutor(&fakeRTTYProcessRunner{})
		if _, err := exec.Execute(ctx, agentcore.ActionCommand{Target: "vyos", Action: ActionRTTY, RPCID: "1", Payload: []byte("{")}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid payload params", func(t *testing.T) {
		exec := NewVyOSRTTYExecutor(&fakeRTTYProcessRunner{})
		if _, err := exec.Execute(ctx, agentcore.ActionCommand{Target: "vyos", Action: ActionRTTY, RPCID: "1", Payload: []byte(`{"id":""}`)}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("binary not found", func(t *testing.T) {
		runner := &fakeRTTYProcessRunner{
			findErr: errors.New("binary not found"),
		}
		exec := NewVyOSRTTYExecutor(runner)
		_, err := exec.Execute(ctx, agentcore.ActionCommand{
			Target:  "vyos",
			Action:  ActionRTTY,
			RPCID:   "1",
			Payload: validRTTYPayloadBytes(),
		})
		if err == nil || !errors.Is(err, runner.findErr) && err.Error() != "find rtty binary: binary not found" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("kill existing error", func(t *testing.T) {
		runner := &fakeRTTYProcessRunner{
			killErr: errors.New("kill failed"),
		}
		exec := NewVyOSRTTYExecutor(runner)
		_, err := exec.Execute(ctx, agentcore.ActionCommand{
			Target:  "vyos",
			Action:  ActionRTTY,
			RPCID:   "1",
			Payload: validRTTYPayloadBytes(),
		})
		if err == nil || err.Error() != "kill existing rtty: kill failed" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing mtls certs error", func(t *testing.T) {
		runner := &fakeRTTYProcessRunner{binPath: "/usr/sbin/rtty"}
		exec := NewVyOSRTTYExecutor(runner)
		_, err := exec.Execute(ctx, agentcore.ActionCommand{
			Target:  "vyos",
			Action:  ActionRTTY,
			RPCID:   "1",
			Payload: validRTTYPayloadBytes(),
		})
		if !errors.Is(err, ErrMTLSCertNotFound) {
			t.Fatalf("expected ErrMTLSCertNotFound, got: %v", err)
		}
	})

	t.Run("start error", func(t *testing.T) {
		runner := &fakeRTTYProcessRunner{
			startErr: errors.New("start failed"),
		}
		exec := NewVyOSRTTYExecutor(runner).WithCertAndKey("/tmp/cert.pem", "/tmp/key.pem")
		_, err := exec.Execute(ctx, agentcore.ActionCommand{
			Target:  "vyos",
			Action:  ActionRTTY,
			RPCID:   "1",
			Payload: validRTTYPayloadBytes(),
		})
		if err == nil || err.Error() != "launch rtty: start failed" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("concurrent execute requests serialized without race", func(t *testing.T) {
		runner := &fakeRTTYProcessRunner{binPath: "/usr/sbin/rtty"}
		exec := NewVyOSRTTYExecutor(runner).WithCertAndKey("/tmp/cert.pem", "/tmp/key.pem")

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := exec.Execute(ctx, agentcore.ActionCommand{
					Target:  "vyos",
					Action:  ActionRTTY,
					RPCID:   strconv.Itoa(idx),
					Payload: validRTTYPayloadBytes(),
				})
				if err != nil {
					t.Errorf("concurrent execute failed: %v", err)
				}
			}(i)
		}
		wg.Wait()
	})
}
