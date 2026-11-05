package supervisor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDraftPolicyRetriesMalformedTests(t *testing.T) {
	good := `{"name":"External sharing","condition":"action.sink.external","effect":"require_approval","tests":[{"name":"external","expected":true,"input":{"subject":{"agent":"claude_code"},"action":{"sink":{"external":true}}}}]}`
	for _, tc := range []struct {
		name string
		bad  string
	}{
		{
			name: "boolean agent",
			bad:  strings.Replace(good, `"agent":"claude_code"`, `"agent":true`, 1),
		},
		{
			name: "malformed JSON",
			bad:  `{"name":`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := &fakeTextModel{responses: []string{tc.bad, good}}
			s := NewWith(nil, writer, WithClock(testClock(t)))
			draft, err := s.DraftPolicy(context.Background(), "Review external sharing", nil)
			if err != nil {
				t.Fatal(err)
			}
			if writer.calls != 2 || !strings.Contains(writer.lastUser, "previous draft was rejected") {
				t.Fatalf("expected one correction with feedback, got %d calls", writer.calls)
			}
			if len(draft.Tests) != 1 || !draft.Tests[0].Actual || draft.Tests[0].Error != "" {
				t.Fatalf("corrected test was not evaluated: %+v", draft.Tests)
			}
		})
	}
}

func TestDraftPolicyRejectsRepeatedMalformedTests(t *testing.T) {
	bad := `{"tests":[{"input":{"subject":{"agent":true}}}]}`
	writer := &fakeTextModel{responses: []string{bad, bad}}
	s := NewWith(nil, writer, WithClock(testClock(t)))
	if _, err := s.DraftPolicy(context.Background(), "Review external sharing", nil); err == nil {
		t.Fatal("expected malformed tests to be rejected without coercion")
	}
	if writer.calls != 2 {
		t.Fatalf("correction must be bounded to one retry, got %d calls", writer.calls)
	}
}

func TestDraftPolicyInputSchemaTypes(t *testing.T) {
	wireInput := draftSchema["properties"].(map[string]any)["tests"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["input"].(map[string]any)
	if wireInput["type"] != "string" {
		t.Fatal("wire schema must keep test inputs compact for provider grammar limits")
	}
	input := mustDraftInputSchema()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"type":[`) {
		t.Fatal("draft schema must use concrete types supported by the gateway providers")
	}
	properties := input["properties"].(map[string]any)
	subject := properties["subject"].(map[string]any)
	agent := subject["properties"].(map[string]any)["agent"].(map[string]any)
	if agent["type"] != "string" || subject["additionalProperties"] != false {
		t.Fatalf("agent must be a string in a closed subject schema: %+v", subject)
	}
	action := properties["action"].(map[string]any)["properties"].(map[string]any)
	external := action["sink"].(map[string]any)["properties"].(map[string]any)["external"].(map[string]any)
	if external["type"] != "boolean" {
		t.Fatalf("external must remain boolean: %+v", external)
	}
}

func TestDraftPolicySerializedInput(t *testing.T) {
	writer := &fakeTextModel{responses: []string{
		`{"name":"External sharing","condition":"action.sink.external","effect":"deny","tests":[{"name":"external","expected":true,"input":"{\"subject\":{\"agent\":true},\"action\":{\"sink\":{\"external\":true}}}"}]}`,
		`{"name":"External sharing","condition":"action.sink.external","effect":"deny","tests":[{"name":"external","expected":true,"input":"{\"subject\":{\"agent\":\"claude_code\"},\"action\":{\"sink\":{\"external\":true}}}"}]}`,
	}}
	s := NewWith(nil, writer, WithClock(testClock(t)))
	draft, err := s.DraftPolicy(context.Background(), "Review external sharing", nil)
	if err != nil {
		t.Fatal(err)
	}
	if writer.calls != 2 || len(draft.Tests) != 1 || !draft.Tests[0].Actual {
		t.Fatalf("serialized input was not repaired and evaluated: %+v", draft)
	}
}
