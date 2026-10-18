//go:build e2e

package llmgateway

import (
	"fmt"
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
