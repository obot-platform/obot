//go:build e2e

package llmgateway

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/obot-platform/obot/tests/e2e/harness"
	"github.com/tidwall/gjson"
)

// TestRoutingAndFormats calls every inference endpoint for models on both providers,
// streamed and not, including cross-format calls, and checks model access enforcement.
func TestRoutingAndFormats(t *testing.T) {
	user := env.newUser(t, "routing")
	gw := user.Gateway(env.A)

	for _, model := range []string{"openai/gpt-e2e-a", "anthropic/claude-e2e-a"} {
		for _, api := range allAPIs {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", model, api, stream), func(t *testing.T) {
					n := nonce("route")
					reply := "pong " + n
					res := callLLM(t, gw, api, model, "say pong "+n+" "+harness.Reply(reply), stream, nil).requireOK(t)
					if res.Text != reply {
						t.Fatalf("want reply %q, got %q\n%s", reply, res.Text, truncate(res.Body))
					}
					if stream && !strings.Contains(res.Header.Get("Content-Type"), "text/event-stream") {
						t.Fatalf("want an event stream, got %q", res.Header.Get("Content-Type"))
					}
					reqs := env.mock.InferenceRequests(n)
					if len(reqs) != 1 {
						t.Fatalf("want 1 upstream request, got %d", len(reqs))
					}
					up := reqs[0]
					provider, nativeModel, _ := strings.Cut(model, "/")
					if up.Model != nativeModel {
						t.Fatalf("upstream got model %q, want %q", up.Model, nativeModel)
					}
					switch provider {
					case "anthropic":
						if up.Format != harness.FormatMessages || up.Key != anthropicKey {
							t.Fatalf("anthropic model was sent as %s with key %q", up.Format, up.Key)
						}
					case "openai":
						if (up.Format != harness.FormatChat && up.Format != harness.FormatResponses) || up.Key != openAIKey {
							t.Fatalf("openai model was sent as %s with key %q", up.Format, up.Key)
						}
					}
					// The engine's identity never reaches the client.
					for name := range res.Header {
						if strings.HasPrefix(strings.ToLower(name), "x-bifrost") || strings.HasPrefix(strings.ToLower(name), "x-bf-") {
							t.Fatalf("response leaks engine header %s", name)
						}
					}
					if strings.Contains(res.Body, "extra_fields") || strings.Contains(strings.ToLower(res.Body), "bifrost") {
						t.Fatalf("response leaks engine fields: %s", truncate(res.Body))
					}
					if got := res.Header.Get("X-Obot-LLM-Provider"); !stream && got != "" && got != provider {
						t.Fatalf("X-Obot-LLM-Provider = %q, want %q", got, provider)
					}
				})
			}
		}
	}

	t.Run("bare model IDs resolve to provider/model", func(t *testing.T) {
		for _, tc := range []struct {
			api, model, provider string
		}{
			{api: apiMessages, model: "claude-e2e-b", provider: "anthropic"},
			{api: apiChat, model: "gpt-e2e-b", provider: "openai"},
		} {
			n := nonce("bare")
			callLLM(t, gw, tc.api, tc.model, "hi "+n, false, nil).requireOK(t)
			reqs := env.mock.InferenceRequests(n)
			if len(reqs) != 1 || reqs[0].Model != tc.model {
				t.Fatalf("bare %s: want one upstream request for %s, got %+v", tc.model, tc.model, reqs)
			}
			wantKey := openAIKey
			if tc.provider == "anthropic" {
				wantKey = anthropicKey
			}
			if reqs[0].Key != wantKey {
				t.Fatalf("bare %s went to the wrong provider (key %q)", tc.model, reqs[0].Key)
			}
		}
	})

	t.Run("token counting endpoints", func(t *testing.T) {
		n := nonce("count")
		resp := gw.Do(t, http.MethodPost, "/api/llm-proxy/v1/messages/count_tokens", map[string]any{
			"model":    "anthropic/claude-e2e-a",
			"messages": []any{map[string]any{"role": "user", "content": "count " + n + " @@usage:42:0@@"}},
		}, map[string]string{"anthropic-version": "2023-06-01"})
		if resp.Status != http.StatusOK || gjson.GetBytes(resp.Body, "input_tokens").Int() != 42 {
			t.Fatalf("count_tokens: %s", resp)
		}
		if reqs := env.mock.InferenceRequests(n); len(reqs) != 1 || reqs[0].Format != harness.FormatCountTokens {
			t.Fatalf("count_tokens upstream: %+v", reqs)
		}
	})

	t.Run("embeddings", func(t *testing.T) {
		// Model access policies only grant models whose usage is "llm", so a model the
		// gateway classifies as an embedding model can't be granted to anyone. The endpoint
		// itself is checked with an LLM model; the mock returns embeddings for any model.
		n := nonce("embed")
		resp := gw.Do(t, http.MethodPost, "/api/llm-proxy/v1/embeddings", map[string]any{
			"model": "openai/gpt-e2e-b",
			"input": []string{"embed " + n, "second"},
		})
		if resp.Status != http.StatusOK {
			t.Fatalf("embeddings: %s", resp)
		}
		if got := len(gjson.GetBytes(resp.Body, "data").Array()); got != 2 {
			t.Fatalf("want 2 embeddings, got %d: %s", got, resp)
		}
		if reqs := env.mock.InferenceRequests(n); len(reqs) != 1 || reqs[0].Format != harness.FormatEmbeddings || reqs[0].Model != "gpt-e2e-b" {
			t.Fatalf("embeddings upstream: %+v", reqs)
		}
		if resp := gw.Do(t, http.MethodPost, "/api/llm-proxy/v1/embeddings", map[string]any{
			"model": "openai/text-embedding-e2e", "input": "x",
		}); resp.Status != http.StatusOK {
			t.Logf("NOTE: an embedding-usage model can't be granted by a model access policy, so it is refused: %s", resp)
		}
	})

	t.Run("models lists only allowed models", func(t *testing.T) {
		ids := listedModels(t, gw)
		for _, want := range []string{"openai/gpt-e2e-a", "anthropic/claude-e2e-a", "anthropic/claude-e2e-b"} {
			if !slices.Contains(ids, want) {
				t.Errorf("/v1/models is missing allowed model %s: %v", want, ids)
			}
		}
		for _, hidden := range []string{"openai/gpt-e2e-vip", "openai/gpt-e2e-secret"} {
			if slices.Contains(ids, hidden) {
				t.Errorf("/v1/models lists disallowed model %s", hidden)
			}
		}
		if vip := listedModels(t, env.vipUser.Gateway(env.A)); !slices.Contains(vip, "openai/gpt-e2e-vip") {
			t.Errorf("the vip user's /v1/models is missing the vip model: %v", vip)
		}

		// Anthropic clients get Anthropic's list format.
		resp := gw.Do(t, http.MethodGet, "/api/llm-proxy/v1/models", nil, map[string]string{"anthropic-version": "2023-06-01"})
		if resp.Status != http.StatusOK || !gjson.GetBytes(resp.Body, "has_more").Exists() {
			t.Fatalf("anthropic model list: %s", resp)
		}
		for _, m := range gjson.GetBytes(resp.Body, "data.#.id").Array() {
			if strings.Contains(m.String(), "vip") || strings.Contains(m.String(), "secret") {
				t.Fatalf("anthropic model list includes disallowed %s", m)
			}
		}
	})

	t.Run("disallowed model is forbidden", func(t *testing.T) {
		for _, model := range []string{"openai/gpt-e2e-vip", "openai/gpt-e2e-secret", "gpt-e2e-vip"} {
			n := nonce("forbidden")
			res := callLLM(t, gw, apiChat, model, "hi "+n, false, nil)
			if res.Status != http.StatusForbidden {
				t.Fatalf("%s: want 403, got %d: %s", model, res.Status, truncate(res.Body))
			}
			if len(env.mock.InferenceRequests(n)) != 0 {
				t.Fatalf("%s: a forbidden request reached the provider", model)
			}
		}
		// The vip user may use it.
		callLLM(t, env.vipUser.Gateway(env.A), apiChat, "openai/gpt-e2e-vip", "hi", false, nil).requireOK(t)
	})

	t.Run("disallowed fallback is rejected", func(t *testing.T) {
		n := nonce("fallback-forbidden")
		res := callLLM(t, gw, apiChat, "openai/gpt-e2e-a", "hi "+n, false, map[string]any{
			"fallbacks": []string{"openai/gpt-e2e-vip"},
		})
		if res.Status != http.StatusForbidden {
			t.Fatalf("want 403 for a disallowed fallback, got %d: %s", res.Status, truncate(res.Body))
		}
		if len(env.mock.InferenceRequests(n)) != 0 {
			t.Fatal("a request with a disallowed fallback reached the provider")
		}
	})

	t.Run("unknown model and unsupported endpoint", func(t *testing.T) {
		if res := callLLM(t, gw, apiChat, "openai/no-such-model", "hi", false, nil); res.Status != http.StatusNotFound {
			t.Fatalf("unknown model: want 404, got %d: %s", res.Status, truncate(res.Body))
		}
		resp := gw.Do(t, http.MethodPost, "/api/llm-proxy/v1/images/generations", map[string]any{"model": "openai/gpt-e2e-a", "prompt": "x"})
		if resp.Status != http.StatusBadRequest {
			t.Fatalf("unsupported endpoint: want 400, got %s", resp)
		}
	})
}
