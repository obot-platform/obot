package llmengine

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
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
