package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
)

const (
	maxDuration         = 300
	maxPackets          = 10000
	maxInterfaceNameLen = 15 // Linux IFNAMSIZ - 1
)

var (
	safeInterfaceRegex = regexp.MustCompile(`^[a-zA-Z0-9_.:@-]+$`)
)

type Command interface {
	Run() error
}

type CommandRunner interface {
	Command(ctx context.Context, name string, args ...string) Command
}

type realCommand struct {
	cmd *exec.Cmd
}

func (c *realCommand) Run() error {
	return c.cmd.Run()
}

type RealCommandRunner struct{}

func NewRealCommandRunner() CommandRunner {
	return &RealCommandRunner{}
}

func (r *RealCommandRunner) Command(ctx context.Context, name string, args ...string) Command {
	return &realCommand{
		cmd: exec.CommandContext(ctx, name, args...),
	}
}

type NetworkInspector interface {
	NetInterfaces() ([]InterfaceInfo, error)
	InterfaceExists(name string) bool
}

type InterfaceInfo struct {
	Name    string
	Alias   string
	IsSlave bool
}

type systemNetworkInspector struct{}

func (s *systemNetworkInspector) InterfaceExists(name string) bool {
	_, err := os.Stat(filepath.Join("/sys/class/net", name))
	return err == nil
}

func (s *systemNetworkInspector) NetInterfaces() ([]InterfaceInfo, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, fmt.Errorf("read /sys/class/net: %w", err)
	}

	var res []InterfaceInfo
	for _, entry := range entries {
		name := entry.Name()
		if name == "lo" {
			continue
		}
		var alias string
		aliasBytes, err := os.ReadFile(filepath.Join("/sys/class/net", name, "ifalias"))
		if err == nil {
			alias = strings.TrimSpace(string(aliasBytes))
		}

		_, errMaster := os.Stat(filepath.Join("/sys/class/net", name, "master"))
		isSlave := (errMaster == nil)

		res = append(res, InterfaceInfo{
			Name:    name,
			Alias:   alias,
			IsSlave: isSlave,
		})
	}
	return res, nil
}

type VyOSTraceExecutor struct {
	runner        CommandRunner
	client        *http.Client
	uploadTimeout time.Duration
	inspector     NetworkInspector
	routeResolver func(network string) string
}

func NewVyOSTraceExecutor(runner CommandRunner, client *http.Client) *VyOSTraceExecutor {
	if client == nil {
		client = http.DefaultClient
	}
	return &VyOSTraceExecutor{
		runner:        runner,
		client:        client,
		uploadTimeout: 30 * time.Second,
		inspector:     &systemNetworkInspector{},
		routeResolver: nil,
	}
}

type tracePayload struct {
	Interface string `json:"interface"`
	Network   string `json:"network"`
	Duration  int    `json:"duration"`
	Packets   int    `json:"packets"`
	URI       string `json:"uri"`
}

// resolveInterfaceByAlias searches available interfaces for an interface declaring targetAlias (case-insensitive).
// It strictly requires that:
// 1. The interface is not loopback (lo) and not a slave port (e.g. member of a bridge).
// 2. Exactly one interface declares this alias (unambiguous candidate). If 0 or >1 candidates match, it returns "".
func resolveInterfaceByAlias(inspector NetworkInspector, targetAlias string) string {
	if inspector == nil {
		inspector = &systemNetworkInspector{}
	}

	itfs, err := inspector.NetInterfaces()
	if err != nil {
		return ""
	}

	var candidates []string
	for _, iface := range itfs {
		if iface.Name == "lo" || iface.IsSlave {
			continue
		}
		if strings.EqualFold(iface.Alias, targetAlias) {
			candidates = append(candidates, iface.Name)
		}
	}

	if len(candidates) == 1 {
		return candidates[0]
	}

	return ""
}

// resolveDefaultWANInterface resolves the WAN interface strictly via alias "WAN".
func resolveDefaultWANInterface(inspector NetworkInspector) string {
	return resolveInterfaceByAlias(inspector, "wan")
}

// resolveDefaultLANInterface resolves the LAN interface strictly via alias "LAN".
func resolveDefaultLANInterface(inspector NetworkInspector) string {
	return resolveInterfaceByAlias(inspector, "lan")
}

func (e *VyOSTraceExecutor) resolveNetwork(ctx context.Context, network string) (string, error) {
	if e.routeResolver != nil {
		resolved := e.routeResolver(network)
		if resolved == "" {
			if network != "" {
				return "", fmt.Errorf("interface is required and could not be determined for network: %s", network)
			}
			return "", errors.New("interface is required and could not be determined")
		}
		return resolved, nil
	}

	inspector := e.inspector
	if inspector == nil {
		inspector = &systemNetworkInspector{}
	}

	netLower := strings.ToLower(strings.TrimSpace(network))
	if netLower == "down" || netLower == "lan" {
		lan := resolveDefaultLANInterface(inspector)
		if lan == "" {
			return "", fmt.Errorf("interface is required and could not be determined for network: %s", network)
		}
		return lan, nil
	}

	wan := resolveDefaultWANInterface(inspector)
	if wan == "" {
		if network != "" {
			return "", fmt.Errorf("interface is required and could not be determined for network: %s", network)
		}
		return "", errors.New("interface is required and could not be determined")
	}
	return wan, nil
}

func (e *VyOSTraceExecutor) Execute(ctx context.Context, msg agentcore.ActionCommand) (Output, error) {
	if ctx == nil {
		return Output{}, errors.New("execute action: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, fmt.Errorf("execute action: %w", err)
	}
	if msg.Target == "" {
		return Output{}, errors.New("execute action: target is empty")
	}
	if msg.Action != ActionTrace {
		return Output{}, fmt.Errorf("execute action: unsupported action %q", msg.Action)
	}
	if msg.RPCID == "" {
		return Output{}, errors.New("execute action: rpc id is empty")
	}
	if len(msg.Payload) == 0 || !json.Valid(msg.Payload) {
		return Output{}, fmt.Errorf("%w: payload must be valid json", ErrInvalidActionPayload)
	}

	var payload tracePayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return Output{}, fmt.Errorf("%w: failed to unmarshal payload: %v", ErrInvalidActionPayload, err)
	}

	// Validate parameter bounds
	if payload.Duration < 0 || payload.Duration > maxDuration {
		return Output{}, fmt.Errorf("%w: duration %d must be between 0 and %d seconds", ErrInvalidActionPayload, payload.Duration, maxDuration)
	}
	if payload.Packets < 0 || payload.Packets > maxPackets {
		return Output{}, fmt.Errorf("%w: packets %d must be between 0 and %d", ErrInvalidActionPayload, payload.Packets, maxPackets)
	}

	// Resolve interface if omitted
	interfaceName := strings.TrimSpace(payload.Interface)
	if len(interfaceName) == 0 {
		// Validate network parameter only when interface resolution is required
		netRaw := strings.TrimSpace(payload.Network)
		netLower := strings.ToLower(netRaw)
		switch netLower {
		case "", "up", "wan", "down", "lan":
			// valid
		default:
			return Output{}, fmt.Errorf("%w: unsupported network %q (allowed: up, wan, down, lan)", ErrInvalidActionPayload, payload.Network)
		}

		var err error
		interfaceName, err = e.resolveNetwork(ctx, payload.Network)
		if err != nil {
			return Output{}, fmt.Errorf("%w: %v", ErrInvalidActionPayload, err)
		}
	}
	if len(interfaceName) == 0 {
		return Output{}, fmt.Errorf("%w: interface is required and could not be determined", ErrInvalidActionPayload)
	}
	if len(interfaceName) > maxInterfaceNameLen {
		return Output{}, fmt.Errorf("%w: interface name %q exceeds maximum length (%d)", ErrInvalidActionPayload, interfaceName, maxInterfaceNameLen)
	}
	if strings.Contains(interfaceName, "..") || strings.Contains(interfaceName, "/") || strings.Contains(interfaceName, "\\") {
		return Output{}, fmt.Errorf("%w: invalid interface name %q", ErrInvalidActionPayload, interfaceName)
	}

	if !safeInterfaceRegex.MatchString(interfaceName) {
		return Output{}, fmt.Errorf("%w: invalid interface name %q", ErrInvalidActionPayload, interfaceName)
	}

	inspector := e.inspector
	if inspector == nil {
		inspector = &systemNetworkInspector{}
	}
	if !inspector.InterfaceExists(interfaceName) {
		return Output{}, fmt.Errorf("%w: interface %q does not exist", ErrInvalidActionPayload, interfaceName)
	}
	payload.Interface = interfaceName

	// Validate URI
	if len(payload.URI) == 0 {
		return Output{}, fmt.Errorf("%w: uri is required", ErrInvalidActionPayload)
	}
	parsedURI, err := url.Parse(payload.URI)
	if err != nil || (parsedURI.Scheme != "http" && parsedURI.Scheme != "https") {
		return Output{}, fmt.Errorf("%w: invalid upload uri %q", ErrInvalidActionPayload, payload.URI)
	}

	// Configure temporary PCAP file path securely
	tempFile, err := os.CreateTemp("", "pcap-*.pcap")
	if err != nil {
		return Output{}, fmt.Errorf("create temporary pcap file: %w", err)
	}
	pcapPath := tempFile.Name()
	tempFile.Close() // Immediately close so tcpdump can write to it

	defer func() {
		_ = os.Remove(pcapPath)
	}()

	// Build tcpdump arguments
	args := []string{"-U", "-i", payload.Interface, "-w", pcapPath}
	if payload.Packets > 0 {
		args = append(args, "-c", strconv.Itoa(payload.Packets))
	}

	// Capture duration (defaults to 60 seconds if not specified)
	durationSec := payload.Duration
	if durationSec <= 0 {
		durationSec = 60
	}
	duration := time.Duration(durationSec) * time.Second

	// Sub-context for execution timeout
	captureCtx, captureCancel := context.WithTimeout(ctx, duration)
	defer captureCancel()

	cmd := e.runner.Command(captureCtx, "/usr/bin/tcpdump", args...)
	runErr := cmd.Run()

	// Verify command outcome
	if runErr != nil {
		if errors.Is(captureCtx.Err(), context.DeadlineExceeded) {
			// Success case: capture completed because the duration timeout expired
		} else if errors.Is(ctx.Err(), context.Canceled) {
			// Operation aborted by caller context cancellation
			return Output{}, fmt.Errorf("capture aborted: %w", ctx.Err())
		} else {
			// Real execution failure (e.g. permission error, interface doesn't exist)
			return Output{}, fmt.Errorf("packet capture failed: %w", runErr)
		}
	}

	// Upload resulting PCAP file
	if err := e.uploadFile(ctx, payload.URI, pcapPath); err != nil {
		return Output{}, fmt.Errorf("upload captured file: %w", err)
	}

	resultPayload, err := json.Marshal(map[string]any{
		"executor":         "vyos_trace",
		"action":           ActionTrace,
		"target":           msg.Target,
		"status":           "completed",
		"interface":        payload.Interface,
		"duration_seconds": durationSec,
		"pcap_file":        filepath.Base(pcapPath),
	})
	if err != nil {
		return Output{}, fmt.Errorf("build trace result payload: %w", err)
	}

	return Output{
		Payload: resultPayload,
		Message: "trace action completed",
	}, nil
}

func (e *VyOSTraceExecutor) uploadFile(ctx context.Context, uri, filePath string) error {
	uploadCtx, cancel := context.WithTimeout(ctx, e.uploadTimeout)
	defer cancel()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	contentType := writer.FormDataContentType()

	go func() {
		var err error
		defer func() {
			if err != nil {
				_ = pw.CloseWithError(err)
			} else {
				_ = pw.Close()
			}
		}()

		file, err := os.Open(filePath)
		if err != nil {
			return
		}
		defer file.Close()

		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			return
		}

		if _, err = io.Copy(part, file); err != nil {
			return
		}

		if err = writer.Close(); err != nil {
			return
		}
	}()

	req, err := http.NewRequestWithContext(uploadCtx, "POST", uri, pr)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("send upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed status=%d response=%q", resp.StatusCode, string(respBody))
	}

	return nil
}
