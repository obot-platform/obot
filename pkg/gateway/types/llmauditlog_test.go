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
	data, err := json.Marshal(ConvertLLMAuditLog(LLMAuditLog{Subscription: true}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"subscription":true`) || strings.Contains(string(data), `"claudeSubscription"`) || strings.Contains(string(data), `"chatgptSubscription"`) {
		t.Fatalf("incorrect subscription type: %s", data)
	}
}
