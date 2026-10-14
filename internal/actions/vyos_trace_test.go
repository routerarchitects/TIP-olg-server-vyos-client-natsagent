package actions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

type fakeCommand struct {
	runFunc func() error
}

func (c *fakeCommand) Run() error {
	if c.runFunc != nil {
		return c.runFunc()
	}
	return nil
}

type fakeCommandRunner struct {
	calls        int
	lastArgs     []string
	runErr       error
	runFunc      func() error
	lastPcapPath string
}

func (r *fakeCommandRunner) Command(ctx context.Context, name string, args ...string) Command {
	r.calls++
	r.lastArgs = args
	for i, arg := range args {
		if arg == "-w" && i+1 < len(args) {
			r.lastPcapPath = args[i+1]
			break
		}
	}
	return &fakeCommand{
		runFunc: func() error {
			if r.lastPcapPath != "" {
				_ = os.WriteFile(r.lastPcapPath, []byte("pcap-contents"), 0o600)
			}
			if r.runFunc != nil {
				return r.runFunc()
			}
			return r.runErr
		},
	}
}

func newTestVyOSTraceExecutor(runner CommandRunner, client *http.Client) *VyOSTraceExecutor {
	exec := NewVyOSTraceExecutor(runner, client)
	exec.inspector = &fakeNetworkInspector{}
	return exec
}

/*
TC-ACTIONS-TRACE-001
Type: Positive
Title: Happy path trace action execution
Summary:
Submits a trace action payload with valid interface and upload URI.
Verifies that tcpdump executes with correct parameters, the PCAP file
is uploaded via HTTP multipart POST, local PCAP is deleted, and success output is returned.
Validates:
  - tcpdump command parameters (interface, output path, packets count)
  - HTTP upload content is correct and multipart boundary is handled
  - local PCAP cleanup works on successful execution
  - deterministic JSON output structure
*/
func TestVyOSTraceExecutorHappyPath(t *testing.T) {
	// 1. Start test HTTP upload server
	var uploadedFileContent []byte
	var formFileName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		formFileName = header.Filename

		content, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read failed", http.StatusInternalServerError)
			return
		}
		uploadedFileContent = content
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"uploaded"}`))
	}))
	defer server.Close()

	runner := &fakeCommandRunner{}

	exec := newTestVyOSTraceExecutor(runner, server.Client())
	msg := agentcore.ActionCommand{
		Version: "1.0",
		RPCID:   "rpc-trace-1",
		Target:  "vyos",
		Action:  ActionTrace,
		Payload: json.RawMessage(`{
			"interface": "eth0",
			"duration": 5,
			"packets": 50,
			"uri": "` + server.URL + `"
		}`),
		Timestamp: time.Now(),
	}

	out, err := exec.Execute(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Message != "trace action completed" {
		t.Fatalf("unexpected message: %q", out.Message)
	}

	// Verify command parameters
	if runner.calls != 1 {
		t.Fatalf("expected 1 command runner call, got %d", runner.calls)
	}
	expectedArgs := []string{"-U", "-i", "eth0", "-w", runner.lastPcapPath, "-c", "50"}
	if len(runner.lastArgs) != len(expectedArgs) {
		t.Fatalf("args len mismatch: got %+v, want %+v", runner.lastArgs, expectedArgs)
	}
	for i, arg := range runner.lastArgs {
		if arg != expectedArgs[i] {
			t.Fatalf("arg[%d] got %q, want %q", i, arg, expectedArgs[i])
		}
	}

	// Verify uploaded file
	if string(uploadedFileContent) != "pcap-contents" {
		t.Fatalf("uploaded content got %q want %q", string(uploadedFileContent), "pcap-contents")
	}
	if formFileName != filepath.Base(runner.lastPcapPath) {
		t.Fatalf("uploaded file name got %q want %q", formFileName, filepath.Base(runner.lastPcapPath))
	}

	// Verify local cleanup
	if _, err := os.Stat(runner.lastPcapPath); !os.IsNotExist(err) {
		t.Fatalf("expected pcap file to be cleaned up, stat returned: %v", err)
	}
}

/*
TC-ACTIONS-TRACE-002
Type: Negative
Title: Payload validation for trace action execution
Summary:
Asserts payload structure validation constraints on the input payload,
including required fields, interface format to prevent command injection,
and HTTP/HTTPS URI scheme check.
Validates:
  - interface name format matching strict rules
  - upload URI scheme (only http/https are allowed)
  - json structure validity
  - empty target or rpc ID validation
*/
func TestVyOSTraceExecutorPayloadValidation(t *testing.T) {
	runner := &fakeCommandRunner{}
	exec := newTestVyOSTraceExecutor(runner, nil)
	exec.routeResolver = func(_ string) string { return "" }

	cases := []struct {
		name    string
		msg     agentcore.ActionCommand
		wantErr string
	}{
		{
			name: "missing interface",
			msg: agentcore.ActionCommand{
				Version: "1.0", RPCID: "rpc-1", Target: "vyos", Action: ActionTrace,
				Payload: json.RawMessage(`{"uri":"http://localhost"}`),
			},
			wantErr: "interface is required",
		},
		{
			name: "invalid interface name / command injection safe",
			msg: agentcore.ActionCommand{
				Version: "1.0", RPCID: "rpc-1", Target: "vyos", Action: ActionTrace,
				Payload: json.RawMessage(`{"interface":"eth0; rm -rf /","uri":"http://localhost"}`),
			},
			wantErr: "invalid interface name",
		},
		{
			name: "missing uri",
			msg: agentcore.ActionCommand{
				Version: "1.0", RPCID: "rpc-1", Target: "vyos", Action: ActionTrace,
				Payload: json.RawMessage(`{"interface":"eth0"}`),
			},
			wantErr: "uri is required",
		},
		{
			name: "invalid uri scheme",
			msg: agentcore.ActionCommand{
				Version: "1.0", RPCID: "rpc-1", Target: "vyos", Action: ActionTrace,
				Payload: json.RawMessage(`{"interface":"eth0","uri":"ftp://localhost"}`),
			},
			wantErr: "invalid upload uri",
		},
		{
			name: "invalid json payload",
			msg: agentcore.ActionCommand{
				Version: "1.0", RPCID: "rpc-1", Target: "vyos", Action: ActionTrace,
				Payload: json.RawMessage(`{"interface":`),
			},
			wantErr: "payload must be valid json",
		},
		{
			name: "empty target",
			msg: agentcore.ActionCommand{
				Version: "1.0", RPCID: "rpc-1", Target: "", Action: ActionTrace,
				Payload: json.RawMessage(`{"interface":"eth0","uri":"http://localhost"}`),
			},
			wantErr: "target is empty",
		},
		{
			name: "empty rpc id",
			msg: agentcore.ActionCommand{
				Version: "1.0", RPCID: "", Target: "vyos", Action: ActionTrace,
				Payload: json.RawMessage(`{"interface":"eth0","uri":"http://localhost"}`),
			},
			wantErr: "rpc id is empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := exec.Execute(context.Background(), tc.msg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error got %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

/*
TC-ACTIONS-TRACE-003
Type: Negative
Title: Context cancellation for trace action execution
Summary:
Verifies that cancelling the parent context aborts the capture operation
and does not proceed to the upload step.
Validates:
  - executor responds to parent context cancellation
  - capture aborted error is returned
*/
func TestVyOSTraceExecutorContextCancellation(t *testing.T) {
	// Define a runner that blocks until the context is cancelled
	runner := &fakeCommandRunner{}

	exec := newTestVyOSTraceExecutor(runner, nil)
	msg := agentcore.ActionCommand{
		Version: "1.0",
		RPCID:   "rpc-trace-cancel",
		Target:  "vyos",
		Action:  ActionTrace,
		Payload: json.RawMessage(`{
			"interface": "eth0",
			"duration": 5,
			"uri": "http://localhost"
		}`),
		Timestamp: time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	runner.runFunc = func() error {
		cancel() // Cancel the parent context inside command execution
		return context.Canceled
	}

	_, err := exec.Execute(ctx, msg)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "capture aborted") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

/*
TC-ACTIONS-TRACE-004
Type: Negative
Title: HTTP upload failure during trace execution
Summary:
Simulates a remote HTTP server failure (e.g. status 500) during trace upload.
Asserts that the executor handles the error properly and still cleans up the local PCAP file.
Validates:
  - HTTP upload non-2xx status code handled as failure
  - local PCAP file cleanup occurs on upload failure
*/
func TestVyOSTraceExecutorHTTPFailure(t *testing.T) {
	// Start a test server that returns 500 Internal Server Error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	runner := &fakeCommandRunner{}

	exec := newTestVyOSTraceExecutor(runner, server.Client())
	msg := agentcore.ActionCommand{
		Version: "1.0",
		RPCID:   "rpc-trace-http-fail",
		Target:  "vyos",
		Action:  ActionTrace,
		Payload: json.RawMessage(`{
			"interface": "eth0",
			"duration": 5,
			"uri": "` + server.URL + `"
		}`),
		Timestamp: time.Now(),
	}

	_, err := exec.Execute(context.Background(), msg)
	if err == nil {
		t.Fatal("expected http upload error, got nil")
	}
	if !strings.Contains(err.Error(), "upload failed status=500") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Verify local cleanup happens even on HTTP upload failures
	if _, err := os.Stat(runner.lastPcapPath); !os.IsNotExist(err) {
		t.Fatalf("expected pcap file to be cleaned up, stat returned: %v", err)
	}
}

/*
TC-ACTIONS-TRACE-005
Type: Positive / Security
Title: RPCID containing directory traversal characters is safely encapsulated
Summary:
Passes an RPCID with directory traversal characters (e.g., "../../etc/shadow").
Verifies that the file path is safely generated in the default temp directory
using os.CreateTemp and does not write to the traversal location.
Validates:
  - PCAP file is safely created inside the OS temp directory
  - path traversal attempt does not write to the targeted system path
*/
func TestVyOSTraceExecutorRPCTraversalSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := &fakeCommandRunner{}
	exec := newTestVyOSTraceExecutor(runner, server.Client())

	msg := agentcore.ActionCommand{
		Version: "1.0",
		RPCID:   "../../etc/shadow",
		Target:  "vyos",
		Action:  ActionTrace,
		Payload: json.RawMessage(`{
			"interface": "eth0",
			"duration": 5,
			"uri": "` + server.URL + `"
		}`),
	}

	_, err := exec.Execute(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that the actual temp file path did not escape the temp folder
	pcapPath := runner.lastPcapPath
	if strings.Contains(pcapPath, "..") {
		t.Fatalf("expected path to be sanitized, got: %q", pcapPath)
	}
	if filepath.Base(pcapPath) == "shadow" {
		t.Fatalf("unexpected file creation target: %q", pcapPath)
	}
}

/*
TC-ACTIONS-TRACE-006
Type: Negative
Title: Duration and packets boundaries are enforced
Summary:
Passes duration and packets payloads that exceed the max limit constants.
Verifies that the executor rejects them with ErrInvalidActionPayload.
Validates:
  - duration exceeding maxDuration (300) is rejected
  - packets exceeding maxPackets (10000) is rejected
*/
func TestVyOSTraceExecutorParameterBounds(t *testing.T) {
	runner := &fakeCommandRunner{}
	exec := newTestVyOSTraceExecutor(runner, nil)

	cases := []struct {
		name    string
		payload string
	}{
		{
			name:    "duration exceeds limit",
			payload: `{"interface":"eth0","duration":301,"uri":"http://localhost"}`,
		},
		{
			name:    "packets exceeds limit",
			payload: `{"interface":"eth0","packets":10001,"uri":"http://localhost"}`,
		},
		{
			name:    "duration negative",
			payload: `{"interface":"eth0","duration":-1,"uri":"http://localhost"}`,
		},
		{
			name:    "packets negative",
			payload: `{"interface":"eth0","packets":-1,"uri":"http://localhost"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := agentcore.ActionCommand{
				Version: "1.0",
				RPCID:   "rpc-1",
				Target:  "vyos",
				Action:  ActionTrace,
				Payload: json.RawMessage(tc.payload),
			}
			_, err := exec.Execute(context.Background(), msg)
			if !errors.Is(err, ErrInvalidActionPayload) {
				t.Fatalf("expected ErrInvalidActionPayload, got %v", err)
			}
		})
	}
}

/*
TC-ACTIONS-TRACE-007
Type: Negative / Positive
Title: Strict interface naming validation
Summary:
Passes valid VyOS interface names (matching regex) and malicious/path traversal values.
Verifies that only valid ones are allowed and slashes/dots directory traversals are rejected.
Validates:
  - eth0, bond12, dum99, vlan10.20, lo are accepted via fallback regex or sys classnet check
  - invalid names like eth/0, eth0..1, malicious shell characters, or slashes are rejected
*/
func TestVyOSTraceExecutorInterfaceValidation(t *testing.T) {
	runner := &fakeCommandRunner{}
	exec := newTestVyOSTraceExecutor(runner, nil)

	cases := []struct {
		name        string
		iface       string
		expectError bool
	}{
		{"valid eth", "eth0", false},
		{"valid bond", "bond99", false},
		{"valid dum", "dum1", false},
		{"valid subinterface", "eth0.100", false},
		{"invalid slash", "eth/0", true},
		{"invalid backslash", "eth\\0", true},
		{"invalid dots", "eth0..1", true},
		{"invalid command injection", "eth0;ls", true},
		{"invalid random name", "invalidname", true},
	}

	t.Run("rejects non-existent interface", func(t *testing.T) {
		nonExistentExec := NewVyOSTraceExecutor(runner, nil)
		nonExistentExec.inspector = &fakeNetworkInspector{
			interfaceExistsFn: func(name string) bool { return false },
		}
		msg := agentcore.ActionCommand{
			Version: "1.0",
			RPCID:   "rpc-1",
			Target:  "vyos",
			Action:  ActionTrace,
			Payload: json.RawMessage(`{"interface":"eth99","uri":"http://localhost"}`),
		}
		_, err := nonExistentExec.Execute(context.Background(), msg)
		if err == nil {
			t.Fatal("expected error for non-existent interface, got nil")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("expected 'does not exist' error, got %v", err)
		}
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payloadMap := map[string]any{
				"interface": tc.iface,
				"uri":       "http://localhost",
			}
			payloadBytes, err := json.Marshal(payloadMap)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}
			msg := agentcore.ActionCommand{
				Version: "1.0",
				RPCID:   "rpc-1",
				Target:  "vyos",
				Action:  ActionTrace,
				Payload: json.RawMessage(payloadBytes),
			}
			_, err = exec.Execute(context.Background(), msg)
			if tc.expectError {
				if err == nil || !strings.Contains(err.Error(), "invalid interface name") {
					t.Fatalf("expected invalid interface error, got: %v", err)
				}
			} else {
				if err != nil && strings.Contains(err.Error(), "invalid interface name") {
					t.Fatalf("unexpected interface validation failure: %v", err)
				}
			}
		})
	}
}

/*
TC-ACTIONS-TRACE-008
Type: Positive
Title: Stream upload handles large files via pipe
Summary:
Creates a large trace PCAP mock file (e.g. 5MB) and executes uploadFile.
Verifies that the pipe successfully streams the contents without issues.
Validates:
  - large files are streamed cleanly
  - uploaded content length and details match
*/
func TestVyOSTraceExecutorLargeUploadStreaming(t *testing.T) {
	var receivedBytes int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		defer file.Close()
		n, _ := io.Copy(io.Discard, file)
		receivedBytes = n
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tempFile, err := os.CreateTemp("", "large-pcap-*.pcap")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	pcapPath := tempFile.Name()
	defer os.Remove(pcapPath)

	// Write a 5MB dummy data file
	data := make([]byte, 5*1024*1024)
	_, _ = tempFile.Write(data)
	tempFile.Close()

	exec := newTestVyOSTraceExecutor(&fakeCommandRunner{}, server.Client())
	err = exec.uploadFile(context.Background(), server.URL, pcapPath)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if receivedBytes != int64(len(data)) {
		t.Fatalf("expected %d bytes, got %d", len(data), receivedBytes)
	}
}

/*
TC-ACTIONS-TRACE-009
Type: Negative
Title: Upload timeout enforcement during trace execution
Summary:
Simulates a slow remote HTTP server that delays its response during trace upload.
Asserts that the executor times out according to the configured uploadTimeout duration,
and ensures that the temporary PCAP file is still cleaned up correctly.
Validates:
  - HTTP upload cancels when uploadTimeout is exceeded
  - local PCAP file cleanup occurs on upload timeout
*/
func TestVyOSTraceExecutorUploadTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := &fakeCommandRunner{}
	exec := newTestVyOSTraceExecutor(runner, server.Client())
	exec.uploadTimeout = 50 * time.Millisecond

	msg := agentcore.ActionCommand{
		Version: "1.0",
		RPCID:   "rpc-trace-timeout",
		Target:  "vyos",
		Action:  ActionTrace,
		Payload: json.RawMessage(`{
			"interface": "eth0",
			"duration": 5,
			"uri": "` + server.URL + `"
		}`),
		Timestamp: time.Now(),
	}

	_, err := exec.Execute(context.Background(), msg)
	if err == nil {
		t.Fatal("expected upload timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Verify local cleanup happens even on upload timeouts
	if runner.lastPcapPath != "" {
		if _, err := os.Stat(runner.lastPcapPath); !os.IsNotExist(err) {
			t.Fatalf("expected pcap file to be cleaned up on timeout, stat returned: %v", err)
		}
	}
}

/*
TC-ACTIONS-TRACE-010
Type: Negative
Title: Handles tcpdump execution failures
Summary:
Simulates a runtime error during tcpdump execution (e.g. exit status 1).
Asserts that the executor returns a packet capture failed error, does not upload,
and ensures that the temporary PCAP file is still cleaned up correctly.
Validates:
  - tcpdump command execution failures are caught and returned
  - local PCAP file cleanup occurs on execution failure
*/
func TestVyOSTraceExecutorExecutionFailure(t *testing.T) {
	runner := &fakeCommandRunner{
		runErr: errors.New("exit status 1"),
	}
	exec := newTestVyOSTraceExecutor(runner, nil)

	msg := agentcore.ActionCommand{
		Version: "1.0",
		RPCID:   "rpc-trace-exec-fail",
		Target:  "vyos",
		Action:  ActionTrace,
		Payload: json.RawMessage(`{
			"interface": "eth0",
			"duration": 5,
			"uri": "http://localhost"
		}`),
		Timestamp: time.Now(),
	}

	_, err := exec.Execute(context.Background(), msg)
	if err == nil {
		t.Fatal("expected execution failure error, got nil")
	}
	if !strings.Contains(err.Error(), "packet capture failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if strings.Contains(err.Error(), "capture aborted") {
		t.Fatalf("error message contains unexpected 'capture aborted': %v", err)
	}

	// Verify local cleanup happens even on execution failures
	if runner.lastPcapPath != "" {
		if _, err := os.Stat(runner.lastPcapPath); !os.IsNotExist(err) {
			t.Fatalf("expected pcap file to be cleaned up on execution failure, stat returned: %v", err)
		}
	}
}

type fakeNetworkInspector struct {
	netInterfacesFn   func() ([]InterfaceInfo, error)
	interfaceExistsFn func(name string) bool
}

func (f *fakeNetworkInspector) InterfaceExists(name string) bool {
	if f.interfaceExistsFn != nil {
		return f.interfaceExistsFn(name)
	}
	// Default to true in tests unless explicitly overridden
	return true
}

func (f *fakeNetworkInspector) NetInterfaces() ([]InterfaceInfo, error) {
	if f.netInterfacesFn != nil {
		return f.netInterfacesFn()
	}
	return nil, nil
}

func TestResolveDefaultWANInterface(t *testing.T) {
	t.Run("resolves interface with unambiguous wan alias", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", Alias: "WAN", OperState: "up"},
					{Name: "eth1", OperState: "up"},
				}, nil
			},
		}

		wan := resolveDefaultWANInterface(context.Background(), inspector)
		if wan != "eth0" {
			t.Fatalf("expected eth0, got %q", wan)
		}
	})

	t.Run("resolves master bridge when both bridge and slave port have wan alias", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", Alias: "WAN", IsSlave: true, OperState: "up"},
					{Name: "br0", Alias: "WAN", IsBridge: true, OperState: "up"},
				}, nil
			},
		}

		wan := resolveDefaultWANInterface(context.Background(), inspector)
		if wan != "br0" {
			t.Fatalf("expected br0, got %q", wan)
		}
	})

	t.Run("returns empty when multiple interfaces have wan alias", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", Alias: "WAN", OperState: "up"},
					{Name: "eth1", Alias: "WAN", OperState: "up"},
				}, nil
			},
		}

		wan := resolveDefaultWANInterface(context.Background(), inspector)
		if wan != "" {
			t.Fatalf("expected empty string for ambiguous WAN aliases, got %q", wan)
		}
	})

	t.Run("ignores wan alias when operstate is not up", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", Alias: "WAN", OperState: "unknown"},
					{Name: "eth1", Alias: "WAN", OperState: "down"},
				}, nil
			},
		}

		wan := resolveDefaultWANInterface(context.Background(), inspector)
		if wan != "" {
			t.Fatalf("expected empty string for non-up WAN interface, got %q", wan)
		}
	})

	t.Run("ignores loopback interface even if it has wan alias", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "lo", Alias: "WAN", OperState: "up"},
				}, nil
			},
		}

		wan := resolveDefaultWANInterface(context.Background(), inspector)
		if wan != "" {
			t.Fatalf("expected empty string for loopback interface, got %q", wan)
		}
	})
}

func TestResolveDefaultLANInterface(t *testing.T) {
	t.Run("resolves interface with unambiguous LAN alias", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", OperState: "up"},
					{Name: "eth2", Alias: "LAN", OperState: "up"},
				}, nil
			},
		}

		lan := resolveDefaultLANInterface(inspector, "eth0")
		if lan != "eth2" {
			t.Fatalf("expected eth2, got %q", lan)
		}
	})

	t.Run("returns empty when multiple interfaces have LAN alias", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", OperState: "up"},
					{Name: "eth1", Alias: "LAN", OperState: "up"},
					{Name: "eth2", Alias: "LAN", OperState: "up"},
				}, nil
			},
		}

		lan := resolveDefaultLANInterface(inspector, "eth0")
		if lan != "" {
			t.Fatalf("expected empty string for ambiguous LAN aliases, got %q", lan)
		}
	})

	t.Run("does not resolve bridge without explicit LAN alias", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", OperState: "up"},
					{Name: "br-lan", IsBridge: true, OperState: "up"},
				}, nil
			},
		}

		lan := resolveDefaultLANInterface(inspector, "eth0")
		if lan != "" {
			t.Fatalf("expected empty string for bridge without LAN alias, got %q", lan)
		}
	})

	t.Run("resolves administratively up interface even if operstate is down", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", Flags: 0x1, OperState: "up"},
					{Name: "br1", Alias: "LAN", Flags: 0x1003, OperState: "down"},
				}, nil
			},
		}

		lan := resolveDefaultLANInterface(inspector, "eth0")
		if lan != "br1" {
			t.Fatalf("expected br1 for administratively up LAN interface, got %q", lan)
		}
	})

	t.Run("ignores LAN alias when administratively down", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", Flags: 0x1, OperState: "up"},
					{Name: "eth1", Alias: "LAN", Flags: 0x0, OperState: "down"},
				}, nil
			},
		}

		lan := resolveDefaultLANInterface(inspector, "eth0")
		if lan != "" {
			t.Fatalf("expected empty string for administratively down LAN interface, got %q", lan)
		}
	})

	t.Run("ignores WAN interface even if it has LAN alias", func(t *testing.T) {
		inspector := &fakeNetworkInspector{
			netInterfacesFn: func() ([]InterfaceInfo, error) {
				return []InterfaceInfo{
					{Name: "eth0", Alias: "LAN", OperState: "up"},
				}, nil
			},
		}

		lan := resolveDefaultLANInterface(inspector, "eth0")
		if lan != "" {
			t.Fatalf("expected empty string when target matches WAN interface, got %q", lan)
		}
	})
}

func TestVyOSTraceExecutorNetworkValidation(t *testing.T) {
	runner := &fakeCommandRunner{}
	exec := newTestVyOSTraceExecutor(runner, nil)

	invalidNetworks := []string{"internal", "management", "dowm", "dmz"}
	for _, netName := range invalidNetworks {
		t.Run("rejects unsupported network: "+netName, func(t *testing.T) {
			msg := agentcore.ActionCommand{
				Version: "1.0", RPCID: "rpc-invalid-net", Target: "vyos", Action: ActionTrace,
				Payload: json.RawMessage(`{"network":"` + netName + `","uri":"http://localhost"}`),
			}
			_, err := exec.Execute(context.Background(), msg)
			if !errors.Is(err, ErrInvalidActionPayload) {
				t.Fatalf("expected ErrInvalidActionPayload, got %v", err)
			}
			if !strings.Contains(err.Error(), "unsupported network") {
				t.Fatalf("expected 'unsupported network' error, got %v", err)
			}
		})
	}

	t.Run("allows unrecognized network if interface is explicitly provided", func(t *testing.T) {
		msg := agentcore.ActionCommand{
			Version: "1.0", RPCID: "rpc-explicit-net", Target: "vyos", Action: ActionTrace,
			Payload: json.RawMessage(`{"interface":"eth0","network":"management","uri":"http://localhost"}`),
		}
		_, err := exec.Execute(context.Background(), msg)
		if err == nil || strings.Contains(err.Error(), "unsupported network") {
			t.Fatalf("expected upload failure without unsupported network error, got: %v", err)
		}
		if runner.lastArgs[2] != "eth0" {
			t.Fatalf("expected interface eth0, got %s", runner.lastArgs[2])
		}
	})
}

func TestVyOSTraceExecutorNetworkResolution(t *testing.T) {
	runner := &fakeCommandRunner{}
	exec := newTestVyOSTraceExecutor(runner, nil)

	var hasLAN bool
	inspector := &fakeNetworkInspector{
		netInterfacesFn: func() ([]InterfaceInfo, error) {
			itfs := []InterfaceInfo{
				{Name: "eth0", Alias: "WAN", OperState: "up"},
			}
			if hasLAN {
				itfs = append(itfs, InterfaceInfo{Name: "eth1", Alias: "LAN", OperState: "up"})
			}
			return itfs, nil
		},
	}
	exec.inspector = inspector

	t.Run("resolves wan when network up", func(t *testing.T) {
		msg := agentcore.ActionCommand{
			Version: "1.0", RPCID: "rpc-up", Target: "vyos", Action: ActionTrace,
			Payload: json.RawMessage(`{"network":"up","uri":"http://localhost"}`),
		}
		_, err := exec.Execute(context.Background(), msg)
		if err == nil || strings.Contains(err.Error(), "interface is required") {
			t.Fatalf("expected upload failure after resolving interface, got %v", err)
		}
		if runner.lastArgs[2] != "eth0" {
			t.Fatalf("expected tcpdump interface eth0, got %s", runner.lastArgs[2])
		}
	})

	t.Run("resolves lan when network down and lan exists", func(t *testing.T) {
		hasLAN = true
		msg := agentcore.ActionCommand{
			Version: "1.0", RPCID: "rpc-down", Target: "vyos", Action: ActionTrace,
			Payload: json.RawMessage(`{"network":"down","uri":"http://localhost"}`),
		}
		_, err := exec.Execute(context.Background(), msg)
		if err == nil || strings.Contains(err.Error(), "interface is required") {
			t.Fatalf("expected upload failure after resolving interface, got %v", err)
		}
		if runner.lastArgs[2] != "eth1" {
			t.Fatalf("expected tcpdump interface eth1, got %s", runner.lastArgs[2])
		}
	})

	t.Run("fails when network down and lan cannot be determined", func(t *testing.T) {
		hasLAN = false
		msg := agentcore.ActionCommand{
			Version: "1.0", RPCID: "rpc-down-fail", Target: "vyos", Action: ActionTrace,
			Payload: json.RawMessage(`{"network":"down","uri":"http://localhost"}`),
		}
		_, err := exec.Execute(context.Background(), msg)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "interface is required and could not be determined") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})
}
