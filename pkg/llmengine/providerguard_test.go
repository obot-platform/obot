package llmengine

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func startTestEngine(t *testing.T) (*Engine, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	e := New()
	if err := e.Start(ctx, Options{DataDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Shutdown)
	return e, ctx
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGuardProviderChangeKeepsCustomProviderConfig(t *testing.T) {
	e, ctx := startTestEngine(t)
	if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{
		"provider":               "qa-custom",
		"custom_provider_config": map[string]any{"base_provider_type": "openai"},
		"network_config":         map[string]any{"base_url": "http://10.1.2.3:8000", "allow_private_network": true},
	}, nil); err != nil {
		t.Fatal(err)
	}

	// What the provider page sends: only the settings it edits.
	update := mustJSON(t, map[string]any{
		"network_config":              map[string]any{"base_url": "http://10.1.2.3:8000", "allow_private_network": true, "default_request_timeout_in_seconds": 45},
		"concurrency_and_buffer_size": map[string]any{"concurrency": 1000, "buffer_size": 5000},
	})
	change, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/qa-custom", update)
	if err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	if err := e.Do(ctx, http.MethodPut, "/api/providers/qa-custom", json.RawMessage(change.Body), &sent); err != nil {
		t.Fatalf("the engine refused the update: %v", err)
	}
	if nc, _ := sent["network_config"].(map[string]any); nc["default_request_timeout_in_seconds"] != float64(45) {
		t.Fatalf("the update wasn't saved: %v", sent["network_config"])
	}
	if cpc, _ := sent["custom_provider_config"].(map[string]any); cpc["base_provider_type"] != "openai" {
		t.Fatalf("the custom provider config wasn't kept: %v", sent["custom_provider_config"])
	}
}
