//go:build e2e

package llmgateway

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/tests/e2e/harness"
)

// TestChatCompletionsOnlyProvider checks a custom OpenAI-compatible provider whose server only
// serves Chat Completions, as many self-hosted servers do, set up as the UI sets it up: its
// base URL ends in /v1, and it doesn't serve the Responses API. Every API format works with its
// models, through the server's Chat Completions.
func TestChatCompletionsOnlyProvider(t *testing.T) {
	user := env.newUser(t, "chatonly")
	gw := user.Gateway(env.A)

	provider := "e2echat" + harness.RandomSuffix()
	key := "e2e-upstream-" + provider
	env.mock.SetModels(key, "chat-model")
	env.mock.SetChatOnly(key)
	env.admin.Post(t, "/api/llm-gateway/admin/providers", map[string]any{
		"provider": provider,
		"network_config": map[string]any{
			// As such servers document their base URL.
			"base_url":              env.mock.URL + "/v1",
			"allow_private_network": true,
		},
		"custom_provider_config": map[string]any{
			"base_provider_type": "openai",
			"allowed_requests": map[string]bool{
				"list_models":            true,
				"model_retrieve":         true,
				"text_completion":        true,
				"text_completion_stream": true,
				"chat_completion":        true,
				"chat_completion_stream": true,
				"embedding":              true,
			},
		},
	}, nil)
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/llm-gateway/admin/providers/"+provider, nil)
	})

	var created struct {
		NetworkConfig struct {
			BaseURL string `json:"base_url"`
		} `json:"network_config"`
	}
	env.admin.Get(t, "/api/llm-gateway/admin/providers/"+provider, &created)
	if created.NetworkConfig.BaseURL != env.mock.URL {
		t.Fatalf("base URL %q, want %q without the /v1 the gateway adds to every path", created.NetworkConfig.BaseURL, env.mock.URL)
	}

	addKey(t, env.admin, provider, "key-"+provider, key)
	target := provider + "/chat-model"
	env.waitForModels(t, env.admin, target)
	env.admin.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
		DisplayName: "e2e " + provider,
		Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: user.ID}},
		Models:      []types.ModelResource{{ID: env.modelID(t, target)}},
	}, nil)
	env.waitForModelAccess(t, gw, target)

	for _, api := range allAPIs {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", api, stream), func(t *testing.T) {
				n := nonce("chatonly")
				reply := "chat only reply " + n
				res := callLLM(t, gw, api, target, "hi "+n+" "+harness.Reply(reply), stream, nil).requireOK(t)
				if res.Text != reply {
					t.Fatalf("want the reply %q, got %q", reply, res.Text)
				}
				for _, r := range env.mock.InferenceRequests(n) {
					if r.Path != "/v1/chat/completions" {
						t.Fatalf("the gateway called %s upstream, want only /v1/chat/completions", r.Path)
					}
				}
			})
		}
	}
}

// TestCustomProviderSettingsUpdate checks the provider page's saves for a custom
// OpenAI-compatible provider: turning the Responses API on and off by changing its allowed
// requests, keeping them when a save leaves custom_provider_config out, and the gateway's
// error for a queue size below the concurrency, which the page shows in the field's words.
func TestCustomProviderSettingsUpdate(t *testing.T) {
	provider := "e2eresp" + harness.RandomSuffix()
	chatOnly := map[string]bool{
		"list_models":            true,
		"model_retrieve":         true,
		"text_completion":        true,
		"text_completion_stream": true,
		"chat_completion":        true,
		"chat_completion_stream": true,
		"embedding":              true,
	}
	path := "/api/llm-gateway/admin/providers/" + provider
	env.admin.Post(t, "/api/llm-gateway/admin/providers", map[string]any{
		"provider": provider,
		"network_config": map[string]any{
			"base_url":              env.mock.URL,
			"allow_private_network": true,
		},
		"custom_provider_config": map[string]any{
			"base_provider_type": "openai",
			"is_key_less":        true,
			"allowed_requests":   chatOnly,
		},
	}, nil)
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, path, nil)
	})

	type providerConfig struct {
		NetworkConfig            map[string]any `json:"network_config"`
		ConcurrencyAndBufferSize map[string]any `json:"concurrency_and_buffer_size"`
		CustomProviderConfig     map[string]any `json:"custom_provider_config"`
	}
	get := func() providerConfig {
		var p providerConfig
		env.admin.Get(t, path, &p)
		return p
	}
	// save sends what the provider page sends: the network settings and queue sizes, and the
	// custom provider config only when the Responses API setting changed.
	save := func(custom map[string]any, concurrency, bufferSize int) *harness.Response {
		body := map[string]any{
			"network_config":              get().NetworkConfig,
			"concurrency_and_buffer_size": map[string]any{"concurrency": concurrency, "buffer_size": bufferSize},
		}
		if custom != nil {
			body["custom_provider_config"] = custom
		}
		return env.admin.Do(t, http.MethodPut, path, body)
	}
	servesResponses := func() bool {
		allowed, _ := get().CustomProviderConfig["allowed_requests"].(map[string]any)
		return allowed == nil || allowed["responses"] == true
	}

	if servesResponses() {
		t.Fatal("a provider created without the Responses API serves it")
	}

	custom := get().CustomProviderConfig
	delete(custom, "allowed_requests")
	if resp := save(custom, 100, 500); resp.Status != http.StatusOK {
		t.Fatalf("turning on the Responses API: %s", resp)
	}
	if !servesResponses() {
		t.Fatal("the provider doesn't serve the Responses API after turning it on")
	}

	custom = get().CustomProviderConfig
	custom["allowed_requests"] = chatOnly
	if resp := save(custom, 100, 500); resp.Status != http.StatusOK {
		t.Fatalf("turning off the Responses API: %s", resp)
	}
	if servesResponses() {
		t.Fatal("the provider serves the Responses API after turning it off")
	}

	if resp := save(nil, 200, 600); resp.Status != http.StatusOK {
		t.Fatalf("saving without the custom provider config: %s", resp)
	}
	if got := get(); servesResponses() || got.CustomProviderConfig["base_provider_type"] != "openai" {
		t.Fatalf("a save without the custom provider config changed it: %v", got.CustomProviderConfig)
	}

	resp := save(nil, 600, 200)
	if resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), "Concurrency must be less than or equal to buffer size") {
		t.Fatalf("a queue size below the concurrency: want 400 with the message the provider page maps to its field, got %s", resp)
	}
}
