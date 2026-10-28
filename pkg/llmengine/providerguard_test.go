package llmengine

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"strings"
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

func TestGuardProviderChangeAddresses(t *testing.T) {
	e, ctx := startTestEngine(t)

	tests := []struct {
		name         string
		baseURL      string
		allowPrivate bool
		wantErr      string
	}{
		{
			name:    "public address",
			baseURL: "https://8.8.8.8/v1",
		},
		{
			name:    "localhost",
			baseURL: "http://localhost:18999",
			wantErr: "needs Allow private network addresses",
		},
		{
			name:    "IPv4 loopback",
			baseURL: "http://127.0.0.1:18999",
			wantErr: "needs Allow private network addresses",
		},
		{
			name:    "IPv6 loopback",
			baseURL: "http://[::1]:18999",
			wantErr: "needs Allow private network addresses",
		},
		{
			name:    "IPv4-mapped loopback",
			baseURL: "http://[::ffff:127.0.0.1]:18999",
			wantErr: "needs Allow private network addresses",
		},
		{
			name:    "private address",
			baseURL: "http://10.1.2.3:8000",
			wantErr: "needs Allow private network addresses",
		},
		{
			name:    "unique local address",
			baseURL: "http://[fd12:3456::1]:8000",
			wantErr: "needs Allow private network addresses",
		},
		{
			name:         "localhost allowed",
			baseURL:      "http://localhost:18999",
			allowPrivate: true,
		},
		{
			name:         "private address allowed",
			baseURL:      "http://[fd12:3456::1]:8000",
			allowPrivate: true,
		},
		{
			name:         "cloud metadata",
			baseURL:      "http://169.254.169.254",
			allowPrivate: true,
			wantErr:      "link-local address, which can't be used",
		},
		{
			name:         "IPv6 cloud metadata",
			baseURL:      "http://[fd00:ec2::254]",
			allowPrivate: true,
			wantErr:      "cloud metadata address, which can't be used",
		},
		{
			name:         "IPv4-mapped cloud metadata",
			baseURL:      "http://[::ffff:169.254.169.254]",
			allowPrivate: true,
			wantErr:      "link-local address, which can't be used",
		},
		{
			name:         "IPv6 link-local",
			baseURL:      "http://[fe80::1]",
			allowPrivate: true,
			wantErr:      "link-local address, which can't be used",
		},
		{
			name:         "unspecified",
			baseURL:      "http://0.0.0.0:18999",
			allowPrivate: true,
			wantErr:      "unspecified address, which can't be used",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := mustJSON(t, map[string]any{
				"provider":               "qa-elh",
				"custom_provider_config": map[string]any{"base_provider_type": "openai"},
				"network_config":         map[string]any{"base_url": tt.baseURL, "allow_private_network": tt.allowPrivate},
			})
			// A new provider, and an update of one that doesn't exist yet, which creates it.
			for _, path := range []string{"providers", "providers/qa-elh"} {
				method := http.MethodPost
				if path != "providers" {
					method = http.MethodPut
				}
				_, err := e.GuardProviderChange(ctx, method, path, body, false)
				switch {
				case tt.wantErr == "" && err != nil:
					t.Errorf("%s %s: unexpected error: %v", method, path, err)
				case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
					t.Errorf("%s %s: expected an error with %q, got %v", method, path, tt.wantErr, err)
				}
			}
		})
	}
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
	change, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/qa-custom", update, false)
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

func TestGuardProviderChangeBaseURLWithKeys(t *testing.T) {
	up := mockOpenAI(t)
	defer up.Close()
	e, ctx := startTestEngine(t)
	if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{
		"provider":       "openai",
		"network_config": map[string]any{"base_url": up.URL, "allow_private_network": true},
	}, nil); err != nil {
		t.Fatal(err)
	}
	var key map[string]any
	if err := e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{
		"name": "stored", "value": "sk-stored-0123456789", "models": []string{"*"}, "weight": 1,
	}, &key); err != nil {
		t.Fatal(err)
	}
	keys, err := e.providerKeys(ctx, "openai")
	if err != nil || len(keys) != 1 {
		t.Fatalf("expected one key, got %v (%v)", keys, err)
	}

	moved := mustJSON(t, map[string]any{"network_config": map[string]any{"base_url": "https://8.8.8.8"}})
	if _, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/openai", moved, false); err == nil || !strings.Contains(err.Error(), "removes its keys") {
		t.Fatalf("expected a base URL change to be refused while the provider has keys, got %v", err)
	}
	change, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/openai", moved, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(change.RemoveKeys) != 1 || change.RemoveKeys[0] != keys[0]["id"] {
		t.Fatalf("expected the key to be removed, got %v", change.RemoveKeys)
	}

	// The same base URL, spelled differently, and other settings keep the keys.
	same := mustJSON(t, map[string]any{"network_config": map[string]any{"base_url": strings.ToUpper(up.URL) + "/", "allow_private_network": true, "max_retries": 3}})
	if change, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/openai", same, false); err != nil || len(change.RemoveKeys) != 0 {
		t.Fatalf("expected an unchanged base URL to keep the keys, got %v, %v", change.RemoveKeys, err)
	}
}

func TestGuardProviderChangeKeyAddresses(t *testing.T) {
	e, ctx := startTestEngine(t)
	if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "vllm"}, nil); err != nil {
		t.Fatal(err)
	}
	vllmKey := func(url, value string) []byte {
		return mustJSON(t, map[string]any{
			"name": "vllm-key", "value": value, "models": []string{"*"}, "weight": 1,
			"vllm_key_config": map[string]any{"url": url, "model_name": "m"},
		})
	}

	// Without private network addresses allowed, a key can't point at one.
	for _, url := range []string{"http://127.0.0.1:8000", "http://localhost:8000", "http://10.1.2.3:8000"} {
		if _, err := e.GuardProviderChange(ctx, http.MethodPost, "providers/vllm/keys", vllmKey(url, "vllm-secret-0123456789"), false); err == nil {
			t.Errorf("expected a key on %s to be refused", url)
		}
	}
	if err := e.Do(ctx, http.MethodPut, "/api/providers/vllm", map[string]any{
		"network_config":              map[string]any{"allow_private_network": true},
		"concurrency_and_buffer_size": map[string]any{"concurrency": 1000, "buffer_size": 5000},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.GuardProviderChange(ctx, http.MethodPost, "providers/vllm/keys", vllmKey("http://169.254.169.254", "vllm-secret-0123456789"), false); err == nil {
		t.Error("expected a key on the metadata address to be refused")
	}
	body := vllmKey("http://10.1.2.3:8000", "vllm-secret-0123456789")
	if _, err := e.GuardProviderChange(ctx, http.MethodPost, "providers/vllm/keys", body, false); err != nil {
		t.Fatal(err)
	}
	if err := e.Do(ctx, http.MethodPost, "/api/providers/vllm/keys", json.RawMessage(body), nil); err != nil {
		t.Fatal(err)
	}
	keys, err := e.providerKeys(ctx, "vllm")
	if err != nil || len(keys) != 1 {
		t.Fatalf("expected one key, got %v (%v)", keys, err)
	}
	id, _ := keys[0]["id"].(string)
	stored := keys[0]

	// The provider can't stop allowing private addresses while a key uses one.
	off := mustJSON(t, map[string]any{"network_config": map[string]any{"allow_private_network": false}})
	if _, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/vllm", off, false); err == nil {
		t.Error("expected turning off private addresses to be refused while a key uses one")
	}

	// Editing the key as listed, such as enabling or disabling it, is fine.
	stored["enabled"] = false
	if _, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/vllm/keys/"+id, mustJSON(t, stored), false); err != nil {
		t.Errorf("expected an edit that keeps the URL to be allowed, got %v", err)
	}
	// Pointing the key somewhere else needs its secret again.
	moved := maps.Clone(stored)
	moved["vllm_key_config"] = map[string]any{"url": "http://10.9.9.9:8000", "model_name": "m"}
	if _, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/vllm/keys/"+id, mustJSON(t, moved), false); err == nil || !strings.Contains(err.Error(), "entering its secrets again") {
		t.Errorf("expected moving the key with its stored secret to be refused, got %v", err)
	}
	delete(moved, "value")
	if _, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/vllm/keys/"+id, mustJSON(t, moved), false); err == nil {
		t.Error("expected moving the key without its secret to be refused")
	}
	moved["value"] = "vllm-new-secret-0123456789"
	if _, err := e.GuardProviderChange(ctx, http.MethodPut, "providers/vllm/keys/"+id, mustJSON(t, moved), false); err != nil {
		t.Errorf("expected moving the key with its secret entered again to be allowed, got %v", err)
	}
}
