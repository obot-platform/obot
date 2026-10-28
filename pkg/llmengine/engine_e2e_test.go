package llmengine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/transports/bifrost-http/lib"
)

func mockOpenAI(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("upstream got %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-4o-mini","object":"model","owned_by":"openai"},{"id":"gpt-4.1","object":"model","owned_by":"openai"}]}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			_ = json.Unmarshal(body, &req)
			if req["stream"] == true {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`+"\n\n")
				_, _ = io.WriteString(w, `data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":"stop"}]}`+"\n\n")
				_, _ = io.WriteString(w, `data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o-mini","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`+"\n\n")
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
				return
			}
			_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"Hello from mock"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
		case strings.HasSuffix(r.URL.Path, "/responses"):
			_, _ = io.WriteString(w, `{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"gpt-4o-mini","output":[{"type":"message","id":"m1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello from responses","annotations":[]}]}],"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func do(t *testing.T, e *Engine, method, path, body string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, string(b)
}

func TestEngineE2E(t *testing.T) {
	up := mockOpenAI(t)
	defer up.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	e := New()
	if err := e.Start(ctx, Options{DataDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	defer e.Shutdown()

	if e.HasUpstreamProviders(ctx) {
		t.Fatal("expected no upstream providers before configuring one")
	}

	mustOK := func(method, path, body string) string {
		t.Helper()
		code, resp := do(t, e, method, path, body)
		if code != http.StatusOK {
			t.Fatalf("%s %s: got %d: %s", method, path, code, resp)
		}
		return resp
	}

	mustOK("POST", "/api/providers", `{"provider":"openai","network_config":{"base_url":"`+up.URL+`","allow_private_network":true,"default_request_timeout_in_seconds":30}}`)
	mustOK("POST", "/api/providers/openai/keys", `{"name":"k1","value":"sk-test-123","models":["*"],"weight":1}`)

	if !e.HasUpstreamProviders(ctx) {
		t.Fatal("expected upstream providers after adding a key")
	}

	models, err := e.ListModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range models.Models {
		ids = append(ids, m.ID)
		if m.ID == "openai/gpt-4o-mini" {
			if m.Metadata["dialect"] != "OpenAIResponses" || m.Metadata["usage"] != "llm" {
				t.Fatalf("unexpected metadata for %s: %v", m.ID, m.Metadata)
			}
		}
	}
	if !strings.Contains(strings.Join(ids, ","), "openai/gpt-4o-mini") {
		t.Fatalf("expected openai/gpt-4o-mini in %v", ids)
	}

	for _, tc := range []struct{ path, body, want string }{
		{"/v1/chat/completions", `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`, "Hello from mock"},
		{"/v1/chat/completions", `{"model":"openai/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hi"}]}`, "lo"},
		// Anthropic Messages to an OpenAI model is translated by Bifrost.
		{"/anthropic/v1/messages", `{"model":"openai/gpt-4o-mini","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`, "Hello from responses"},
		{"/v1/responses", `{"model":"openai/gpt-4o-mini","input":"hi"}`, "Hello from responses"},
	} {
		if resp := mustOK("POST", tc.path, tc.body); !strings.Contains(resp, tc.want) {
			t.Fatalf("POST %s: expected %q in %s", tc.path, tc.want, resp)
		}
	}
}

func TestPlainTextSecrets(t *testing.T) {
	key := map[string]any{
		"name":               "imported",
		"value":              "env.NOT_A_REFERENCE",
		"models":             []string{"*"},
		"bedrock_key_config": map[string]any{"region": "us-east-1", "access_key": "AKIA"},
	}
	plainTextSecrets(key)

	value, ok := key["value"].(map[string]any)
	if !ok || value["value"] != "env.NOT_A_REFERENCE" || value["type"] != "plain_text" {
		t.Fatalf("value not wrapped: %#v", key["value"])
	}
	cfg := key["bedrock_key_config"].(map[string]any)
	if region, ok := cfg["region"].(map[string]any); !ok || region["type"] != "plain_text" {
		t.Fatalf("key config not wrapped: %#v", cfg)
	}
	if key["name"] != "imported" {
		t.Fatalf("name should be left alone: %#v", key["name"])
	}
}

func TestMaxRequestBodyBytesFollowsTheSettings(t *testing.T) {
	e, ctx := startTestEngine(t)
	if got := e.MaxRequestBodyBytes(); got != 100<<20 {
		t.Fatalf("MaxRequestBodyBytes() = %d, want the 100 MB default", got)
	}
	var config struct {
		ClientConfig map[string]any `json:"client_config"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/config", nil, &config); err != nil {
		t.Fatal(err)
	}
	config.ClientConfig["max_request_body_size_mb"] = 1
	config.ClientConfig["log_retention_days"] = 1
	if err := e.Do(ctx, http.MethodPut, "/api/config", map[string]any{"client_config": config.ClientConfig}, nil); err != nil {
		t.Fatal(err)
	}
	// The engine applies its own limit only when it next starts; Obot's applies at once.
	if got := e.MaxRequestBodyBytes(); got != 1<<20 {
		t.Fatalf("MaxRequestBodyBytes() = %d after the change, want 1 MB", got)
	}
}

// A raised request size limit applies at once: until the engine reloads, its HTTP server
// still runs with the old one, and must not refuse a body Obot's check let through.
func TestRaisedRequestSizeLimitAppliesAtOnce(t *testing.T) {
	up := mockOpenAI(t)
	defer up.Close()
	e, ctx := startTestEngine(t)
	if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{
		"provider":       "openai",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true, "default_request_timeout_in_seconds": 30},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "k1", "value": "sk-test", "models": []string{"*"}, "weight": 1}, nil); err != nil {
		t.Fatal(err)
	}
	setLimit := func(mb int) {
		t.Helper()
		var config struct {
			ClientConfig map[string]any `json:"client_config"`
		}
		if err := e.Do(ctx, http.MethodGet, "/api/config", nil, &config); err != nil {
			t.Fatal(err)
		}
		config.ClientConfig["max_request_body_size_mb"] = mb
		config.ClientConfig["log_retention_days"] = 1
		if err := e.Do(ctx, http.MethodPut, "/api/config", map[string]any{"client_config": config.ClientConfig}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// The engine starts with a 1 MB limit, then it is raised to 2 MB.
	setLimit(1)
	if _, err := e.Reload(1); err != nil {
		t.Fatal(err)
	}
	setLimit(2)
	if got := e.MaxRequestBodyBytes(); got != 2<<20 {
		t.Fatalf("MaxRequestBodyBytes() = %d, want 2 MB", got)
	}

	body := `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"` + strings.Repeat("a", 3<<19) + `"}]}`
	if code, resp := do(t, e, http.MethodPost, "/v1/chat/completions", body); code != http.StatusOK || !strings.Contains(resp, "Hello from mock") {
		t.Fatalf("a 1.5 MB request under the raised limit got %d: %.200s", code, resp)
	}
}

// Warp, the engine's dashboard agent, is on by default; Obot turns it off, and it stays off
// even if the engine's API is asked to turn it on.
func TestWarpIsOff(t *testing.T) {
	e, ctx := startTestEngine(t)
	warpEnabled := func() bool {
		return e.inst.Load().server.Config.FeatureFlags.IsEnabled(lib.FeatureFlagWarp)
	}
	if warpEnabled() {
		t.Fatal("warp is on")
	}
	_ = e.Do(ctx, http.MethodPut, "/api/feature-flags/"+lib.FeatureFlagWarp, map[string]any{"enabled": true}, nil)
	if warpEnabled() {
		t.Fatal("the engine's API turned warp on")
	}
}
