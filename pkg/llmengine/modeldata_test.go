package llmengine

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/modeldata"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
)

// TestEngineBootsWithoutNetwork starts the gateway with no way to reach the internet. It must
// boot from the model data in its data dir, and price requests with it, without fetching
// anything from the engine's vendor.
func TestEngineBootsWithoutNetwork(t *testing.T) {
	for _, v := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(v, "http://127.0.0.1:9")
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	dataDir := t.TempDir()
	modeldatatest.Write(t, dataDir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	e := New()
	if err := e.Start(ctx, Options{DataDir: dataDir}); err != nil {
		t.Fatal(err)
	}
	defer e.Shutdown()

	catalog := e.inst.Load().server.Config.ModelCatalog
	usage := &schemas.BifrostLLMUsage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000, TotalTokens: 2_000_000}
	if cost := catalog.CalculateCostForUsage(usage, schemas.OpenAI, "gpt-4o", schemas.ChatCompletionRequest, nil); cost <= 0 {
		t.Errorf("gpt-4o costs %v for a million tokens each way, want a price from the model data", cost)
	}
}

// The engine re-reads the model data files on its own schedule (hourly, through the same
// sync ForceReloadPricing runs), so a refresh that rewrites them changes its prices, and an
// engine that started without model data gets prices once the files are written.
func TestEnginePicksUpChangedModelData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dataDir := t.TempDir()
	pricingPath, paramsPath := modeldata.Paths(dataDir)
	for _, path := range []string{pricingPath, paramsPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e := New()
	if err := e.Start(ctx, Options{DataDir: dataDir}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Shutdown)
	catalog := e.inst.Load().server.Config.ModelCatalog
	gpt4o := func() float64 {
		t.Helper()
		usage := &schemas.BifrostLLMUsage{PromptTokens: 1_000_000, TotalTokens: 1_000_000}
		return catalog.CalculateCostForUsage(usage, schemas.OpenAI, "gpt-4o", schemas.ChatCompletionRequest, nil)
	}
	if cost := gpt4o(); cost != 0 {
		t.Fatalf("gpt-4o costs %v with no model data, want 0", cost)
	}

	// The files are written, as a refresh would.
	modeldatatest.Write(t, dataDir)
	if err := catalog.ForceReloadPricing(ctx); err != nil {
		t.Fatal(err)
	}
	if cost := gpt4o(); math.Abs(cost-2.5) > 1e-9 {
		t.Fatalf("a million gpt-4o input tokens cost %v after the files were written, want 2.5", cost)
	}

	// A refresh changes a price.
	data, err := os.ReadFile(pricingPath)
	if err != nil {
		t.Fatal(err)
	}
	var sheet map[string]map[string]any
	if err := json.Unmarshal(data, &sheet); err != nil {
		t.Fatal(err)
	}
	sheet["gpt-4o"]["input_cost_per_token"] = 1e-5
	if data, err = json.Marshal(sheet); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pricingPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ForceReloadPricing(ctx); err != nil {
		t.Fatal(err)
	}
	if cost := gpt4o(); math.Abs(cost-10) > 1e-9 {
		t.Fatalf("a million gpt-4o input tokens cost %v after the price changed, want 10", cost)
	}
}

// TestEngineForwardsRequestParameters checks that a request's output token limit and
// sampling parameters reach the provider for a model the model data knows, in every API
// format. The model data lists models' capabilities but not the parameters each takes, so
// the engine must not drop the parameters it doesn't list.
func TestEngineForwardsRequestParameters(t *testing.T) {
	var (
		lock   sync.Mutex
		bodies = map[string]map[string]any{}
	)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		lock.Lock()
		bodies[r.URL.Path] = req
		lock.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}],"stop_reason":"max_tokens","usage":{"input_tokens":5,"output_tokens":5}}`)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"length"}],"usage":{"prompt_tokens":5,"completion_tokens":20,"total_tokens":25}}`)
		case strings.HasSuffix(r.URL.Path, "/responses"):
			_, _ = io.WriteString(w, `{"id":"resp_1","object":"response","created_at":1,"status":"incomplete","model":"gpt-4o-mini","output":[{"type":"message","id":"m1","role":"assistant","status":"incomplete","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":5,"output_tokens":20,"total_tokens":25}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer up.Close()

	dataDir := t.TempDir()
	modeldatatest.Write(t, dataDir)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	e := New()
	if err := e.Start(ctx, Options{DataDir: dataDir}); err != nil {
		t.Fatal(err)
	}
	defer e.Shutdown()
	for _, provider := range []string{"openai", "anthropic"} {
		if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{
			"provider":       provider,
			"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true, "default_request_timeout_in_seconds": 30},
		}, nil); err != nil {
			t.Fatal(err)
		}
		if err := e.Do(ctx, http.MethodPost, "/api/providers/"+provider+"/keys", map[string]any{"name": provider + "-key", "value": "sk-test", "models": []string{"*"}, "weight": 1}, nil); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		path     string
		body     string
		upstream string
		limit    string
	}{
		{
			name:     "chat completions max_tokens to OpenAI",
			path:     "/v1/chat/completions",
			body:     `{"model":"openai/gpt-4o-mini","max_tokens":20,"temperature":0.5,"messages":[{"role":"user","content":"hi"}]}`,
			upstream: "/v1/chat/completions",
			limit:    "max_completion_tokens",
		},
		{
			name:     "chat completions max_completion_tokens to Anthropic",
			path:     "/v1/chat/completions",
			body:     `{"model":"anthropic/claude-sonnet-4-5","max_completion_tokens":20,"temperature":0.5,"messages":[{"role":"user","content":"hi"}]}`,
			upstream: "/v1/messages",
			limit:    "max_tokens",
		},
		{
			name:     "messages to Anthropic",
			path:     "/anthropic/v1/messages",
			body:     `{"model":"anthropic/claude-sonnet-4-5","max_tokens":20,"temperature":0.5,"messages":[{"role":"user","content":"hi"}]}`,
			upstream: "/v1/messages",
			limit:    "max_tokens",
		},
		{
			name:     "responses to OpenAI",
			path:     "/v1/responses",
			body:     `{"model":"openai/gpt-4o-mini","max_output_tokens":20,"temperature":0.5,"input":"hi"}`,
			upstream: "/v1/responses",
			limit:    "max_output_tokens",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock.Lock()
			clear(bodies)
			lock.Unlock()
			if code, resp := do(t, e, http.MethodPost, tt.path, tt.body); code != http.StatusOK {
				t.Fatalf("got %d: %s", code, resp)
			}
			lock.Lock()
			sent := bodies[tt.upstream]
			lock.Unlock()
			if sent == nil {
				t.Fatalf("the upstream got no request at %s, got %v", tt.upstream, bodies)
			}
			if sent[tt.limit] != float64(20) || sent["temperature"] != 0.5 {
				t.Fatalf("the upstream got %s=%v and temperature=%v, want 20 and 0.5: %v", tt.limit, sent[tt.limit], sent["temperature"], sent)
			}
		})
	}
}
