package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	if err := NewEnforcementHandler(enforcementTestServerURL).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
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
	if err := NewEnforcementHandler(enforcementTestServerURL).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
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
	if err := NewEnforcementHandler(enforcementTestServerURL).Decide(newEnforcementDeviceContext(t, gatewayClient, body, config.ID, rec)); err != nil {
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
	if err := NewEnforcementHandler(enforcementTestServerURL).Decide(newEnforcementDeviceContext(t, gatewayClient, body, 9999, rec)); err != nil {
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
	if err := NewEnforcementHandler(enforcementTestServerURL).Decide(ctx); err != nil {
		t.Fatalf("decide: %v", err)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Decision != types.EnforcementDecisionDeny {
		t.Fatalf("decision = %q, want deny when no configuration is associated", resp.Decision)
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
	if err := NewEnforcementHandler(enforcementTestServerURL).Decide(ctx); err != nil {
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
	if err := NewEnforcementHandler(enforcementTestServerURL).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
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
	if err := NewEnforcementHandler(enforcementTestServerURL).Decide(newEnforcementDeviceContext(t, gatewayClient, body, configID, rec)); err != nil {
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

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
