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
	c := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, true)
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

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
