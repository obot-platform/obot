package handlers

import "testing"

func TestDecodeLocalAgentAuditLogInputsAllowsIngestPayloadWithoutServerFields(t *testing.T) {
	inputs, err := decodeLocalAgentAuditLogInputs([]byte(`{
		"eventID": "event-1",
		"eventName": "post-tool-use",
		"client": {
			"name": "local-agent"
		}
	}`))
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("expected one input, got %d", len(inputs))
	}

	input := inputs[0]
	if input.EventID != "event-1" {
		t.Fatalf("expected eventID to decode, got %q", input.EventID)
	}
	if input.EventName != "post-tool-use" {
		t.Fatalf("expected eventName to decode, got %q", input.EventName)
	}
	if input.Client.Name != "local-agent" {
		t.Fatalf("expected client name to decode, got %q", input.Client.Name)
	}
	if input.Client.Version != "" {
		t.Fatalf("expected client version to be optional, got %q", input.Client.Version)
	}
	if input.CreatedAt != nil {
		t.Fatalf("expected createdAt to be optional, got %v", input.CreatedAt)
	}
}
