package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"k8s.io/apiserver/pkg/authentication/user"
)

const enforcementTestServerURL = "https://obot.example.com"

func newEnforcementTestHandler(t *testing.T) *EnforcementHandler {
	t.Helper()

	h, err := NewEnforcementHandler(enforcementTestServerURL)
	if err != nil {
		t.Fatalf("new enforcement handler: %v", err)
	}
	return h
}

func TestEnforcementDecideAllowWritesRowAndReturnsAllow(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{AllowEverything: true})

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search",
		Kind:       "mcp",
		ServerName: "docs",
		Server:     types.EnforcementDecisionServer{URL: "https://gitmcp.io/docs"},
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionAllow {
		t.Fatalf("decision = %q, want allow (reason %q)", resp.Decision, resp.Reason)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	if row.Decision != types.EnforcementDecisionAllow {
		t.Fatalf("logged decision = %q, want allow", row.Decision)
	}
	if row.MDMConfigurationID != configID {
		t.Fatalf("logged config id = %d, want %d", row.MDMConfigurationID, configID)
	}
	if row.DeviceID != "device-1" {
		t.Fatalf("logged device id = %q, want device-1", row.DeviceID)
	}
}

func TestEnforcementDecideDenyWritesRowAndReturnsDeny(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	// An empty allowlist denies everything (fail-closed default).
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search",
		Kind:       "mcp",
		ServerName: "docs",
		Server:     types.EnforcementDecisionServer{URL: "https://gitmcp.io/docs"},
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny", resp.Decision)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	if row.Decision != types.EnforcementDecisionDeny {
		t.Fatalf("logged decision = %q, want deny", row.Decision)
	}
}

func TestEnforcementDecideDisabledEnforcementAllowsWithoutLogging(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	// Enforcement disabled, with an allowlist that would otherwise deny everything.
	config, err := gatewayClient.CreateMDMConfiguration(t.Context(), 1, &gtypes.MDMConfiguration{
		EnforcementEnabled:   false,
		EnforcementAllowlist: types.EnforcementAllowlist{},
	})
	if err != nil {
		t.Fatalf("create MDM configuration: %v", err)
	}

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search",
		Kind:       "mcp",
		ServerName: "docs",
		Server:     types.EnforcementDecisionServer{URL: "https://gitmcp.io/docs"},
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, config.ID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionAllow {
		t.Fatalf("decision = %q, want allow when enforcement is disabled", resp.Decision)
	}

	// Nothing is ever buffered, so no row should appear even after a flush window.
	time.Sleep(200 * time.Millisecond)
	_, total, err := gatewayClient.GetEnforcementDecisions(t.Context(), gatewayclient.EnforcementDecisionOptions{})
	if err != nil {
		t.Fatalf("list enforcement decisions: %v", err)
	}
	if total != 0 {
		t.Fatalf("logged %d decision rows, want 0 (enforcement disabled must not log)", total)
	}
}

func TestEnforcementDecideDisabledEnforcementAllowsUnresolvedCall(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	config, err := gatewayClient.CreateMDMConfiguration(t.Context(), 1, &gtypes.MDMConfiguration{
		EnforcementEnabled:   false,
		EnforcementAllowlist: types.EnforcementAllowlist{},
	})
	if err != nil {
		t.Fatalf("create MDM configuration: %v", err)
	}

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:            "claude_code",
		Tool:             "search",
		Kind:             "mcp",
		ServerName:       "linear",
		Unresolved:       true,
		UnresolvedReason: `stdio command "node" is not a supported package runner`,
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, config.ID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionAllow {
		t.Fatalf("decision = %q, want allow when enforcement is disabled (reason %q)", resp.Decision, resp.Reason)
	}
	assertNoEnforcementDecisions(t, gatewayClient)
}

func TestEnforcementDecideUnresolvedCallIsDeniedAndLabelled(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{AllowEverything: true})

	const reason = `MCP server "linear" was not found in any Claude Code MCP configuration`
	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:            "claude_code",
		Tool:             "search_issues",
		Kind:             "mcp",
		ServerName:       "linear",
		Server:           types.EnforcementDecisionServer{Command: "npx"},
		Unresolved:       true,
		UnresolvedReason: reason,
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny for an unresolved call even under allow-everything", resp.Decision)
	}
	if resp.Reason != reason {
		t.Fatalf("response reason = %q, want the device's reason %q", resp.Reason, reason)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	if !row.Unresolved {
		t.Fatal("logged row is not marked unresolved")
	}
	if row.UnresolvedReason != reason {
		t.Fatalf("logged unresolved reason = %q, want %q", row.UnresolvedReason, reason)
	}
	if row.Reason != reason {
		t.Fatalf("logged decision reason = %q, want %q", row.Reason, reason)
	}
	// The partial identity is what makes the row actionable: the server name
	// names the target and the executable says what was about to run.
	if row.ServerName != "linear" {
		t.Fatalf("logged server name = %q, want linear", row.ServerName)
	}
	if row.Server == nil || row.Server.Command != "npx" {
		t.Fatalf("logged server = %+v, want the reported command npx", row.Server)
	}
}

func TestEnforcementDecideResolvedCallIsNotMarkedUnresolved(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{AllowEverything: true})

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:  "claude_code",
		Tool:   "search",
		Kind:   "mcp",
		Server: types.EnforcementDecisionServer{URL: "https://gitmcp.io/docs"},
		// A stale reason with the flag unset must not label the row.
		UnresolvedReason: "left over from an earlier attempt",
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	if got := decodeDecisionResponse(t, rec).Decision; got != types.EnforcementDecisionAllow {
		t.Fatalf("decision = %q, want allow", got)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	if row.Unresolved {
		t.Fatal("logged row is marked unresolved for a resolved call")
	}
	if row.UnresolvedReason != "" {
		t.Fatalf("logged unresolved reason = %q, want empty", row.UnresolvedReason)
	}
}

func TestEnforcementDecideConnectorCallRoundTrips(t *testing.T) {
	for _, tt := range []struct {
		name      string
		entry     string
		wantAllow bool
	}{
		{"exact display name", "claude.ai Linear", true},
		{"case-insensitive", "CLAUDE.AI LINEAR", true},
		{"different connector", "claude.ai Notion", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gatewayClient := newEnforcementTestGatewayClient(t)
			configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Connector: tt.entry}},
			})

			rec := httptest.NewRecorder()
			body := types.EnforcementDecisionRequest{
				Agent:      "claude_code",
				Tool:       "search_issues",
				Kind:       "mcp",
				ServerName: "claude_ai_linear",
				Server:     types.EnforcementDecisionServer{Connector: "claude.ai Linear"},
			}
			if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
				t.Fatalf("decide: %v", err)
			}

			want := types.EnforcementDecisionDeny
			if tt.wantAllow {
				want = types.EnforcementDecisionAllow
			}
			if got := decodeDecisionResponse(t, rec).Decision; got != want {
				t.Fatalf("decision = %q, want %q", got, want)
			}

			row := waitForEnforcementDecision(t, gatewayClient)
			if row.Server == nil || row.Server.Connector != "claude.ai Linear" {
				t.Fatalf("logged server = %+v, want the attested connector", row.Server)
			}
		})
	}
}

func TestSanitizeUnresolvedReason(t *testing.T) {
	long := strings.Repeat("a", maxUnresolvedReasonRunes+50)
	multibyte := strings.Repeat("é", maxUnresolvedReasonRunes+50)

	for _, tt := range []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "  \t\n ", ""},
		{"trimmed", "  not a supported runner  ", "not a supported runner"},
		{"kept whole", "npx flag --registry is not allowed", "npx flag --registry is not allowed"},
		{"truncated", long, long[:maxUnresolvedReasonRunes]},
		{"truncated on a rune boundary", multibyte, strings.Repeat("é", maxUnresolvedReasonRunes)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeUnresolvedReason(tt.raw)
			if got != tt.want {
				t.Fatalf("sanitizeUnresolvedReason(%.40q…) = %.40q…, want %.40q…", tt.raw, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("sanitizeUnresolvedReason produced invalid UTF-8")
			}
		})
	}
}

// TestTruncateRunesDoesNotCopyOversizedInput pins the reason truncateRunes walks
// byte offsets instead of converting to []rune: the device supplies these strings
// and the body limit allows megabytes, so materializing a rune slice would let a
// caller cost 4 bytes of scratch heap per input byte just to keep a 256-rune
// prefix. The returned prefix is a substring, so the bound costs no allocation.
func TestTruncateRunesDoesNotCopyOversizedInput(t *testing.T) {
	oversized := strings.Repeat("a", 1<<20)

	if got := truncateRunes(oversized, maxIdentifierRunes); got != oversized[:maxIdentifierRunes] {
		t.Fatalf("truncateRunes returned %d runes, want %d", utf8.RuneCountInString(got), maxIdentifierRunes)
	}

	if allocs := testing.AllocsPerRun(100, func() {
		_ = truncateRunes(oversized, maxIdentifierRunes)
	}); allocs != 0 {
		t.Fatalf("truncateRunes allocated %v times per call on an oversized input, want 0", allocs)
	}
}

func TestTruncateRunes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		raw     string
		maximum int
		want    string
	}{
		{"empty", "", 8, ""},
		{"under the limit", "abc", 8, "abc"},
		{"exactly at the limit", "abcd", 4, "abcd"},
		{"over the limit", "abcdef", 4, "abcd"},
		{"zero limit", "abc", 0, ""},
		{"multibyte cut on a rune boundary", "héllo wörld", 4, "héll"},
		{"multibyte kept whole", "日本語", 8, "日本語"},
		{"multibyte cut mid-string", "日本語テキスト", 3, "日本語"},
		// Runes, not grapheme clusters. Written decomposed on purpose: cutting
		// between a base letter and its combining mark is still a valid-UTF-8
		// cut, which is all an audit field needs.
		{"combining marks are counted per rune", "e\u0301e\u0301", 3, "e\u0301e"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateRunes(tt.raw, tt.maximum)
			if got != tt.want {
				t.Fatalf("truncateRunes(%q, %d) = %q, want %q", tt.raw, tt.maximum, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("truncateRunes(%q, %d) produced invalid UTF-8: %q", tt.raw, tt.maximum, got)
			}
			if n := utf8.RuneCountInString(got); n > tt.maximum {
				t.Fatalf("truncateRunes(%q, %d) returned %d runes", tt.raw, tt.maximum, n)
			}
		})
	}
}

func TestEnforcementDecideUnknownConfigurationDenies(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{Agent: "codex", Tool: "run", Kind: "shell"}
	// Point at a configuration id that does not exist.
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, 9999, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny for unknown configuration", resp.Decision)
	}
}

func TestEnforcementDecideMissingConfigurationDenies(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/enforcement/decisions",
		bytes.NewReader(mustMarshal(t, types.EnforcementDecisionRequest{Agent: "codex", Tool: "run", Kind: "shell"})))
	ctx := api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			UID:    "device:device-1",
			Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
			// No mdm_configuration_id in Extra.
			Extra: map[string][]string{"device_id": {"device-1"}},
		},
	}
	if err := newEnforcementTestHandler(t).Decide(ctx); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny when no configuration is associated", resp.Decision)
	}
	assertNoEnforcementDecisions(t, gatewayClient)
}

func TestEnforcementDecideWithoutDeviceIdentityDeniesWithoutLogging(t *testing.T) {
	for _, tt := range []struct {
		name  string
		extra map[string][]string
	}{
		{"no extra at all", nil},
		{"empty extra", map[string][]string{}},
		{"configuration id but no device id", map[string][]string{"mdm_configuration_id": {"1"}}},
		{"device id but unparseable configuration id", map[string][]string{
			"device_id":            {"device-1"},
			"mdm_configuration_id": {"not-a-number"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gatewayClient := newEnforcementTestGatewayClient(t)
			// A real, enforcement-enabled allow-everything fleet exists; the caller
			// still must not be able to borrow it or log against it.
			createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{AllowEverything: true})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/enforcement/decisions",
				bytes.NewReader(mustMarshal(t, types.EnforcementDecisionRequest{Agent: "codex", Tool: "run", Kind: "shell"})))
			ctx := api.Context{
				ResponseWriter: rec,
				Request:        req,
				GatewayClient:  gatewayClient,
				User: &user.DefaultInfo{
					UID:    "u1",
					Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
					Extra:  tt.extra,
				},
			}
			if err := newEnforcementTestHandler(t).Decide(ctx); err != nil {
				t.Fatalf("decide: %v", err)
			}

			resp := decodeDecisionResponse(t, rec)
			if resp.Decision != types.EnforcementDecisionDeny {
				t.Fatalf("decision = %q, want deny for a non-device caller", resp.Decision)
			}
			assertNoEnforcementDecisions(t, gatewayClient)
		})
	}
}

func TestEnforcementDecideIgnoresBodySuppliedConfigurationID(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	denyConfigID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})
	allowConfigID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{AllowEverything: true})

	// The authenticated identity points at the deny config, but the body tries to
	// smuggle the allow config id. The handler must use only the authenticated id.
	rawBody := fmt.Sprintf(`{"agent":"codex","tool":"run","kind":"shell","mdmConfigurationID":%d}`, allowConfigID)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/enforcement/decisions", bytes.NewReader([]byte(rawBody)))
	ctx := api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			UID:    "device:device-1",
			Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
			Extra: map[string][]string{
				"device_id":            {"device-1"},
				"mdm_configuration_id": {fmt.Sprintf("%d", denyConfigID)},
			},
		},
	}
	if err := newEnforcementTestHandler(t).Decide(ctx); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny (body config id must be ignored)", resp.Decision)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	if row.MDMConfigurationID != denyConfigID {
		t.Fatalf("logged config id = %d, want authenticated deny config %d", row.MDMConfigurationID, denyConfigID)
	}
}

func TestEnforcementDecideDeniesForeignHostUnderObotHostedToggle(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{AllowAllObotHostedMCP: true})

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search",
		Kind:       "mcp",
		ServerName: "docs",
		Server:     types.EnforcementDecisionServer{URL: "https://evil.example.com/mcp"},
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny (foreign-host call is not Obot-hosted)", resp.Decision)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	if row.ObotHosted {
		t.Fatalf("logged ObotHosted = true, want false (determined server-side)")
	}
}

// TestEnforcementObotHosted proves that a URL whose hostname matches Obot's server URL is treated as Obot-hosted and is
// allowed under AllowAllObotHostedMCP.
func TestEnforcementObotHosted(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{AllowAllObotHostedMCP: true})

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search",
		Kind:       "mcp",
		ServerName: "docs",
		Server:     types.EnforcementDecisionServer{URL: "https://obot.example.com/mcp/foo"},
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionAllow {
		t.Fatalf("decision = %q, want allow (matching-host URL is Obot-hosted)", resp.Decision)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	if !row.ObotHosted {
		t.Fatalf("logged ObotHosted = false, want true (recomputed server-side)")
	}
}

func TestEnforcementObotHostedRequiresFullOrigin(t *testing.T) {
	// enforcementTestServerURL is https://obot.example.com (implicit port 443).
	for _, tt := range []struct {
		name       string
		callURL    string
		obotHosted bool
	}{
		{"exact origin", "https://obot.example.com/mcp/foo", true},
		{"explicit default port", "https://obot.example.com:443/mcp/foo", true},
		{"host case-insensitive", "https://OBOT.EXAMPLE.COM/mcp/foo", true},
		{"different port", "https://obot.example.com:8443/mcp/foo", false},
		{"scheme mismatch", "http://obot.example.com/mcp/foo", false},
		{"scheme mismatch with explicit port", "http://obot.example.com:443/mcp/foo", false},
		{"foreign host", "https://evil.example.com/mcp/foo", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gatewayClient := newEnforcementTestGatewayClient(t)
			configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{AllowAllObotHostedMCP: true})

			rec := httptest.NewRecorder()
			body := types.EnforcementDecisionRequest{
				Agent:      "claude_code",
				Tool:       "search",
				Kind:       "mcp",
				ServerName: "docs",
				Server:     types.EnforcementDecisionServer{URL: tt.callURL},
			}
			if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
				t.Fatalf("decide: %v", err)
			}

			want := types.EnforcementDecisionDeny
			if tt.obotHosted {
				want = types.EnforcementDecisionAllow
			}
			if got := decodeDecisionResponse(t, rec).Decision; got != want {
				t.Fatalf("decision = %q, want %q for %s", got, want, tt.callURL)
			}

			if row := waitForEnforcementDecision(t, gatewayClient); row.ObotHosted != tt.obotHosted {
				t.Fatalf("logged ObotHosted = %v, want %v for %s", row.ObotHosted, tt.obotHosted, tt.callURL)
			}
		})
	}
}

// TestEnforcementDecideRecordsOnlyDecisionRelevantURLParts proves the decision
// log keeps the matched origin and path of a device-reported URL while the
// credential-bearing parts — userinfo, query string, fragment — never reach the
// database.
func TestEnforcementDecideRecordsOnlyDecisionRelevantURLParts(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{
		Servers: []types.AllowlistServer{{URL: "https://gitmcp.io/docs"}},
	})

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search",
		Kind:       "mcp",
		ServerName: "docs",
		Server:     types.EnforcementDecisionServer{URL: "https://user:tok-secret@gitmcp.io/docs?api_key=sk-secret#frag"},
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	if got := decodeDecisionResponse(t, rec).Decision; got != types.EnforcementDecisionAllow {
		t.Fatalf("decision = %q, want allow (query string must not affect matching)", got)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	if row.Server == nil {
		t.Fatal("logged row has no server identity")
	}
	if want := "https://gitmcp.io/docs"; row.Server.URL != want {
		t.Fatalf("logged server URL = %q, want %q", row.Server.URL, want)
	}
	if row.Server.Hostname != "gitmcp.io" {
		t.Fatalf("logged server hostname = %q, want gitmcp.io", row.Server.Hostname)
	}
	// Nothing secret may survive anywhere in the persisted row.
	for _, secret := range []string{"tok-secret", "sk-secret", "api_key", "frag"} {
		if serialized := string(mustMarshal(t, row)); strings.Contains(serialized, secret) {
			t.Fatalf("logged row leaks %q: %s", secret, serialized)
		}
	}
}

// TestEnforcementDecideRecordsPackageIdentityWithoutCommandArguments proves that
// for an npm/pypi package server the row keeps the source/name/version the
// evaluator compared, and reduces the launch command to its executable.
func TestEnforcementDecideRecordsPackageIdentityWithoutCommandArguments(t *testing.T) {
	for _, tt := range []struct {
		name    string
		source  types.AllowlistServerPackageSource
		command string
	}{
		{"npm", types.AllowlistServerPackageSourceNPM, "npx -y @scope/server --api-key=sk-secret"},
		{"pypi", types.AllowlistServerPackageSourcePyPI, "env TOKEN=tok-secret uvx some-server --token sk-secret"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gatewayClient := newEnforcementTestGatewayClient(t)
			configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{
					Package: &types.AllowlistServerPackage{Source: tt.source, Name: "@scope/server", Version: "1.2.3"},
				}},
			})

			rec := httptest.NewRecorder()
			body := types.EnforcementDecisionRequest{
				Agent: "claude_code",
				Tool:  "search",
				Kind:  "mcp",
				Server: types.EnforcementDecisionServer{
					Package: &types.AllowlistServerPackage{Source: tt.source, Name: "@scope/server", Version: "1.2.3"},
					Command: tt.command,
				},
			}
			if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
				t.Fatalf("decide: %v", err)
			}

			if got := decodeDecisionResponse(t, rec).Decision; got != types.EnforcementDecisionAllow {
				t.Fatalf("decision = %q, want allow (package identity matches)", got)
			}

			row := waitForEnforcementDecision(t, gatewayClient)
			if row.Server == nil || row.Server.Package == nil {
				t.Fatalf("logged row lost its package identity: %+v", row.Server)
			}
			// The dimensions the decision compared are recorded in full.
			if row.Server.Package.Source != tt.source {
				t.Fatalf("logged package source = %q, want %q", row.Server.Package.Source, tt.source)
			}
			if row.Server.Package.Name != "@scope/server" {
				t.Fatalf("logged package name = %q, want @scope/server", row.Server.Package.Name)
			}
			if row.Server.Package.Version != "1.2.3" {
				t.Fatalf("logged package version = %q, want 1.2.3", row.Server.Package.Version)
			}
			// The command keeps its executable and nothing else.
			if want := strings.Fields(tt.command)[0]; row.Server.Command != want {
				t.Fatalf("logged command = %q, want %q (arguments must be dropped)", row.Server.Command, want)
			}
			for _, secret := range []string{"sk-secret", "tok-secret", "--api-key", "--token"} {
				if serialized := string(mustMarshal(t, row)); strings.Contains(serialized, secret) {
					t.Fatalf("logged row leaks %q: %s", secret, serialized)
				}
			}
		})
	}
}

func TestSanitizeServerURL(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"already clean", "https://gitmcp.io/docs", "https://gitmcp.io/docs"},
		{"explicit port kept", "https://gitmcp.io:8443/docs", "https://gitmcp.io:8443/docs"},
		{"trailing slash kept", "https://gitmcp.io/docs/", "https://gitmcp.io/docs/"},
		{"query dropped", "https://gitmcp.io/docs?api_key=secret", "https://gitmcp.io/docs"},
		{"bare query marker dropped", "https://gitmcp.io/docs?", "https://gitmcp.io/docs"},
		{"fragment dropped", "https://gitmcp.io/docs#secret", "https://gitmcp.io/docs"},
		{"userinfo dropped", "https://user:pass@gitmcp.io/docs", "https://gitmcp.io/docs"},
		{"user only dropped", "https://user@gitmcp.io/docs", "https://gitmcp.io/docs"},
		{"all three dropped", "https://user:pass@gitmcp.io/docs?k=v#f", "https://gitmcp.io/docs"},
		{"escaped path preserved", "https://gitmcp.io/a%20b?k=v", "https://gitmcp.io/a%20b"},
		{"unparseable cut at query", "https://exa mple.com/docs?k=secret", "https://exa mple.com/docs"},
		{"unparseable without query kept", "https://exa mple.com/docs", "https://exa mple.com/docs"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeServerURL(tt.raw); got != tt.want {
				t.Fatalf("sanitizeServerURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestSanitizeServerCommand(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   \t ", ""},
		{"bare executable", "npx", "npx"},
		{"flags dropped", "npx -y @scope/server --api-key=sk-secret", "npx"},
		{"inline env dropped", "env TOKEN=sk-secret uvx some-server", "env"},
		{"absolute path kept", "/usr/local/bin/my-server --token sk-secret", "/usr/local/bin/my-server"},
		{"leading whitespace ignored", "  uvx  some-server  ", "uvx"},
		{"tab separated", "node\tserver.js\t--key=sk-secret", "node"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeServerCommand(tt.raw); got != tt.want {
				t.Fatalf("sanitizeServerCommand(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseEnforcementDecisionOptionsConfigurationID(t *testing.T) {
	for _, tt := range []struct {
		name  string
		query string
		want  []uint
	}{
		{"absent", "", nil},
		{"empty value", "mdm_configuration_id=", nil},
		{"single", "mdm_configuration_id=7", []uint{7}},
		{"zero", "mdm_configuration_id=0", []uint{0}},
		{"repeated", "mdm_configuration_id=3&mdm_configuration_id=5", []uint{3, 5}},
		{"comma separated", "mdm_configuration_id=3,5,11", []uint{3, 5, 11}},
		{"whitespace padded", "mdm_configuration_id=%203%20,%205%20", []uint{3, 5}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			opts, err := parseEnforcementDecisionOptions(query)
			if err != nil {
				t.Fatalf("parseEnforcementDecisionOptions: %v", err)
			}
			if !slices.Equal(opts.MDMConfigurationID, tt.want) {
				t.Fatalf("MDMConfigurationID = %v, want %v", opts.MDMConfigurationID, tt.want)
			}
		})
	}

	for _, tt := range []struct {
		name  string
		query string
	}{
		{"non-numeric", "mdm_configuration_id=abc"},
		{"sql-ish", "mdm_configuration_id=1%3Bdrop"},
		{"fractional", "mdm_configuration_id=1.5"},
		{"negative", "mdm_configuration_id=-1"},
		{"overflows uint64", "mdm_configuration_id=99999999999999999999"},
		{"one bad value among good ones", "mdm_configuration_id=3,abc"},
	} {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			_, err = parseEnforcementDecisionOptions(query)
			if err == nil {
				t.Fatalf("parseEnforcementDecisionOptions(%q) = nil error, want a bad-request error", tt.query)
			}
			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) {
				t.Fatalf("error is %T, want *types.ErrHTTP: %v", err, err)
			}
			if httpErr.Code != http.StatusBadRequest {
				t.Fatalf("error HTTP code = %d, want %d (err: %v)", httpErr.Code, http.StatusBadRequest, err)
			}
		})
	}
}

// --- helpers ---

func newEnforcementTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("create storage services: %v", err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("migrate gateway db: %v", err)
	}
	c := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func createEnforcementTestConfig(t *testing.T, c *gatewayclient.Client, allowlist types.EnforcementAllowlist) uint {
	t.Helper()
	config, err := c.CreateMDMConfiguration(t.Context(), 1, &gtypes.MDMConfiguration{
		EnforcementEnabled:   true,
		EnforcementAllowlist: allowlist,
	})
	if err != nil {
		t.Fatalf("create MDM configuration: %v", err)
	}
	return config.ID
}

func newEnforcementDeviceContext(t *testing.T, c *gatewayclient.Client, body types.EnforcementDecisionRequest, configID uint, rec *httptest.ResponseRecorder) api.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/enforcement/decisions", bytes.NewReader(mustMarshal(t, body)))
	return api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  c,
		User: &user.DefaultInfo{
			UID:    "device:device-1",
			Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
			Extra: map[string][]string{
				"device_id":            {"device-1"},
				"mdm_configuration_id": {fmt.Sprintf("%d", configID)},
			},
		},
	}
}

func decodeDecisionResponse(t *testing.T, rec *httptest.ResponseRecorder) types.EnforcementDecisionResponse {
	t.Helper()
	var resp types.EnforcementDecisionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode decision response: %v (body %s)", err, rec.Body.String())
	}
	return resp
}

func waitForEnforcementDecision(t *testing.T, c *gatewayclient.Client) types.EnforcementDecisionEvent {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := c.GetEnforcementDecisions(t.Context(), gatewayclient.EnforcementDecisionOptions{})
		if err != nil {
			t.Fatalf("list enforcement decisions: %v", err)
		}
		if total >= 1 && len(logs) >= 1 {
			return presentEnforcementDecision(logs[0])
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for enforcement decision row")
	return types.EnforcementDecisionEvent{}
}

// assertNoEnforcementDecisions fails if any decision row is recorded.
func assertNoEnforcementDecisions(t *testing.T, c *gatewayclient.Client) {
	t.Helper()
	time.Sleep(200 * time.Millisecond)
	logs, total, err := c.GetEnforcementDecisions(t.Context(), gatewayclient.EnforcementDecisionOptions{})
	if err != nil {
		t.Fatalf("list enforcement decisions: %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("recorded %d enforcement decision row(s), want 0", max(total, int64(len(logs))))
	}
}

func assertEnforcementDecisionCount(t *testing.T, c *gatewayclient.Client, want int64) {
	t.Helper()
	time.Sleep(200 * time.Millisecond)
	_, total, err := c.GetEnforcementDecisions(t.Context(), gatewayclient.EnforcementDecisionOptions{})
	if err != nil {
		t.Fatalf("list enforcement decisions: %v", err)
	}
	if total != want {
		t.Fatalf("recorded %d enforcement decision row(s), want %d", total, want)
	}
}

func newEnforcementAdminContext(t *testing.T, c *gatewayclient.Client, id string, rec *httptest.ResponseRecorder) api.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/enforcement-decisions/allowlist-check/"+id, nil)
	ctx := api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  c,
		User: &user.DefaultInfo{
			UID:    "42",
			Groups: []string{types.GroupAuthenticated, types.GroupAdmin},
		},
	}
	ctx.SetPathValue("id", id)
	return ctx
}

func decodeAllowlistCheck(t *testing.T, rec *httptest.ResponseRecorder) types.EnforcementDecisionAllowlistCheck {
	t.Helper()
	var resp types.EnforcementDecisionAllowlistCheck
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode allowlist check: %v (body %s)", err, rec.Body.String())
	}
	return resp
}

// checkEnforcementAllowlist replays the recorded decision and returns the result.
func checkEnforcementAllowlist(t *testing.T, c *gatewayclient.Client, id string) types.EnforcementDecisionAllowlistCheck {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := newEnforcementTestHandler(t).CheckDecisionAllowlist(newEnforcementAdminContext(t, c, id, rec)); err != nil {
		t.Fatalf("check decision allowlist: %v", err)
	}
	return decodeAllowlistCheck(t, rec)
}

func setEnforcementTestPolicy(t *testing.T, c *gatewayclient.Client, configID uint, enabled bool, allowlist types.EnforcementAllowlist) {
	t.Helper()
	if err := c.UpdateMDMConfigurationEnforcement(t.Context(), configID, enabled, allowlist, nil); err != nil {
		t.Fatalf("update enforcement policy: %v", err)
	}
}

// recordEnforcementDenyRow drives a real Decide against an empty allowlist so the
// row under test carries the same sanitization and truncation as production data.
func recordEnforcementDenyRow(t *testing.T, c *gatewayclient.Client, configID uint, body types.EnforcementDecisionRequest) types.EnforcementDecisionEvent {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, c, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}
	if got := decodeDecisionResponse(t, rec).Decision; got != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny for this fixture", got)
	}
	return waitForEnforcementDecision(t, c)
}

func enforcementTestMCPCall(url string) types.EnforcementDecisionRequest {
	return types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search",
		Kind:       "mcp",
		ServerName: "docs",
		Server:     types.EnforcementDecisionServer{URL: url},
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestEnforcementCheckAllowlistFlipsToAllowAfterWidening(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})
	row := recordEnforcementDenyRow(t, gatewayClient, configID, enforcementTestMCPCall("https://gitmcp.io/docs"))

	if got := checkEnforcementAllowlist(t, gatewayClient, row.ID); got.AllowlistDecision != types.EnforcementDecisionDeny {
		t.Fatalf("decision before widening = %q, want deny", got.AllowlistDecision)
	}

	setEnforcementTestPolicy(t, gatewayClient, configID, true, types.EnforcementAllowlist{
		Servers: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
	})

	got := checkEnforcementAllowlist(t, gatewayClient, row.ID)
	if got.AllowlistDecision != types.EnforcementDecisionAllow {
		t.Fatalf("decision after widening = %q, want allow (reason %q)", got.AllowlistDecision, got.AllowlistReason)
	}
	if got.ID != row.ID {
		t.Fatalf("checked id = %q, want %q", got.ID, row.ID)
	}
	if !got.EnforcementEnabled {
		t.Fatal("enforcementEnabled = false, want true")
	}
	if got.AllowlistReason == "" {
		t.Fatal("allowlistReason is empty, want the evaluator's justification")
	}
}

func TestEnforcementCheckAllowlistWritesNoDecisionRow(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})
	row := recordEnforcementDenyRow(t, gatewayClient, configID, enforcementTestMCPCall("https://gitmcp.io/docs"))
	assertEnforcementDecisionCount(t, gatewayClient, 1)

	// Replay it both ways: still denied, and then allowed. Neither is a device
	// telling us what it did, so neither belongs in the log.
	checkEnforcementAllowlist(t, gatewayClient, row.ID)
	setEnforcementTestPolicy(t, gatewayClient, configID, true, types.EnforcementAllowlist{AllowEverything: true})
	checkEnforcementAllowlist(t, gatewayClient, row.ID)

	assertEnforcementDecisionCount(t, gatewayClient, 1)
}

func TestEnforcementCheckAllowlistStaysDenyWhenThePolicyIsUnchanged(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})
	row := recordEnforcementDenyRow(t, gatewayClient, configID, enforcementTestMCPCall("https://gitmcp.io/docs"))

	got := checkEnforcementAllowlist(t, gatewayClient, row.ID)
	if got.AllowlistDecision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny", got.AllowlistDecision)
	}
	if got.AllowlistReason != "no matching allowlist entry" {
		t.Fatalf("reason = %q, want the evaluator's no-match reason", got.AllowlistReason)
	}
}

// The check answers "does a rule cover this call?", which is not the same question
// as "would this call get through?". Enforcement being switched off must never be
// reported as a rule that covers the call.
func TestEnforcementCheckAllowlistDisabledEnforcementStillReportsTheAllowlistVerdict(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})
	row := recordEnforcementDenyRow(t, gatewayClient, configID, enforcementTestMCPCall("https://gitmcp.io/docs"))

	setEnforcementTestPolicy(t, gatewayClient, configID, false, types.EnforcementAllowlist{})
	got := checkEnforcementAllowlist(t, gatewayClient, row.ID)
	if got.AllowlistDecision != types.EnforcementDecisionDeny {
		t.Fatalf("decision with enforcement off and no rule = %q, want deny", got.AllowlistDecision)
	}
	if got.EnforcementEnabled {
		t.Fatal("enforcementEnabled = true, want false")
	}

	setEnforcementTestPolicy(t, gatewayClient, configID, false, types.EnforcementAllowlist{AllowEverything: true})
	got = checkEnforcementAllowlist(t, gatewayClient, row.ID)
	if got.AllowlistDecision != types.EnforcementDecisionAllow {
		t.Fatalf("decision with enforcement off and allow-everything = %q, want allow", got.AllowlistDecision)
	}
	if got.EnforcementEnabled {
		t.Fatal("enforcementEnabled = true, want false")
	}
}

func TestEnforcementCheckAllowlistUnknownDecisionReturnsNotFound(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})

	rec := httptest.NewRecorder()
	err := newEnforcementTestHandler(t).CheckDecisionAllowlist(newEnforcementAdminContext(t, gatewayClient, "999999", rec))
	if err == nil {
		t.Fatal("check decision allowlist = nil error, want not found")
	}
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("error is %T, want *types.ErrHTTP: %v", err, err)
	}
	if httpErr.Code != http.StatusNotFound {
		t.Fatalf("error HTTP code = %d, want %d (err: %v)", httpErr.Code, http.StatusNotFound, err)
	}
}

func TestEnforcementCheckAllowlistRejectsAMalformedID(t *testing.T) {
	for _, id := range []string{"", "abc", "1.5", "-1", "99999999999999999999"} {
		t.Run(fmt.Sprintf("id=%q", id), func(t *testing.T) {
			gatewayClient := newEnforcementTestGatewayClient(t)
			rec := httptest.NewRecorder()
			err := newEnforcementTestHandler(t).CheckDecisionAllowlist(newEnforcementAdminContext(t, gatewayClient, id, rec))
			if err == nil {
				t.Fatalf("check decision allowlist(%q) = nil error, want bad request", id)
			}
			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) {
				t.Fatalf("error is %T, want *types.ErrHTTP: %v", err, err)
			}
			if httpErr.Code != http.StatusBadRequest {
				t.Fatalf("error HTTP code = %d, want %d (err: %v)", httpErr.Code, http.StatusBadRequest, err)
			}
		})
	}
}

// A call the device could not identify has nothing to match on, so no rule — not
// even allow-everything — can cover it.
func TestEnforcementCheckAllowlistUnresolvedRowStaysDeniedUnderAllowEverything(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})
	body := types.EnforcementDecisionRequest{
		Agent:            "claude_code",
		Tool:             "search",
		Kind:             "mcp",
		Unresolved:       true,
		UnresolvedReason: "no MCP configuration matched the tool",
	}
	row := recordEnforcementDenyRow(t, gatewayClient, configID, body)

	setEnforcementTestPolicy(t, gatewayClient, configID, true, types.EnforcementAllowlist{AllowEverything: true})

	got := checkEnforcementAllowlist(t, gatewayClient, row.ID)
	if got.AllowlistDecision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny for an unresolved call", got.AllowlistDecision)
	}
	if got.AllowlistReason != "no MCP configuration matched the tool" {
		t.Fatalf("reason = %q, want the recorded unresolved reason", got.AllowlistReason)
	}
}

func TestEnforcementCheckAllowlistMatchesAPackageRowAfterAllowlisting(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})
	body := types.EnforcementDecisionRequest{
		Agent: "claude_code",
		Tool:  "search",
		Kind:  "mcp",
		Server: types.EnforcementDecisionServer{
			Package: &types.AllowlistServerPackage{
				Source:  types.AllowlistServerPackageSourceNPM,
				Name:    "@scope/server",
				Version: "1.2.3",
			},
			Command: "npx -y @scope/server --api-key=sk-secret",
		},
	}
	row := recordEnforcementDenyRow(t, gatewayClient, configID, body)

	setEnforcementTestPolicy(t, gatewayClient, configID, true, types.EnforcementAllowlist{
		Servers: []types.AllowlistServer{{
			Package: &types.AllowlistServerPackage{
				Source: types.AllowlistServerPackageSourceNPM,
				Name:   "@scope/server",
			},
		}},
	})

	if got := checkEnforcementAllowlist(t, gatewayClient, row.ID); got.AllowlistDecision != types.EnforcementDecisionAllow {
		t.Fatalf("decision = %q, want allow (reason %q)", got.AllowlistDecision, got.AllowlistReason)
	}
}

func TestNormalizedCallFromDecisionLog(t *testing.T) {
	base := gtypes.EnforcementDecisionLog{
		Agent:            "claude_code",
		Tool:             "search",
		Kind:             "mcp",
		ServerName:       "docs",
		ObotHosted:       true,
		ServerURL:        "https://gitmcp.io/docs",
		ServerHostname:   "gitmcp.io",
		ServerCommand:    "npx",
		ServerConnector:  "Google Calendar",
		Unresolved:       true,
		UnresolvedReason: "no MCP configuration matched the tool",
	}

	// ObotHosted comes from the argument, never the row: the caller recomputes it
	// against Obot's current server URL.
	call := normalizedCallFromDecisionLog(base, false)
	if call.ObotHosted {
		t.Fatal("ObotHosted = true, want the argument's false rather than the row's true")
	}
	if call.Agent != "claude_code" || call.Tool != "search" || call.Kind != "mcp" || call.ServerName != "docs" {
		t.Fatalf("call identity not carried over: %+v", call)
	}
	if call.Server.URL != "https://gitmcp.io/docs" || call.Server.Command != "npx" {
		t.Fatalf("server URL/command not carried over: %+v", call.Server)
	}
	// Unlike a device report, the stored hostname was derived from the URL, so it
	// is honored rather than dropped.
	if call.Server.Hostname != "gitmcp.io" {
		t.Fatalf("Server.Hostname = %q, want the recorded hostname", call.Server.Hostname)
	}
	if call.Server.Connector != "Google Calendar" {
		t.Fatalf("Server.Connector = %q, want the recorded connector", call.Server.Connector)
	}
	if !call.Unresolved || call.UnresolvedReason != "no MCP configuration matched the tool" {
		t.Fatalf("unresolved state not carried over: %+v", call)
	}
	if call.Server.Package != nil {
		t.Fatalf("Server.Package = %+v, want nil when the row records no package", call.Server.Package)
	}

	withPackage := base
	withPackage.ServerPackageSource = string(types.AllowlistServerPackageSourceNPM)
	withPackage.ServerPackageName = "@scope/server"
	withPackage.ServerPackageVersion = "1.2.3"
	call = normalizedCallFromDecisionLog(withPackage, true)
	if call.Server.Package == nil {
		t.Fatal("Server.Package = nil, want the recorded package identity")
	}
	if call.Server.Package.Source != types.AllowlistServerPackageSourceNPM ||
		call.Server.Package.Name != "@scope/server" ||
		call.Server.Package.Version != "1.2.3" {
		t.Fatalf("package identity not carried over: %+v", call.Server.Package)
	}

	sourceOnly := base
	sourceOnly.ServerPackageSource = string(types.AllowlistServerPackageSourcePyPI)
	if normalizedCallFromDecisionLog(sourceOnly, false).Server.Package == nil {
		t.Fatal("Server.Package = nil, want a package identity when only the source was recorded")
	}
}

func TestEnforcementDecideIgnoresDeviceReportedHostname(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{
		Servers: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
	})

	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search",
		Kind:       "mcp",
		ServerName: "docs",
		Server: types.EnforcementDecisionServer{
			URL:      "https://evil.example.com/mcp",
			Hostname: "gitmcp.io",
		},
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	if got := decodeDecisionResponse(t, rec).Decision; got != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny: a claimed hostname must not match an allowlist entry", got)
	}
	row := waitForEnforcementDecision(t, gatewayClient)
	if row.Server == nil {
		t.Fatal("logged row has no server identity")
	}
	if want := "evil.example.com"; row.Server.Hostname != want {
		t.Fatalf("logged hostname = %q, want %q derived from the URL", row.Server.Hostname, want)
	}
}

func TestEnforcementDecideBoundsDeviceSuppliedStrings(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{})

	huge := strings.Repeat("A", 200_000)
	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent:      huge,
		Tool:       huge,
		Kind:       huge,
		ServerName: huge,
		Server: types.EnforcementDecisionServer{
			URL:       "https://a.example.com/" + huge,
			Command:   huge,
			Connector: huge,
			Package:   &types.AllowlistServerPackage{Source: "npm", Name: huge, Version: huge},
		},
		Unresolved:       true,
		UnresolvedReason: huge,
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}

	row := waitForEnforcementDecision(t, gatewayClient)
	for _, f := range []struct {
		name  string
		value string
		max   int
	}{
		{"agent", row.Agent, maxIdentifierRunes},
		{"tool", row.Tool, maxIdentifierRunes},
		{"kind", row.Kind, maxIdentifierRunes},
		{"serverName", row.ServerName, maxIdentifierRunes},
		{"unresolvedReason", row.UnresolvedReason, maxUnresolvedReasonRunes},
		{"server.url", row.Server.URL, maxServerURLRunes},
		{"server.connector", row.Server.Connector, maxIdentifierRunes},
		{"server.package.name", row.Server.Package.Name, maxIdentifierRunes},
		{"server.package.version", row.Server.Package.Version, maxIdentifierRunes},
	} {
		if n := utf8.RuneCountInString(f.value); n > f.max {
			t.Errorf("%s stored %d runes, want at most %d", f.name, n, f.max)
		}
	}
}

func TestEnforcementDecideEvaluatesTheFullURLNotTheTruncatedOne(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	// The entry allows exactly one path, and its descendants.
	allowed := "https://a.example.com/" + strings.Repeat("p", maxServerURLRunes)
	configID := createEnforcementTestConfig(t, gatewayClient, types.EnforcementAllowlist{
		Servers: []types.AllowlistServer{{URL: allowed}},
	})

	// A sibling path that shares the allowed path's first maxServerURLRunes runes
	// but is a different path: truncation would make it match.
	sibling := allowed + "-elsewhere"
	rec := httptest.NewRecorder()
	body := types.EnforcementDecisionRequest{
		Agent: "claude_code", Tool: "search", Kind: "mcp", ServerName: "docs",
		Server: types.EnforcementDecisionServer{URL: sibling},
	}
	if err := newEnforcementTestHandler(t).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
		t.Fatalf("decide: %v", err)
	}
	if got := decodeDecisionResponse(t, rec).Decision; got != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny: the full URL is a sibling path, not a descendant", got)
	}
}
