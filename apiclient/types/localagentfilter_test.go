package types

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLocalAgentFilterEventV1Fixture(t *testing.T) {
	data, err := os.ReadFile("testdata/localagentfilter/v1/tool-arguments.json")
	if err != nil {
		t.Fatal(err)
	}

	var event LocalAgentFilterEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	assertEquivalentJSON(t, data, event)

	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["device"]; ok {
		t.Fatal("device identity must not be accepted from the event body")
	}
	if _, ok := wire["deploymentId"]; ok {
		t.Fatal("deployment identity must not be accepted from the event body")
	}

	wire["device"] = json.RawMessage(`{"id":"untrusted-device"}`)
	wire["deploymentId"] = json.RawMessage(`"untrusted-deployment"`)
	untrustedData, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var submitted LocalAgentFilterEvent
	if err := json.Unmarshal(untrustedData, &submitted); err != nil {
		t.Fatal(err)
	}
	sanitizedData, err := json.Marshal(submitted)
	if err != nil {
		t.Fatal(err)
	}
	var sanitized map[string]json.RawMessage
	if err := json.Unmarshal(sanitizedData, &sanitized); err != nil {
		t.Fatal(err)
	}
	if _, ok := sanitized["device"]; ok {
		t.Fatal("untrusted device identity was retained")
	}
	if _, ok := sanitized["deploymentId"]; ok {
		t.Fatal("untrusted deployment identity was retained")
	}
}

func TestLocalAgentFilterEventValidate(t *testing.T) {
	data, err := os.ReadFile("testdata/localagentfilter/v1/tool-arguments.json")
	if err != nil {
		t.Fatal(err)
	}

	var valid LocalAgentFilterEvent
	if err := json.Unmarshal(data, &valid); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*LocalAgentFilterEvent)
	}{
		{name: "unknown API version", mutate: func(e *LocalAgentFilterEvent) { e.APIVersion = "future/v2" }},
		{name: "missing occurrence time", mutate: func(e *LocalAgentFilterEvent) { e.OccurredAt = Time{} }},
		{name: "invalid event", mutate: func(e *LocalAgentFilterEvent) { e.Event.Phase = FilterPhaseResponse }},
		{name: "missing provider", mutate: func(e *LocalAgentFilterEvent) { e.Context.LocalAgent.Provider = "" }},
		{name: "invalid payload", mutate: func(e *LocalAgentFilterEvent) { e.Payload = json.RawMessage(`{`) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := *valid.DeepCopy()
			tt.mutate(&event)
			if err := event.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
