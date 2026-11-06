//go:build e2e

package llmgateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
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

	t.Run("Vertex credentials must be a service account key", func(t *testing.T) {
		vertexKey := func(name string, creds any) map[string]any {
			return map[string]any{
				"name":   name,
				"models": []string{"*"},
				"weight": 1,
				"vertex_key_config": map[string]any{
					"project_id":       "e2e-project",
					"region":           "us-central1",
					"auth_credentials": creds,
				},
			}
		}
		for _, tc := range []struct {
			name string
			path string
			body map[string]any
		}{
			{
				name: "external account",
				path: "/api/llm-gateway/admin/providers/vertex/keys",
				body: vertexKey("ext", `{"type":"external_account","credential_source":{"file":"/etc/hostname"}}`),
			},
			{
				name: "authorized user as a secret object",
				path: "/api/llm-gateway/admin/providers/vertex/keys",
				body: vertexKey("user", map[string]any{"value": `{"type":"authorized_user"}`, "type": "plain_text"}),
			},
			{
				name: "a second type in another case",
				path: "/api/llm-gateway/admin/providers/vertex/keys",
				body: vertexKey("dup", `{"type":"service_account","TYPE":"external_account"}`),
			},
			{
				name: "field names in another case",
				path: "/api/llm-gateway/admin/providers/vertex/keys",
				body: map[string]any{"name": "case", "models": []string{"*"}, "VERTEX_KEY_CONFIG": map[string]any{"project_id": "p", "region": "us-central1", "Auth_Credentials": `{"type":"impersonated_service_account"}`}},
			},
			{
				name: "in a new provider's keys",
				path: "/api/llm-gateway/admin/providers",
				body: map[string]any{
					"provider": "vertex",
					"keys":     []any{vertexKey("ext", `{"type":"external_account"}`)},
				},
			},
		} {
			resp := admin.Do(t, http.MethodPost, tc.path, tc.body)
			if resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), "service account key") {
				t.Errorf("%s: want 400, got %s", tc.name, resp)
			}
		}

		var existing map[string]any
		if resp := admin.Do(t, http.MethodGet, "/api/llm-gateway/admin/providers/vertex", nil); resp.Status == http.StatusNotFound {
			admin.Post(t, "/api/llm-gateway/admin/providers", map[string]any{"provider": "vertex"}, &existing)
			t.Cleanup(func() { admin.Do(t, http.MethodDelete, "/api/llm-gateway/admin/providers/vertex", nil) })
		}

		// A service account key, and a key without credentials (the server's own identity),
		// are accepted, and so is a key sent back as the gateway lists it, as the key dialog
		// does when it saves other changes.
		saKey := `{"type":"service_account","project_id":"e2e-project","private_key_id":"abc","client_email":"e2e@e2e-project.iam.gserviceaccount.com"}`
		for _, k := range []map[string]any{
			vertexKey("e2e-sa-"+harness.RandomSuffix(), saKey),
			vertexKey("e2e-adc-"+harness.RandomSuffix(), ""),
		} {
			name := k["name"].(string)
			if resp := admin.Do(t, http.MethodPost, "/api/llm-gateway/admin/providers/vertex/keys", k); resp.Status != http.StatusOK {
				t.Fatalf("%s: want 200, got %s", name, resp)
			}
			id := findKeyID(t, admin, "vertex", name)
			if id == "" {
				t.Fatalf("%s was not created", name)
			}
			t.Cleanup(func() { admin.Do(t, http.MethodDelete, "/api/llm-gateway/admin/providers/vertex/keys/"+id, nil) })
			var listed map[string]any
			admin.Get(t, "/api/llm-gateway/admin/providers/vertex/keys/"+id, &listed)
			delete(listed, "id")
			if resp := admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/providers/vertex/keys/"+id, listed); resp.Status != http.StatusOK {
				t.Errorf("%s sent back as listed: want 200, got %s", name, resp)
			}
		}
	})

	t.Run("proxy settings can't weaken the TLS that protects the server's identity", func(t *testing.T) {
		ca := testCACertPEM(t)
		proxyWithCA := map[string]any{"type": "http", "url": "http://127.0.0.1:3128", "ca_cert_pem": ca}
		for _, tc := range []struct {
			name   string
			method string
			path   string
			body   map[string]any
		}{
			{
				name:   "a new Bedrock provider",
				method: http.MethodPost,
				path:   "/api/llm-gateway/admin/providers",
				body:   map[string]any{"provider": "bedrock", "proxy_config": proxyWithCA},
			},
			{
				name:   "a new Vertex provider, in another case and as a secret object",
				method: http.MethodPost,
				path:   "/api/llm-gateway/admin/providers",
				body: map[string]any{
					"provider":     "vertex",
					"PROXY_CONFIG": map[string]any{"type": "http", "url": "http://127.0.0.1:3128", "Ca_Cert_Pem": map[string]any{"value": ca, "type": "plain_text"}},
				},
			},
			{
				name:   "a new Azure provider",
				method: http.MethodPost,
				path:   "/api/llm-gateway/admin/providers",
				body:   map[string]any{"provider": "azure", "proxy_config": proxyWithCA},
			},
		} {
			resp := admin.Do(t, tc.method, tc.path, tc.body)
			if resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), "custom CA") {
				t.Errorf("%s: want 400, got %s", tc.name, resp)
				if resp.Status == http.StatusOK {
					admin.Do(t, http.MethodDelete, "/api/llm-gateway/admin/providers/"+tc.body["provider"].(string), nil)
				}
			}
		}

		// A provider with its own keys may trust a TLS-inspecting proxy's CA.
		name := "e2eproxyca" + harness.RandomSuffix()
		resp := admin.Do(t, http.MethodPost, "/api/llm-gateway/admin/providers", map[string]any{
			"provider":               name,
			"custom_provider_config": map[string]any{"base_provider_type": "openai"},
			"network_config":         map[string]any{"base_url": env.mock.URL, "allow_private_network": true},
			"proxy_config":           proxyWithCA,
		})
		if resp.Status != http.StatusOK {
			t.Fatalf("a proxy CA on a provider with its own keys: want 200, got %s", resp)
		}
		admin.Do(t, http.MethodDelete, "/api/llm-gateway/admin/providers/"+name, nil)
	})

	t.Run("the gateway proxy can't turn off certificate checks", func(t *testing.T) {
		var original map[string]any
		admin.Get(t, "/api/llm-gateway/admin/proxy-config", &original)
		t.Cleanup(func() { admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/proxy-config", original) })
		for _, key := range []string{"skip_tls_verify", "Skip_TLS_Verify"} {
			for _, enabled := range []bool{true, false} {
				resp := admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/proxy-config", map[string]any{
					"enabled": enabled,
					"type":    "http",
					"url":     "https://127.0.0.1:3128",
					key:       true,
				})
				if resp.Status != http.StatusBadRequest {
					t.Errorf("%s with the proxy enabled=%v: want 400, got %s", key, enabled, resp)
				}
			}
		}
		resp := admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/proxy-config", map[string]any{
			"enabled":         false,
			"type":            "http",
			"url":             "https://127.0.0.1:3128",
			"skip_tls_verify": false,
		})
		if resp.Status != http.StatusOK {
			t.Errorf("a proxy with certificate checks on: want 200, got %s", resp)
		}
	})

	t.Run("Anthropic's proxy can't trust another CA while subscriptions are allowed", func(t *testing.T) {
		var original map[string]any
		admin.Get(t, "/api/llm-gateway/settings", &original)
		t.Cleanup(func() { admin.Do(t, http.MethodPut, "/api/llm-gateway/settings", original) })
		admin.Put(t, "/api/llm-gateway/settings", map[string]any{"subscriptionPassthrough": true}, nil)

		ca := testCACertPEM(t)
		// What the provider page sends, plus a proxy: the provider's own network settings, so
		// nothing but the proxy changes.
		var current map[string]any
		admin.Get(t, "/api/llm-gateway/admin/providers/anthropic", &current)
		update := func(proxyKey string, proxy map[string]any) map[string]any {
			return map[string]any{
				"network_config":              current["network_config"],
				"concurrency_and_buffer_size": current["concurrency_and_buffer_size"],
				proxyKey:                      proxy,
			}
		}
		for _, tc := range []struct {
			name string
			body map[string]any
		}{
			{
				name: "a proxy CA",
				body: update("proxy_config", map[string]any{"type": "http", "url": "http://127.0.0.1:3128", "ca_cert_pem": ca}),
			},
			{
				name: "in another case and as a secret object",
				body: update("Proxy_Config", map[string]any{"type": "http", "url": "http://127.0.0.1:3128", "CA_CERT_PEM": map[string]any{"value": ca}}),
			},
		} {
			resp := admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/providers/anthropic", tc.body)
			if resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), "Claude subscriptions") {
				t.Errorf("%s: want 400, got %s", tc.name, resp)
				if resp.Status == http.StatusOK {
					admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/providers/anthropic", update("proxy_config", map[string]any{"type": "none"}))
				}
			}
		}
		// The provider still answers with its own settings.
		var provider map[string]any
		admin.Get(t, "/api/llm-gateway/admin/providers/anthropic", &provider)
		if ca := gjson.Get(mustMarshal(t, provider), "proxy_config.ca_cert_pem.value").String(); ca != "" {
			t.Fatalf("a refused change set the proxy CA: %q", ca)
		}
	})

	t.Run("gateway settings can be saved", func(t *testing.T) {
		var before map[string]any
		admin.Get(t, "/api/llm-gateway/admin/config", &before)
		client := before["client_config"].(map[string]any)
		original := client["drop_excess_requests"]
		client["drop_excess_requests"] = original != true
		// Send the whole config back, framework settings included, as a client that read it
		// would. Obot keeps the framework settings it manages itself.
		resp := admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/config", map[string]any{
			"client_config":    client,
			"framework_config": before["framework_config"],
		})
		if resp.Status != http.StatusOK {
			t.Fatalf("save settings: want 200, got %s", resp)
		}
		var after map[string]any
		admin.Get(t, "/api/llm-gateway/admin/config", &after)
		if got := after["client_config"].(map[string]any)["drop_excess_requests"]; got != client["drop_excess_requests"] {
			t.Errorf("drop_excess_requests = %v after saving %v", got, client["drop_excess_requests"])
		}
		if got, want := after["framework_config"].(map[string]any)["pricing_url"], before["framework_config"].(map[string]any)["pricing_url"]; got != want {
			t.Errorf("pricing_url changed from %v to %v", want, got)
		}
		client["drop_excess_requests"] = original
		admin.Do(t, http.MethodPut, "/api/llm-gateway/admin/config", map[string]any{"client_config": client})
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

	t.Run("the engine's MCP library settings are hidden", func(t *testing.T) {
		resp := admin.Do(t, http.MethodGet, "/api/llm-gateway/admin/config", nil)
		if resp.Status != http.StatusOK || strings.Contains(string(resp.Body), "mcp_library") {
			t.Fatalf("GET config: %s", resp)
		}
	})

	t.Run("guardrails can't use loopback or metadata addresses", func(t *testing.T) {
		for _, config := range []map[string]any{
			{"apiKey": "fake-x", "baseURL": "http://169.254.169.254"},
			{"apiKey": "fake-x", "baseURL": "http://[fd00:ec2::254]"},
			{"apiKey": "fake-x", "baseURL": "http://127.0.0.1:8080"},
			{"apiKey": "fake-x", "baseURL": "http://localhost:8080"},
		} {
			resp := admin.Do(t, http.MethodPost, "/api/llm-gateway/guardrails", map[string]any{"name": "e2e metadata " + harness.RandomSuffix(), "type": "lakera", "config": config})
			if resp.Status != http.StatusBadRequest {
				t.Errorf("guardrail on %v: want 400, got %s", config["baseURL"], resp)
				if id := gjson.GetBytes(resp.Body, "id").String(); id != "" {
					admin.Do(t, http.MethodDelete, "/api/llm-gateway/guardrails/"+id, nil)
				}
			}
		}
		// A guardrail service on a private network, such as a self-hosted Presidio, is fine.
		var created struct {
			ID string `json:"id"`
		}
		admin.Post(t, "/api/llm-gateway/guardrails", map[string]any{
			"name":   "e2e private presidio " + harness.RandomSuffix(),
			"type":   "presidio",
			"config": map[string]any{"analyzerURL": "http://10.255.255.1:3000", "language": "en"},
		}, &created)
		admin.Do(t, http.MethodDelete, "/api/llm-gateway/guardrails/"+created.ID, nil)
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

// TestProviderAddresses checks where providers may connect, and that their keys can't be
// redirected elsewhere.
func TestProviderAddresses(t *testing.T) {
	admin := env.admin
	newProvider := func(t *testing.T, baseURL string, allowPrivate bool) (string, *harness.Response) {
		t.Helper()
		name := "e2eaddr" + harness.RandomSuffix()
		resp := admin.Do(t, http.MethodPost, "/api/llm-gateway/admin/providers", map[string]any{
			"provider":               name,
			"custom_provider_config": map[string]any{"base_provider_type": "openai"},
			"network_config":         map[string]any{"base_url": baseURL, "allow_private_network": allowPrivate, "default_request_timeout_in_seconds": 30},
		})
		if resp.Status == http.StatusOK {
			t.Cleanup(func() { admin.Do(t, http.MethodDelete, "/api/llm-gateway/admin/providers/"+name, nil) })
		}
		return name, resp
	}

	t.Run("loopback addresses need private network addresses allowed", func(t *testing.T) {
		for _, u := range []string{"http://localhost:18999", "http://127.0.0.1:18999", "http://[::1]:18999"} {
			if _, resp := newProvider(t, u, false); resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), "Allow private network addresses") {
				t.Errorf("%s without private addresses allowed: want 400, got %s", u, resp)
			}
			if _, resp := newProvider(t, u, true); resp.Status != http.StatusOK {
				t.Errorf("%s with private addresses allowed: want 200, got %s", u, resp)
			}
		}
	})

	t.Run("metadata addresses are never allowed", func(t *testing.T) {
		for _, u := range []string{"http://169.254.169.254", "http://[fd00:ec2::254]", "http://[::ffff:169.254.169.254]"} {
			if _, resp := newProvider(t, u, true); resp.Status != http.StatusBadRequest {
				t.Errorf("%s: want 400, got %s", u, resp)
			}
		}
	})

	t.Run("a custom provider's settings can be saved", func(t *testing.T) {
		name, resp := newProvider(t, env.mock.URL, true)
		if resp.Status != http.StatusOK {
			t.Fatalf("create: %s", resp)
		}
		// What the provider page sends: the settings it edits, without custom_provider_config.
		var saved map[string]any
		admin.Put(t, "/api/llm-gateway/admin/providers/"+name, map[string]any{
			"network_config":              map[string]any{"base_url": env.mock.URL, "allow_private_network": true, "default_request_timeout_in_seconds": 45},
			"concurrency_and_buffer_size": map[string]any{"concurrency": 1000, "buffer_size": 5000},
		}, &saved)
		admin.Get(t, "/api/llm-gateway/admin/providers/"+name, &saved)
		if got := gjson.Get(mustMarshal(t, saved), "network_config.default_request_timeout_in_seconds").Int(); got != 45 {
			t.Errorf("timeout = %d, want 45", got)
		}
		if got := gjson.Get(mustMarshal(t, saved), "custom_provider_config.base_provider_type").String(); got != "openai" {
			t.Errorf("custom provider config was lost: %v", saved["custom_provider_config"])
		}
	})

	t.Run("changing the base URL removes the keys", func(t *testing.T) {
		name, resp := newProvider(t, env.mock.URL, true)
		if resp.Status != http.StatusOK {
			t.Fatalf("create: %s", resp)
		}
		keyName := "k-" + name
		if addKey(t, admin, name, keyName, "e2e-upstream-"+name) == "" {
			t.Fatal("key was not created")
		}
		moved := map[string]any{
			"network_config":              map[string]any{"base_url": env.mock.URL + "/moved", "allow_private_network": true},
			"concurrency_and_buffer_size": map[string]any{"concurrency": 1000, "buffer_size": 5000},
		}
		path := "/api/llm-gateway/admin/providers/" + name
		if resp := admin.Do(t, http.MethodPut, path, moved); resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), "removes its keys") {
			t.Fatalf("moving the provider with its keys: want 400, got %s", resp)
		}
		if findKeyID(t, admin, name, keyName) == "" {
			t.Fatal("a refused change removed the key")
		}
		admin.Put(t, path+"?removeKeys=true", moved, nil)
		if findKeyID(t, admin, name, keyName) != "" {
			t.Fatal("the key was kept after moving the provider")
		}
		var after map[string]any
		admin.Get(t, path, &after)
		if got := gjson.Get(mustMarshal(t, after), "network_config.base_url").String(); !strings.HasSuffix(got, "/moved") {
			t.Fatalf("base URL = %q after the move", got)
		}
	})
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// testCACertPEM returns a self-signed CA certificate, as an administrator would add to trust a
// TLS-inspecting proxy.
func testCACertPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "e2e proxy CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
