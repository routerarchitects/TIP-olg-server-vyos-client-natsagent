package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
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
	maxDuration = 300
	maxPackets  = 10000
)

var (
	vyosInterfaceRegex = regexp.MustCompile(`^(eth|bond|dum|vlan|wlan|lo|br)[0-9]+(\.[0-9]+)?$`)
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
	DefaultRouteDev(ctx context.Context) (string, error)
	ProcNetRoute() ([]byte, error)
	NetInterfaces() ([]InterfaceInfo, error)
}

type InterfaceInfo struct {
	Name      string
	Alias     string
	IsBridge  bool
	OperState string
	Flags     uint32
}

type systemNetworkInspector struct{}

// parseDefaultRouteFromIPRoute parses lines from `ip -4 route show default` (or `ip route show default`)
// and returns the interface of the default route with the lowest metric.
func parseDefaultRouteFromIPRoute(out []byte) string {
	lines := strings.Split(string(out), "\n")
	var bestIface string
	bestMetric := int(^uint(0) >> 1)
	found := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "default") {
			continue
		}
		fields := strings.Fields(line)
		var dev string
		metric := 0
		for i := 0; i < len(fields); i++ {
			if fields[i] == "dev" && i+1 < len(fields) {
				dev = fields[i+1]
			}
			if fields[i] == "metric" && i+1 < len(fields) {
				if m, err := strconv.Atoi(fields[i+1]); err == nil {
					metric = m
				}
			}
		}
		if dev != "" && dev != "lo" {
			if !found || metric < bestMetric {
				bestMetric = metric
				bestIface = dev
				found = true
			}
		}
	}
	return bestIface
}

func (s *systemNetworkInspector) DefaultRouteDev(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "/sbin/ip", "-4", "route", "show", "default").Output()
	if err != nil {
		return "", err
	}
	dev := parseDefaultRouteFromIPRoute(out)
	if dev != "" {
		return dev, nil
	}
	return "", errors.New("no default route dev found in ip route output")
}

func (s *systemNetworkInspector) ProcNetRoute() ([]byte, error) {
	return os.ReadFile("/proc/net/route")
}

func (s *systemNetworkInspector) NetInterfaces() ([]InterfaceInfo, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		itfs, netErr := net.Interfaces()
		if netErr != nil {
			return nil, err
		}
		var res []InterfaceInfo
		for _, iface := range itfs {
			if (iface.Flags & net.FlagLoopback) != 0 {
				continue
			}
			res = append(res, InterfaceInfo{
				Name:  iface.Name,
				Flags: uint32(iface.Flags),
			})
		}
		return res, nil
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
		var operState string
		stateBytes, err := os.ReadFile(filepath.Join("/sys/class/net", name, "operstate"))
		if err == nil {
			operState = strings.TrimSpace(string(stateBytes))
		}
		_, errBridge := os.Stat(filepath.Join("/sys/class/net", name, "bridge"))
		isBridge := (errBridge == nil)

		var flags uint32
		flagsBytes, err := os.ReadFile(filepath.Join("/sys/class/net", name, "flags"))
		if err == nil {
			f, _ := strconv.ParseUint(strings.TrimSpace(string(flagsBytes)), 0, 32)
			flags = uint32(f)
		}

		res = append(res, InterfaceInfo{
			Name:      name,
			Alias:     alias,
			IsBridge:  isBridge,
			OperState: operState,
			Flags:     flags,
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

// parseDefaultRouteFromProcNetRoute extracts the interface with the lowest metric default route (00000000) that is RTF_UP.
func parseDefaultRouteFromProcNetRoute(content []byte) string {
	if len(content) == 0 {
		return ""
	}
	var bestIface string
	bestMetric := int(^uint(0) >> 1) // math.MaxInt

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		// /proc/net/route columns:
		// 0: Iface, 1: Destination, 2: Gateway, 3: Flags, 4: RefCnt, 5: Use, 6: Metric, 7: Mask...
		if len(fields) < 7 || fields[1] != "00000000" {
			continue
		}

		// Ensure RTF_UP (0x0001) flag is set
		flags, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil || (flags&0x0001) == 0 {
			continue
		}

		if fields[0] == "lo" {
			continue
		}

		metric, err := strconv.Atoi(fields[6])
		if err != nil {
			metric = 0
		}

		if metric < bestMetric {
			bestMetric = metric
			bestIface = fields[0]
		}
	}
	return bestIface
}

// resolveDefaultWANInterface detects the active primary WAN interface using:
// 1. Kernel default route query via `ip -4 route show default` (inspects 0.0.0.0/0, lowest metric)
// 2. /proc/net/route parsing for destination 00000000 with RTF_UP (0x1) and lowest metric
// 3. Fallback: Search interfaces for an unambiguous active interface declaring alias "WAN" (operstate "up").
func resolveDefaultWANInterface(ctx context.Context, inspector NetworkInspector) string {
	if inspector == nil {
		inspector = &systemNetworkInspector{}
	}

	// 1. Inspect kernel routing table for the default route (0.0.0.0/0)
	if dev, err := inspector.DefaultRouteDev(ctx); err == nil && dev != "" && dev != "lo" {
		return dev
	}

	// 2. Parse /proc/net/route with RTF_UP check and lowest metric tracking for default route 00000000
	if routeData, err := inspector.ProcNetRoute(); err == nil {
		if dev := parseDefaultRouteFromProcNetRoute(routeData); dev != "" {
			return dev
		}
	}

	// 3. Fallback: Search interfaces for an unambiguous active interface declaring alias "WAN"
	if itfs, err := inspector.NetInterfaces(); err == nil {
		var candidates []string
		for _, iface := range itfs {
			if iface.Name == "lo" {
				continue
			}
			// Validate suitable operational state (must be up)
			if !strings.EqualFold(iface.OperState, "up") {
				continue
			}
			if strings.EqualFold(iface.Alias, "wan") {
				candidates = append(candidates, iface.Name)
			}
		}
		// Require an unambiguous candidate
		if len(candidates) == 1 {
			return candidates[0]
		}
	}

	return ""
}

// resolveDefaultLANInterface detects a LAN interface without guessing names:
// 1. Collect all non-WAN interfaces with alias "LAN" whose operstate is usable ("up").
// 2. Require exactly one unambiguous candidate. If 0 or >1 candidates exist, return ""
//    (avoids guessing or treating arbitrary bridges as LAN).
func resolveDefaultLANInterface(inspector NetworkInspector, wanIface string) string {
	if inspector == nil {
		inspector = &systemNetworkInspector{}
	}

	itfs, err := inspector.NetInterfaces()
	if err != nil {
		return ""
	}

	// Collect matching LAN aliases with usable operational state
	var candidates []string
	for _, iface := range itfs {
		if iface.Name == "lo" || iface.Name == wanIface {
			continue
		}
		if !strings.EqualFold(iface.OperState, "up") {
			continue
		}
		if strings.EqualFold(iface.Alias, "lan") {
			candidates = append(candidates, iface.Name)
		}
	}

	// Require exactly one unambiguous candidate
	if len(candidates) == 1 {
		return candidates[0]
	}

	// Avoid guessing or treating an arbitrary bridge as LAN when unconfigured or ambiguous.
	return ""
}

// resolveNetworkInterface resolves the interface name based on network direction ("up" for WAN, "down" for LAN).
func resolveNetworkInterface(network string) string {
	inspector := &systemNetworkInspector{}
	netLower := strings.ToLower(strings.TrimSpace(network))
	if netLower == "down" || netLower == "lan" {
		wan := resolveDefaultWANInterface(context.Background(), inspector)
		return resolveDefaultLANInterface(inspector, wan)
	}
	return resolveDefaultWANInterface(context.Background(), inspector)
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
		wan := resolveDefaultWANInterface(ctx, inspector)
		lan := resolveDefaultLANInterface(inspector, wan)
		if lan == "" {
			return "", fmt.Errorf("interface is required and could not be determined for network: %s", network)
		}
		return lan, nil
	}

	wan := resolveDefaultWANInterface(ctx, inspector)
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

	// Validate network parameter
	netRaw := strings.TrimSpace(payload.Network)
	netLower := strings.ToLower(netRaw)
	switch netLower {
	case "", "up", "wan", "down", "lan":
		// valid
	default:
		return Output{}, fmt.Errorf("%w: unsupported network %q (allowed: up, down, lan)", ErrInvalidActionPayload, payload.Network)
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
		var err error
		interfaceName, err = e.resolveNetwork(ctx, payload.Network)
		if err != nil {
			return Output{}, fmt.Errorf("%w: %v", ErrInvalidActionPayload, err)
		}
	}
	if len(interfaceName) == 0 {
		return Output{}, fmt.Errorf("%w: interface is required and could not be determined", ErrInvalidActionPayload)
	}
	if strings.Contains(interfaceName, "..") || strings.Contains(interfaceName, "/") || strings.Contains(interfaceName, "\\") {
		return Output{}, fmt.Errorf("%w: invalid interface name %q", ErrInvalidActionPayload, interfaceName)
	}

	var interfaceValid bool
	if _, err := os.Stat("/sys/class/net/" + interfaceName); err == nil {
		interfaceValid = true
	} else {
		if vyosInterfaceRegex.MatchString(interfaceName) {
			interfaceValid = true
		}
	}
	if !interfaceValid {
		return Output{}, fmt.Errorf("%w: invalid interface name %q", ErrInvalidActionPayload, interfaceName)
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
