package testutil

import (
	"context"
	"strings"
	"testing"

	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/renderervyos"
)

/*
TC-TESTUTIL-FIXTURES-001
Type: Positive
Title: Minimal desired config renders with real renderer
Summary:
Builds the shared minimal desired config fixture and runs it through
the real VyOS renderer adapter. The default minimal fixture should be
safe for future tests that expect a renderable desired payload.

Validates:
  - MinimalDesiredConfig renders without error
  - rendered output contains VyOS set commands
*/
func TestMinimalDesiredConfigRendersWithRealRenderer(t *testing.T) {
	adapter, err := renderervyos.New()
	if err != nil {
		t.Fatalf("new renderer adapter: %v", err)
	}

	out, err := adapter.Render(context.Background(), MinimalDesiredConfig())
	if err != nil {
		t.Fatalf("render minimal desired config: %v", err)
	}
	if !strings.Contains(out.Text, "set interfaces bridge") {
		t.Fatalf("rendered text got=%q want bridge set commands", out.Text)
	}
}

/*
TC-TESTUTIL-FIXTURES-002
Type: Safety
Title: Placeholder desired config remains explicit
Summary:
Builds the placeholder-only desired config fixture and inspects its
payload shape directly. The fixture intentionally preserves the old
empty payload behavior under a name that limits its scope, without
depending on how a particular renderer version treats empty config.

Validates:
  - MinimalPlaceholderDesiredConfig keeps the explicit placeholder payload
  - placeholder payload stays distinct from MinimalDesiredConfig
*/
func TestMinimalPlaceholderDesiredConfigIsExplicitlyPlaceholderOnly(t *testing.T) {
	placeholder := MinimalPlaceholderDesiredConfig()
	if got := string(placeholder.Record.Payload); got != `{"interfaces":[],"services":{}}` {
		t.Fatalf("placeholder payload got=%s want explicit placeholder payload", got)
	}
	if string(placeholder.Record.Payload) == string(MinimalDesiredConfig().Record.Payload) {
		t.Fatal("placeholder fixture should remain distinct from minimal renderable desired config")
	}
}
