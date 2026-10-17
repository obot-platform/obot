package llmengine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bifrostcore "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/modelcatalog"
	"github.com/maximhq/bifrost/framework/modelcatalog/datasheet"
	"github.com/obot-platform/obot/apiclient/types"
)

// testDatasheet is a small model catalog datasheet, in the format the gateway syncs.
const (
	testDatasheet = `{
	"claude-test-5": {
		"provider": "anthropic",
		"mode": "chat",
		"base_model": "claude-test-5",
		"max_input_tokens": 200000,
		"max_output_tokens": 64000,
		"input_cost_per_token": 3e-06,
		"output_cost_per_token": 1.5e-05,
		"cache_read_input_token_cost": 3e-07,
		"cache_creation_input_token_cost": 3.75e-06
	},
	"claude-old-1": {
		"provider": "anthropic",
		"mode": "chat",
		"is_deprecated": true,
		"input_cost_per_token": 1e-06,
		"output_cost_per_token": 2e-06
	},
	"claude-embed-1": {
		"provider": "anthropic",
		"mode": "embedding",
		"input_cost_per_token": 1e-07
	},
	"gpt-test": {
		"provider": "openai",
		"mode": "chat",
		"input_cost_per_token": 1e-06,
		"output_cost_per_token": 2e-06
	}
}`
)

// testCatalog loads testDatasheet from a file, so tests never fetch the real datasheet.
func testCatalog(t *testing.T) *modelcatalog.ModelCatalog {
	t.Helper()
	path := filepath.Join(t.TempDir(), "datasheet.json")
	if err := os.WriteFile(path, []byte(testDatasheet), 0o600); err != nil {
		t.Fatal(err)
	}
	store := datasheet.New(nil, bifrostcore.NewDefaultLogger(schemas.LogLevelError), datasheet.Config{URL: "file://" + path})
	if err := store.LoadFromURLIntoMemory(t.Context()); err != nil {
		t.Fatal(err)
	}
	return modelcatalog.NewTestCatalogWithDatasheet(store)
}

func TestCatalogModels(t *testing.T) {
	models := catalogModels(testCatalog(t), "anthropic")
	if len(models) != 1 || models[0].ID != "anthropic/claude-test-5" {
		t.Fatalf("expected only the current chat model, got %+v", models)
	}
	m := models[0]
	if m.ContextLength != 200000 || m.MaxOutput != 64000 {
		t.Fatalf("unexpected limits: %+v", m)
	}
	if m.Pricing["prompt"] == "" || m.Pricing["completion"] == "" {
		t.Fatalf("expected pricing, got %v", m.Pricing)
	}
}

func TestListModelsFromCatalogForClaudeSubscriptions(t *testing.T) {
	// Stands in for Anthropic's API, which the gateway lists models from when it has a key.
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = io.WriteString(w, `{"data":[{"id":"claude-live-1","type":"model","display_name":"Claude Live 1","created_at":"2026-01-01T00:00:00Z"}],"has_more":false,"first_id":"claude-live-1","last_id":"claude-live-1"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer up.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var subscriptions atomic.Bool
	e := New()
	catalog := testCatalog(t)
	e.catalog = func() modelCatalog { return catalog }
	if err := e.Start(ctx, Options{
		DataDir: t.TempDir(),
		ClaudeSubscriptions: func(context.Context) (bool, error) {
			return subscriptions.Load(), nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	defer e.Shutdown()

	if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{
		"provider":       "anthropic",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true, "default_request_timeout_in_seconds": 30},
	}, nil); err != nil {
		t.Fatal(err)
	}

	list := func() map[string]map[string]string {
		t.Helper()
		models, err := e.ListModels(ctx)
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]map[string]string{}
		for _, m := range models.Models {
			byID[m.ID] = m.Metadata
		}
		return byID
	}

	configured := func() bool {
		t.Helper()
		names, err := e.ConfiguredProviders(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Contains(names, "anthropic")
	}

	if models := list(); len(models) != 0 {
		t.Fatalf("expected no models without keys or subscriptions, got %v", models)
	}
	// Without keys or subscriptions the provider has nothing to serve, so the models it
	// listed for subscriptions aren't kept, and it doesn't count as usable.
	if configured() {
		t.Fatal("an Anthropic provider without keys or subscriptions keeps its models")
	}
	if has, err := e.hasUpstreamProviders(ctx); err != nil || has {
		t.Fatalf("an Anthropic provider without keys or subscriptions is usable: %v, %v", has, err)
	}

	subscriptions.Store(true)
	if !configured() {
		t.Fatal("an Anthropic provider used for subscriptions doesn't keep its models")
	}
	if has, err := e.hasUpstreamProviders(ctx); err != nil || !has {
		t.Fatalf("an Anthropic provider used for subscriptions isn't usable: %v, %v", has, err)
	}
	models := list()
	ids := make([]string, 0, len(models))
	for id := range models {
		ids = append(ids, id)
	}
	if !slices.Equal(ids, []string{"anthropic/claude-test-5"}) {
		t.Fatalf("expected the catalog's current Anthropic chat models, got %v", ids)
	}
	metadata := models["anthropic/claude-test-5"]
	if metadata["usage"] != string(types.ModelUsageLLM) || metadata["dialect"] != "AnthropicMessages" {
		t.Fatalf("unexpected metadata: %v", metadata)
	}
	cost, ok := CostFromMetadata(metadata)
	if !ok {
		t.Fatalf("expected a cost from %v", metadata)
	}
	if want := (types.ModelCost{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75}); !closeCost(cost, want) {
		t.Fatalf("expected cost per million tokens %+v, got %+v", want, cost)
	}

	// A provider whose keys are only disabled keeps its models, subscriptions or not, like any
	// other provider whose keys are disabled for a while.
	if err := e.Do(ctx, http.MethodPost, "/api/providers/anthropic/keys", map[string]any{"name": "paused", "value": "sk-ant-paused", "models": []string{"*"}, "weight": 1, "enabled": false}, nil); err != nil {
		t.Fatal(err)
	}
	if _, keys, enabled, err := e.anthropicKeys(ctx); err != nil || keys != 1 || enabled != 0 {
		t.Fatalf("want one disabled key, got %d keys, %d enabled: %v", keys, enabled, err)
	}
	subscriptions.Store(false)
	if !configured() {
		t.Fatal("an Anthropic provider with disabled keys lost its models")
	}
	subscriptions.Store(true)
	// The gateway's own model listing lists a provider's models with its keys, so with every
	// key disabled it lists none of them, and Obot's model listing adds the catalog's.
	if got := e.ClaudeSubscriptionModels(ctx); len(got) != 1 || got[0].ID != "anthropic/claude-test-5" || got[0].Name == "" {
		t.Fatalf("want the catalog's Anthropic models for a provider with only disabled keys, got %+v", got)
	}

	// A key lists the provider's own models instead.
	if err := e.Do(ctx, http.MethodPost, "/api/providers/anthropic/keys", map[string]any{"name": "k", "value": "sk-ant-test", "models": []string{"*"}, "weight": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if models := list(); models["anthropic/claude-test-5"] != nil || models["anthropic/claude-live-1"] == nil {
		t.Fatalf("expected the provider's listed models with a key, got %v", models)
	}
	if got := e.ClaudeSubscriptionModels(ctx); len(got) != 0 {
		t.Fatalf("want no models added to the gateway's listing with an enabled key, got %+v", got)
	}
}

func closeCost(a, b types.ModelCost) bool {
	near := func(x, y float64) bool { return x-y < 1e-9 && y-x < 1e-9 }
	return near(a.Input, b.Input) && near(a.Output, b.Output) && near(a.CacheRead, b.CacheRead) && near(a.CacheWrite, b.CacheWrite)
}
