//go:build e2e

package llmgateway

import (
	"fmt"
	"strings"
	"testing"

	"github.com/obot-platform/obot/tests/e2e/harness"
)

// TestToolCallStopReason checks that a Messages client is told to run the tool when the model
// calls one, whichever provider serves it and whether or not it streams. Claude Code streams,
// and stops after the first tool call when told the turn ended.
func TestToolCallStopReason(t *testing.T) {
	u := env.newUser(t, "tool-stop")
	for _, model := range []string{"openai/gpt-e2e-a", "anthropic/claude-e2e-a"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				n := nonce("toolstop")
				res := callLLM(t, u.Gateway(env.A), apiMessages, model,
					n+" "+harness.ToolCall("deploy", map[string]any{"note": n}), stream, map[string]any{
						"tools": toolsFor(apiMessages),
					}).requireOK(t)
				if res.ToolArgs == "" {
					t.Fatalf("want a tool call: %s", truncate(res.Body))
				}
				if res.StopReason != "tool_use" {
					t.Fatalf("want stop_reason tool_use, got %q: %s", res.StopReason, truncate(res.Body))
				}
			})
		}
	}
}

// TestReasoningEffortForModelsThatDontReason checks that reasoning_effort, which clients such as
// Goose send with every request, doesn't reach an OpenAI model that doesn't reason (OpenAI
// rejects it there). The mock's model doesn't reason.
func TestReasoningEffortForModelsThatDontReason(t *testing.T) {
	u := env.newUser(t, "reasoning")
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			n := nonce("effort")
			callLLM(t, u.Gateway(env.A), apiChat, "openai/gpt-e2e-a", n, stream, map[string]any{
				"reasoning_effort": "medium",
			}).requireOK(t)
			reqs := env.mock.InferenceRequests(n)
			if len(reqs) == 0 {
				t.Fatal("the request didn't reach the provider")
			}
			for _, req := range reqs {
				if strings.Contains(req.Body, `"reasoning_effort"`) || strings.Contains(req.Body, `"reasoning":`) {
					t.Fatalf("want no reasoning settings upstream, got %s", truncate(req.Body))
				}
			}
		})
	}
}
