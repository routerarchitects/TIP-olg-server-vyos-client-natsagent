package configure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/renderer"
	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/testutil"
)

/*
TC-LOGGING-001
Type: Safety
Title: Info level does not log payload
Summary:
Runs configure with a desired payload containing sensitive-looking data.
Default logging should report safe metadata such as payload size, target,
UUID, and RPC ID without logging raw payload JSON or secret values.

Validates:
  - configure succeeds
  - raw desired payload is absent from logs
  - secret-looking payload values are absent from logs
  - safe payload size metadata is present
*/
func TestLoggingInfoLevelDoesNotLogPayload(t *testing.T) {
	secret := "payload-password-secret"
	payload := json.RawMessage(`{"system":{"password":"` + secret + `"},"interfaces":[]}`)
	fixture := newLoggingConfigureFixture(t, payload, "set system host-name safe\n", DebugLogging{})

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	assertLogDoesNotContain(t, fixture.logs, string(payload))
	assertLogDoesNotContain(t, fixture.logs, secret)
	assertLogContains(t, fixture.logs, "payload_size_bytes")
}

/*
TC-LOGGING-002
Type: Safety
Title: Info level does not log rendered commands
Summary:
Runs configure with rendered command text containing sensitive-looking data.
Default info logs should include rendered size/count metadata only and must
not contain raw rendered command text.

Validates:
  - rendered commands are absent from logs
  - sensitive command fragments are absent from logs
  - safe rendered size/count metadata is present
*/
func TestLoggingInfoLevelDoesNotLogRenderedCommands(t *testing.T) {
	rendered := "set system login user admin authentication plaintext-password rendered-secret\n"
	fixture := newLoggingConfigureFixture(t, json.RawMessage(`{"interfaces":[]}`), rendered, DebugLogging{})

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	assertLogDoesNotContain(t, fixture.logs, rendered)
	assertLogDoesNotContain(t, fixture.logs, "rendered-secret")
	assertLogContains(t, fixture.logs, "rendered_size_bytes")
	assertLogContains(t, fixture.logs, "rendered_command_count")
}

/*
TC-LOGGING-004
Type: Safety
Title: Debug with payload flag does not log raw payload
Summary:
Runs configure with explicit payload debug logging enabled. The service
should emit payload metadata only when the injected debug config enables
payload logging, without writing the raw payload body.

Validates:
  - debug payload logging is explicit
  - payload size metadata is present
  - payload_body_omitted marker is present
  - raw payload key is absent
  - raw payload value is absent
*/
func TestLoggingDebugWithPayloadFlagDoesNotLogRawPayload(t *testing.T) {
	secret := "debug-payload-secret"
	payload := json.RawMessage(`{"password":"` + secret + `"}`)
	fixture := newLoggingConfigureFixture(t, payload, "set system host-name debug\n", DebugLogging{LogPayloads: true})

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	assertLogContains(t, fixture.logs, "payload_size_bytes")
	assertLogContains(t, fixture.logs, "payload_body_omitted")
	assertLogDoesNotContain(t, fixture.logs, "payload_json")
	assertLogDoesNotContain(t, fixture.logs, secret)
}

/*
TC-LOGGING-005
Type: Safety
Title: Debug without payload flag does not log payload
Summary:
Runs configure with a logger but without the explicit payload debug flag.
Debug-capable logging alone should not emit raw payload JSON.

Validates:
  - configure succeeds
  - payload_json is absent
  - raw payload value is absent
*/
func TestLoggingDebugWithoutPayloadFlagDoesNotLogPayload(t *testing.T) {
	secret := "debug-without-flag-secret"
	payload := json.RawMessage(`{"token":"` + secret + `"}`)
	fixture := newLoggingConfigureFixture(t, payload, "set system host-name no-payload\n", DebugLogging{})

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	assertLogDoesNotContain(t, fixture.logs, "payload_json")
	assertLogDoesNotContain(t, fixture.logs, secret)
}

/*
TC-LOGGING-009
Type: Load
Title: Large payload logging does not crash
Summary:
Runs configure with a large desired payload at default logging settings.
Logging should not panic or include the full large payload contents.

Validates:
  - configure completes
  - large payload contents are not logged by default
  - safe payload size metadata is logged
*/
func TestLoggingLargePayloadDoesNotCrash(t *testing.T) {
	payload := testutil.LargePayload(512)
	fixture := newLoggingConfigureFixture(t, payload, "set system host-name large\n", DebugLogging{})

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	assertLogDoesNotContain(t, fixture.logs, string(payload))
	assertLogContains(t, fixture.logs, "payload_size_bytes")
}

/*
TC-LOGGING-010
Type: Safety
Title: Large payload does not convert unnecessarily to string
Summary:
Exercises the default configure logging path with a large payload.
The test avoids brittle allocation assertions and verifies the observable
safety contract: the large payload body is not emitted in logs by default.

Validates:
  - configure completes
  - large payload body is absent from logs
*/
func TestLoggingLargePayloadDoesNotConvertUnnecessarilyToString(t *testing.T) {
	payload := testutil.LargePayload(768)
	fixture := newLoggingConfigureFixture(t, payload, "set system host-name large-safe\n", DebugLogging{})

	if err := fixture.service.Handle(context.Background(), fixture.msg); err != nil {
		t.Fatalf("handle: %v", err)
	}

	assertLogDoesNotContain(t, fixture.logs, string(payload))
}

/*
TC-LOGGING-011
Type: Safety
Title: Failure logs contain safe error context
Summary:
Runs configure with a renderer failure whose dependency error contains
sensitive-looking command text. Failure logs should include safe context
and error_code without leaking the dependency error detail.

Validates:
  - failure is logged
  - target, UUID, RPC ID, stage, and error_code are logged
  - raw payload and dependency error details are absent
*/
func TestFailureLogsContainSafeErrorContext(t *testing.T) {
	payloadSecret := "payload-secret-value"
	dependencySecret := "set system login user admin authentication plaintext-password dependency-secret"
	fixture := newLoggingConfigureFixture(t, json.RawMessage(`{"password":"`+payloadSecret+`"}`), "set system host-name ignored\n", DebugLogging{})
	fixture.renderer.Err = errors.New(dependencySecret)

	err := fixture.service.Handle(context.Background(), fixture.msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	assertLogContains(t, fixture.logs, "configure failed")
	assertLogContains(t, fixture.logs, fixture.msg.Target)
	assertLogContains(t, fixture.logs, fixture.msg.UUID)
	assertLogContains(t, fixture.logs, fixture.msg.RPCID)
	assertLogContains(t, fixture.logs, "render_failed")
	assertLogDoesNotContain(t, fixture.logs, payloadSecret)
	assertLogDoesNotContain(t, fixture.logs, dependencySecret)
}

type loggingConfigureFixture struct {
	msg      agentcore.ConfigureNotification
	logs     *testutil.LogCapture
	renderer *testutil.FakeRenderer
	service  *Service
}

func newLoggingConfigureFixture(t *testing.T, payload json.RawMessage, renderedText string, debug DebugLogging) loggingConfigureFixture {
	t.Helper()

	msg := testutil.MinimalConfigureNotification()
	msg.UUID = "cfg-logging"
	msg.RPCID = "rpc-logging"
	desired := testutil.DesiredConfig(msg.Target, msg.UUID, payload)
	client := &testutil.FakeConfigureClient{Desired: &desired}
	store := &testutil.FakeStateStore{}
	rndr := &testutil.FakeRenderer{
		Output: renderer.Output{
			Target: msg.Target,
			UUID:   msg.UUID,
			Text:   renderedText,
		},
		UseOutput: true,
	}
	apply := &testutil.FakeApplyEngine{}
	logs := &testutil.LogCapture{}

	svc, err := NewService(Dependencies{
		Client:      client,
		StateStore:  store,
		Renderer:    rndr,
		ApplyEngine: apply,
		Logger:      logs,
		Debug:       debug,
		Now: func() time.Time {
			return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return loggingConfigureFixture{
		msg:      msg,
		logs:     logs,
		renderer: rndr,
		service:  svc,
	}
}

func assertLogContains(t *testing.T, logs *testutil.LogCapture, text string) {
	t.Helper()
	if !logs.Contains(text) {
		t.Fatalf("expected logs to contain %q; entries=%s", text, formatLogEntries(logs))
	}
}

func assertLogDoesNotContain(t *testing.T, logs *testutil.LogCapture, text string) {
	t.Helper()
	if logs.Contains(text) {
		t.Fatalf("expected logs not to contain %q; entries=%s", text, formatLogEntries(logs))
	}
}

func formatLogEntries(logs *testutil.LogCapture) string {
	entries := logs.Entries()
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, entry.String())
	}
	return strings.Join(parts, "\n")
}
