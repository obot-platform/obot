//go:build e2e

package llmgateway

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/tests/e2e/harness"
)

// TestFallbacksAndRetries checks that a failing primary model falls back to the request's
// fallback models, and that a provider configured with retries retries transient failures.
func TestFallbacksAndRetries(t *testing.T) {
	user := env.newUser(t, "fallback")
	gw := user.Gateway(env.A)

	for _, api := range allAPIs {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", api, stream), func(t *testing.T) {
				n := nonce("fallback")
				reply := "fallback reply " + n
				res := callLLM(t, gw, api, "openai/gpt-e2e-broken", "hi "+n+" "+harness.Reply(reply), stream, map[string]any{
					"fallbacks": []string{"anthropic/claude-e2e-a"},
				}).requireOK(t)
				if res.Text != reply {
					t.Fatalf("want the fallback's reply %q, got %q", reply, res.Text)
				}
				reqs := env.mock.InferenceRequests(n)
				if len(reqs) < 2 {
					t.Fatalf("want the primary and the fallback upstream, got %d requests", len(reqs))
				}
				if reqs[0].Model != "gpt-e2e-broken" {
					t.Fatalf("first upstream request went to %s, want the primary", reqs[0].Model)
				}
				last := reqs[len(reqs)-1]
				if last.Model != "claude-e2e-a" || last.Format != harness.FormatMessages {
					t.Fatalf("last upstream request: %s %s, want the anthropic fallback", last.Format, last.Model)
				}
			})
		}
	}

	t.Run("bare fallback model ID", func(t *testing.T) {
		n := nonce("fallback-bare")
		callLLM(t, gw, apiChat, "anthropic/claude-e2e-broken", "hi "+n, false, map[string]any{
			"fallbacks": []string{"gpt-e2e-b"},
		}).requireOK(t)
		reqs := env.mock.InferenceRequests(n)
		if last := reqs[len(reqs)-1]; last.Model != "gpt-e2e-b" || last.Key != openAIKey {
			t.Fatalf("fallback went to %s with key %q", last.Model, last.Key)
		}
	})

	t.Run("no fallback: the error is returned", func(t *testing.T) {
		res := callLLM(t, gw, apiChat, "openai/gpt-e2e-broken", "hi "+nonce("nofallback"), false, nil)
		if res.Status < 500 {
			t.Fatalf("want a 5xx from a failing model with no fallback, got %d: %s", res.Status, truncate(res.Body))
		}
	})

	t.Run("provider retries transient failures", func(t *testing.T) {
		provider := "e2eretry" + harness.RandomSuffix()
		key := "e2e-upstream-" + provider
		env.mock.SetModels(key, "retry-model")
		env.admin.Post(t, "/api/llm-gateway/admin/providers", map[string]any{
			"provider": provider,
			"network_config": map[string]any{
				"base_url":                           env.mock.URL,
				"allow_private_network":              true,
				"default_request_timeout_in_seconds": 30,
				"max_retries":                        3,
				"retry_backoff_initial":              100,
				"retry_backoff_max":                  200,
			},
			"concurrency_and_buffer_size": map[string]any{"concurrency": 10, "buffer_size": 100},
			"custom_provider_config":      map[string]any{"base_provider_type": "openai"},
		}, nil)
		t.Cleanup(func() {
			env.admin.Do(t, http.MethodDelete, "/api/llm-gateway/admin/providers/"+provider, nil)
		})
		addKey(t, env.admin, provider, "k1", key)
		target := provider + "/retry-model"
		env.waitForModels(t, env.admin, target)
		env.admin.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
			DisplayName: "e2e " + provider,
			Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: user.ID}},
			Models:      []types.ModelResource{{ID: env.modelID(t, target)}},
		}, nil)
		env.waitForModelAccess(t, gw, target)

		n := nonce("retry")
		var res *llmResult
		harness.Eventually(t, 30*time.Second, "the retrying provider on A", func() error {
			res = callLLM(t, gw, apiChat, target, "hi "+n+" @@flaky:2@@", false, nil)
			if res.Status == http.StatusNotFound || res.Status == http.StatusServiceUnavailable && len(env.mock.InferenceRequests(n)) == 0 {
				return fmt.Errorf("HTTP %d", res.Status)
			}
			return nil
		})
		res.requireOK(t)
		if got := len(env.mock.InferenceRequests(n)); got != 3 {
			t.Fatalf("want 2 failed attempts and 1 success upstream, got %d attempts", got)
		}
	})
}
