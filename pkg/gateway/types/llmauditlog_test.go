package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// What message policies did to requests made before the LLM gateway replaced them stays in
// the database, but isn't shown.
func TestConvertLLMAuditLogLeavesOutMessagePolicies(t *testing.T) {
	data, err := json.Marshal(ConvertLLMAuditLog(LLMAuditLog{
		ID:                        "a1",
		MessagePolicyTriggered:    true,
		PolicyModifiedRequestBody: json.RawMessage(`{"prompt":"blocked"}`),
		Subscription:              true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"messagePolicyTriggered", "policyModifiedRequestBody", "blocked"} {
		if strings.Contains(string(data), field) {
			t.Errorf("the audit log entry shows %q: %s", field, data)
		}
	}
	if !strings.Contains(string(data), `"subscription":true`) {
		t.Errorf("the audit log entry doesn't say a Claude subscription paid for it: %s", data)
	}
}

func TestConvertLLMAuditLogSubscription(t *testing.T) {
	for _, tc := range []struct {
		name               string
		provider           string
		subscription       bool
		claudeSubscription bool
	}{
		{
			name:               "Claude subscription",
			provider:           "anthropic",
			subscription:       true,
			claudeSubscription: true,
		},
		{
			name:         "ChatGPT subscription",
			provider:     "openai",
			subscription: true,
		},
		{
			name:     "Anthropic configured key",
			provider: "anthropic",
		},
		{
			name:     "OpenAI configured key",
			provider: "openai",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(ConvertLLMAuditLog(LLMAuditLog{
				ServedProvider: tc.provider,
				Subscription:   tc.subscription,
			}))
			if err != nil {
				t.Fatal(err)
			}
			// Older consumers decode only claudeSubscription and ignore the new field.
			var legacy struct {
				ClaudeSubscription bool `json:"claudeSubscription"`
			}
			if err := json.Unmarshal(data, &legacy); err != nil {
				t.Fatal(err)
			}
			if legacy.ClaudeSubscription != tc.claudeSubscription ||
				strings.Contains(string(data), `"subscription":true`) != tc.subscription ||
				strings.Contains(string(data), `"claudeSubscription":true`) != tc.claudeSubscription ||
				strings.Contains(string(data), `"chatgptSubscription"`) {
				t.Fatalf("incorrect subscription attribution: %s", data)
			}
		})
	}
}
