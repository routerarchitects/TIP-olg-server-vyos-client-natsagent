package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

var ErrMTLSCertNotFound = errors.New("mTLS certificate or key not found: required for secure-rtty (checked /etc/olg/secrets, /etc/rtty, /etc/olg/client)")

// RTTYProcessRunner abstracts process execution for rtty to enable unit testing.
type RTTYProcessRunner interface {
	FindBinary() (string, error)
	KillExisting(ctx context.Context) error
	Start(bin string, args []string) error
}

type RealRTTYProcessRunner struct{}

func NewRealRTTYProcessRunner() RTTYProcessRunner {
	return &RealRTTYProcessRunner{}
}

func (r *RealRTTYProcessRunner) FindBinary() (string, error) {
	candidates := []string{
		"/usr/sbin/rtty",
		"/usr/bin/rtty",
		"/usr/local/bin/rtty",
	}
	for _, path := range candidates {
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() && (fi.Mode()&0o111 != 0) {
			return path, nil
		}
	}
	if lp, err := exec.LookPath("rtty"); err == nil {
		return lp, nil
	}
	return "", errors.New("rtty binary not found in /usr/sbin, /usr/bin, /usr/local/bin, or PATH")
}

func (r *RealRTTYProcessRunner) KillExisting(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "pkill", "-x", "rtty")
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		// pkill exit code 1 means no matching processes were found, which is normal.
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil
		}
		log.Printf("rtty: kill existing process failed: %v", err)
	}
	return nil
}

// Start launches rtty detached from the short-lived action RPC context so the
// terminal session outlives the NATS command invocation. Its lifecycle is governed
// by OWGW socket disconnection, user logout, or preemption by KillExisting.
func (r *RealRTTYProcessRunner) Start(bin string, args []string) error {
	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start rtty: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// The 50ms wait is an OS-level startup sanity check to catch immediate startup failures
	// (e.g. dynamic linker errors like missing libev, invalid CLI flags, or permission errors)
	// before acknowledging the RPC. In accordance with the OpenWiFi/uCentral asynchronous
	// control-plane specification, the RPC acknowledges successful process initiation ("rtty session initiated").
	// The actual TLS handshake and device registration occur out-of-band between rtty and
	// OWGW port 5912, which the cloud gateway verifies before opening the browser terminal viewport.
	select {
	case err := <-done:
		return fmt.Errorf("rtty exited immediately: %w", err)
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

type VyOSRTTYExecutor struct {
	runner   RTTYProcessRunner
	certFile string
	keyFile  string
	mu       sync.Mutex
}

func NewVyOSRTTYExecutor(runner RTTYProcessRunner) *VyOSRTTYExecutor {
	if runner == nil {
		runner = NewRealRTTYProcessRunner()
	}
	return &VyOSRTTYExecutor{
		runner: runner,
	}
}

// WithCertAndKey sets custom cert and key paths (useful for tests or custom environments).
func (e *VyOSRTTYExecutor) WithCertAndKey(certFile, keyFile string) *VyOSRTTYExecutor {
	e.certFile = certFile
	e.keyFile = keyFile
	return e
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func (e *VyOSRTTYExecutor) findCertAndKey() (string, string) {
	if e.certFile != "" && e.keyFile != "" {
		return e.certFile, e.keyFile
	}
	if c := os.Getenv("RTTY_CERT_FILE"); c != "" {
		if k := os.Getenv("RTTY_KEY_FILE"); k != "" {
			if fileExists(c) && fileExists(k) {
				return c, k
			}
		}
	}
	candidates := [][2]string{
		{"/etc/olg/secrets/cert.pem", "/etc/olg/secrets/key.pem"},
		{"/etc/rtty/cert.pem", "/etc/rtty/key.pem"},
		{"/etc/olg/client/cert.pem", "/etc/olg/client/key.pem"},
	}
	for _, pair := range candidates {
		if fileExists(pair[0]) && fileExists(pair[1]) {
			return pair[0], pair[1]
		}
	}
	return "", ""
}

func (e *VyOSRTTYExecutor) Execute(ctx context.Context, msg agentcore.ActionCommand) (Output, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if ctx == nil {
		return Output{}, errors.New("execute action: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, fmt.Errorf("execute action: %w", err)
	}
	if msg.Target == "" {
		return Output{}, errors.New("execute action: target is empty")
	}
	if msg.Action != ActionRTTY {
		return Output{}, fmt.Errorf("execute action: unsupported action %q", msg.Action)
	}
	if msg.RPCID == "" {
		return Output{}, errors.New("execute action: rpc id is empty")
	}
	if len(msg.Payload) == 0 || !json.Valid(msg.Payload) {
		return Output{}, fmt.Errorf("%w: payload must be valid json", ErrInvalidActionPayload)
	}

	var payload RTTYPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return Output{}, fmt.Errorf("%w: parse rtty payload: %w", ErrInvalidActionPayload, err)
	}
	if err := payload.Validate(); err != nil {
		return Output{}, err
	}

	bin, err := e.runner.FindBinary()
	if err != nil {
		return Output{}, fmt.Errorf("find rtty binary: %w", err)
	}

	if err := e.runner.KillExisting(ctx); err != nil {
		return Output{}, fmt.Errorf("kill existing rtty: %w", err)
	}

	args := []string{
		"-I", payload.ID,
		"-h", payload.Server,
		"-p", strconv.Itoa(payload.Port),
		"-t", payload.Token,
		"-s",
		"-a",
		"-v",
		"-d", "VyOS",
	}
	if payload.Timeout > 0 {
		args = append(args, "-e", strconv.Itoa(payload.Timeout))
	}
	cert, key := e.findCertAndKey()
	if cert == "" || key == "" {
		return Output{}, ErrMTLSCertNotFound
	}
	args = append(args, "-c", cert, "-k", key)

	if err := e.runner.Start(bin, args); err != nil {
		return Output{}, fmt.Errorf("launch rtty: %w", err)
	}

	resPayload, err := json.Marshal(map[string]any{
		"error": 0,
		"text":  "Success",
		"id":    payload.ID,
	})
	if err != nil {
		return Output{}, fmt.Errorf("build rtty result payload: %w", err)
	}

	// Return success acknowledging process initiation. In the uCentral architecture, OWGW's
	// RTTYS server independently verifies TLS session registration on port 5912 before
	// allowing browser websocket attachment on port 5913.
	return Output{
		Payload: resPayload,
		Message: "rtty session initiated",
	}, nil
}
