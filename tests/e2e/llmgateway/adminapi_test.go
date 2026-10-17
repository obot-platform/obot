//go:build e2e

package llmgateway

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/tests/e2e/harness"
	"github.com/tidwall/gjson"
)

// rawGet requests a path without letting the client clean it, so dot segments and encoded
// separators reach the server as written.
func rawGet(t *testing.T, c *harness.Client, rawPath string) *harness.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.URL.Opaque = "//" + req.URL.Host + rawPath
	resp, err := c.HTTP.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", rawPath, err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 64*1024)
	n, _ := resp.Body.Read(buf)
	sb.Write(buf[:n])
	return &harness.Response{Status: resp.StatusCode, Header: resp.Header, Body: []byte(sb.String())}
}

func TestAdminAPISafety(t *testing.T) {
	admin := env.admin

	t.Run("path traversal is refused", func(t *testing.T) {
		for _, p := range []string{
			"/api/llm-gateway/admin/providers/%2e%2e/session/is-auth-enabled",
			"/api/llm-gateway/admin/providers/%2E%2E/mcp/clients",
			"/api/llm-gateway/admin/providers/..%2fsession",
			"/api/llm-gateway/admin/providers%2f..%2fsession",
			"/api/llm-gateway/admin/providers/..%5csession",
			"/api/llm-gateway/admin/providers/../session/is-auth-enabled",
			"/api/llm-gateway/admin/providers/./../mcp/clients",
			"/api/llm-gateway/admin/session/is-auth-enabled",
			"/api/llm-gateway/admin/mcp/clients",
			"/api/llm-gateway/admin/governance/virtual-keys",
		} {
			resp := rawGet(t, admin, p)
			if resp.Status == http.StatusOK {
				t.Errorf("%s: want refusal, got 200: %s", p, truncate(string(resp.Body)))
			}
			if resp.Status >= 300 && resp.Status < 400 {
				// The mux may redirect a cleaned path; the redirect must not land on an
				// engine API either.
				loc := resp.Header.Get("Location")
				if follow := rawGet(t, admin, loc); follow.Status == http.StatusOK {
					t.Errorf("%s redirected to %s, which answered 200", p, loc)
				}
			}
		}
		// The allowlist itself still works.
		admin.Get(t, "/api/llm-gateway/admin/providers", nil)
	})

	t.Run("env and vault secret references are rejected", func(t *testing.T) {
		provider := "e2eenv" + harness.RandomSuffix()
		for i, body := range []map[string]any{
			{"name": "k", "value": "env.HOME", "models": []string{"*"}, "weight": 1},
			{"name": "k", "value": map[string]any{"type": "env", "env_var": "HOME", "value": ""}, "models": []string{"*"}, "weight": 1},
			{"name": "k", "value": map[string]any{"value": "x", "from_env": true}, "models": []string{"*"}, "weight": 1},
			{"name": "k", "value": "vault.secret/data/obot", "models": []string{"*"}, "weight": 1},
			// Bifrost matches field names in any case.
			{"name": "k", "value": map[string]any{"value": "", "ref": "HOME", "TYPE": "env"}, "models": []string{"*"}, "weight": 1},
			{"name": "k", "value": map[string]any{"value": "", "env_var": "HOME", "FROM_ENV": true}, "models": []string{"*"}, "weight": 1},
		} {
			resp := admin.Do(t, http.MethodPost, "/api/llm-gateway/admin/providers/openai/keys", body)
			if resp.Status != http.StatusBadRequest {
				t.Errorf("reference %d: want 400, got %s", i, resp)
			}
		}
		resp := admin.Do(t, http.MethodPost, "/api/llm-gateway/admin/providers", map[string]any{
			"provider": provider,
			"network_config": map[string]any{
				"base_url":      env.mock.URL,
				"extra_headers": map[string]string{"X-Leak": "env.OBOT_SERVER_DSN"},
			},
			"custom_provider_config": map[string]any{"base_provider_type": "openai"},
		})
		if resp.Status != http.StatusBadRequest {
			t.Errorf("env reference in a provider header: want 400, got %s", resp)
			admin.Do(t, http.MethodDelete, "/api/llm-gateway/admin/providers/"+provider, nil)
		}
	})

	t.Run("the engine's own login can't be turned on", func(t *testing.T) {
		// Obot reaches the engine with the setup token that creates the engine's first admin
		// account. Turning that login on would lock Obot out of managing its own gateway.
		var cfg struct {
			ClientConfig map[string]any `json:"client_config"`
		}
		admin.Get(t, "/api/llm-gateway/admin/config", &cfg)
		// The engine refuses a config update unless its log retention is at least a day.
		cfg.ClientConfig["log_retention_days"] = 1
		login := map[string]any{"is_enabled": true, "admin_username": "e2e", "admin_password": "Str0ng!Passw0rd-e2e"}
		for _, key := range []string{"auth_config", "AUTH_CONFIG"} {
			resp := admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/config", map[string]any{
				"client_config": cfg.ClientConfig,
				key:             login,
			})
			if resp.Status != http.StatusBadRequest {
				t.Errorf("%s: want 400, got %s", key, resp)
			}
		}
		// Obot still manages the gateway.
		admin.Get(t, "/api/llm-gateway/admin/providers", nil)
	})

	t.Run("the engine's plugins and cache are not exposed", func(t *testing.T) {
		// Obot's guardrail plugins must stay enabled, and responses aren't cached, so neither
		// API is reachable.
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "/api/llm-gateway/admin/plugins", nil},
			{http.MethodDelete, "/api/llm-gateway/admin/plugins/obot-guardrails-input", nil},
			{http.MethodPut, "/api/llm-gateway/admin/plugins/obot-guardrails-output", map[string]any{"name": "obot-guardrails-output", "enabled": false}},
			{http.MethodPost, "/api/llm-gateway/admin/plugins", map[string]any{"name": "semantic_cache", "enabled": true}},
			{http.MethodDelete, "/api/llm-gateway/admin/cache/clear", nil},
		} {
			if resp := admin.Do(t, tc.method, tc.path, tc.body); resp.Status != http.StatusNotFound {
				t.Errorf("%s %s: want 404, got %s", tc.method, tc.path, resp)
			}
		}
	})

	t.Run("guardrail credentials are never returned", func(t *testing.T) {
		apiKey := "e2e-presidio-key-" + harness.RandomSuffix()
		var created struct {
			ID                string   `json:"id"`
			ConfiguredSecrets []string `json:"configuredSecrets"`
		}
		resp := admin.Post(t, "/api/llm-gateway/guardrails", map[string]any{
			"name":   "e2e presidio creds " + harness.RandomSuffix(),
			"type":   "presidio",
			"config": map[string]any{"analyzerURL": "http://presidio.invalid:3000", "apiKey": apiKey, "language": "en"},
		}, &created)
		t.Cleanup(func() { admin.Do(t, http.MethodDelete, "/api/llm-gateway/guardrails/"+created.ID, nil) })
		if strings.Contains(string(resp.Body), apiKey) {
			t.Fatalf("create returned the credential: %s", resp)
		}
		if len(created.ConfiguredSecrets) != 1 || created.ConfiguredSecrets[0] != "apiKey" {
			t.Fatalf("want configuredSecrets [apiKey], got %v", created.ConfiguredSecrets)
		}
		for _, r := range []*harness.Replica{env.A, env.B} {
			for _, p := range []string{"/api/llm-gateway/guardrails/" + created.ID, "/api/llm-gateway/guardrails"} {
				if got := admin.On(r.BaseURL).Do(t, http.MethodGet, p, nil); got.Status != http.StatusOK || strings.Contains(string(got.Body), apiKey) {
					t.Fatalf("GET %s on %s: %s", p, r.Config.Name, got)
				}
			}
		}

		t.Run("changing the endpoint requires re-entering credentials", func(t *testing.T) {
			path := "/api/llm-gateway/guardrails/" + created.ID
			moved := map[string]any{"name": "moved", "type": "presidio", "config": map[string]any{"analyzerURL": "http://attacker.invalid:3000", "language": "en"}}
			if resp := admin.Do(t, http.MethodPut, path, moved); resp.Status != http.StatusBadRequest {
				t.Fatalf("moving the endpoint without the credential: want 400, got %s", resp)
			}
			// Other settings can change without the credential, and it is kept.
			lang := map[string]any{"name": "lang", "type": "presidio", "config": map[string]any{"analyzerURL": "http://presidio.invalid:3000", "language": "de"}}
			var kept struct {
				ConfiguredSecrets []string `json:"configuredSecrets"`
			}
			admin.Put(t, path, lang, &kept)
			if len(kept.ConfiguredSecrets) != 1 {
				t.Fatalf("the credential was dropped: %v", kept.ConfiguredSecrets)
			}
			// Re-entering it allows the move.
			moved["config"].(map[string]any)["apiKey"] = "new-" + apiKey
			if resp := admin.Do(t, http.MethodPut, path, moved); resp.Status != http.StatusOK || strings.Contains(string(resp.Body), apiKey) {
				t.Fatalf("moving the endpoint with the credential: %s", resp)
			}
		})
	})

	t.Run("non-admins are forbidden and auditors can read", func(t *testing.T) {
		basic := env.newUser(t, "basic-admin-api")
		auditor := newAuditor(t)

		reads := []string{
			"/api/llm-gateway/status",
			"/api/llm-gateway/budgets",
			"/api/llm-gateway/admin/providers",
			"/api/token-usage-stats",
			"/api/llm-gateway/guardrail-policies",
			"/api/llm-gateway/guardrails",
			"/api/llm-gateway/metrics",
		}
		writes := []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, "/api/llm-gateway/budgets", map[string]any{"name": "x", "limits": []any{map[string]any{"metric": "requests", "amount": 1, "window": "1h"}}, "profileID": "x"}},
			{http.MethodPost, "/api/llm-gateway/admin/providers", map[string]any{"provider": "mistral"}},
			{http.MethodPost, "/api/llm-gateway/guardrail-policies", map[string]any{"name": "x"}},
			{http.MethodPost, "/api/llm-gateway/sync-models", map[string]any{}},
			{http.MethodDelete, "/api/llm-gateway/budgets/" + defaultBudgetID, nil},
		}
		for _, who := range []struct {
			name string
			c    *harness.Client
		}{
			{"basic user session", basic.Session},
			{"basic user API key", harness.NewAPIKeyClient(env.A.BaseURL, basic.Key)},
		} {
			for _, p := range reads {
				if resp := who.c.Do(t, http.MethodGet, p, nil); resp.Status != http.StatusForbidden && resp.Status != http.StatusUnauthorized {
					t.Errorf("%s GET %s: want 403, got %s", who.name, p, resp)
				}
			}
			for _, w := range writes {
				if resp := who.c.Do(t, w.method, w.path, w.body); resp.Status != http.StatusForbidden && resp.Status != http.StatusUnauthorized {
					t.Errorf("%s %s %s: want 403, got %s", who.name, w.method, w.path, resp)
				}
			}
		}
		for _, p := range reads {
			if resp := auditor.Session.Do(t, http.MethodGet, p, nil); resp.Status != http.StatusOK {
				t.Errorf("auditor GET %s: want 200, got %s", p, resp)
			}
		}
		for _, w := range writes {
			if resp := auditor.Session.Do(t, w.method, w.path, w.body); resp.Status != http.StatusForbidden {
				t.Errorf("auditor %s %s: want 403, got %s", w.method, w.path, resp)
			}
		}
		// The default budget survived the attempts.
		admin.Get(t, "/api/llm-gateway/budgets/"+defaultBudgetID, nil)
	})
}

// newAuditor creates a basic user with the auditor role.
func newAuditor(t *testing.T) *testUser {
	t.Helper()
	auditor := env.newUser(t, "auditor")
	env.admin.Expect(t, http.StatusOK, http.MethodPatch, "/api/users/"+auditor.ID, map[string]any{"role": 4 | 32}, nil)
	harness.Eventually(t, 30*time.Second, "auditor role", func() error {
		resp := auditor.Session.Do(t, http.MethodGet, "/api/me", nil)
		if gjson.GetBytes(resp.Body, "role").Int()&32 == 0 && gjson.GetBytes(resp.Body, "effectiveRole").Int()&32 == 0 {
			return fmt.Errorf("role %s", gjson.GetBytes(resp.Body, "role"))
		}
		return nil
	})
	return auditor
}
