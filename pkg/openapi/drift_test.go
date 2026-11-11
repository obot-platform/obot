package openapi

import (
	"encoding/json"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func driftSchema(t *testing.T, paths map[string]any) []byte {
	t.Helper()
	document := map[string]any{
		"openapi": "3.1.0",
		"info":    map[string]any{"title": "drift", "version": "1"},
		"servers": []any{map[string]any{"url": "https://api.example.test"}},
		"paths":   paths,
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	return data
}

func driftResult(t *testing.T, schema []byte) *Result {
	t.Helper()
	return &Result{Schema: json.RawMessage(schema), BaseURL: "https://api.example.test"}
}

func driftConfig() types.OpenAPIRuntimeConfig {
	return types.OpenAPIRuntimeConfig{BaseURL: "https://api.example.test"}
}

func TestProbeSourceDriftNoChange(t *testing.T) {
	stored := driftSchema(t, map[string]any{
		"/items/{id}": map[string]any{"get": map[string]any{"operationId": "getItem", "parameters": []any{map[string]any{"name": "id", "in": "path", "required": true}}}},
	})
	report, err := ProbeSourceDrift(driftResult(t, stored), stored, driftConfig(), nil)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if report.HasDrift() {
		t.Fatalf("expected no drift, got %+v", report)
	}
	if report.SourceDigest != report.SnapshotDigest {
		t.Fatalf("digests should match for identical schemas")
	}
}

func TestProbeSourceDriftAddedRemovedChanged(t *testing.T) {
	stored := driftSchema(t, map[string]any{
		"/items/{id}": map[string]any{"get": map[string]any{"operationId": "getItem", "parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}}},
		"/removed":    map[string]any{"post": map[string]any{"operationId": "removedOp"}},
	})
	// Upstream adds /added, removes /removed, and changes /items/{id} parameter type.
	source := driftSchema(t, map[string]any{
		"/items/{id}": map[string]any{"get": map[string]any{"operationId": "getItem", "parameters": []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer"}}}}},
		"/added":      map[string]any{"get": map[string]any{"operationId": "addedOp"}},
	})

	report, err := ProbeSourceDrift(driftResult(t, source), stored, driftConfig(), nil)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !report.HasDrift() {
		t.Fatalf("expected drift")
	}
	assertList(t, report.Added, []string{"GET /added"})
	assertList(t, report.Removed, []string{"POST /removed"})
	assertList(t, report.Changed, []string{"GET /items/{id}"})
	if report.SourceDigest == report.SnapshotDigest {
		t.Fatalf("digests should differ")
	}
}

func TestProbeSourceDriftDescriptionOnlyChangeIsReported(t *testing.T) {
	stored := driftSchema(t, map[string]any{
		"/items": map[string]any{"get": map[string]any{"operationId": "list", "description": "old text"}},
	})
	source := driftSchema(t, map[string]any{
		"/items": map[string]any{"get": map[string]any{"operationId": "list", "description": "new text"}},
	})
	report, err := ProbeSourceDrift(driftResult(t, source), stored, driftConfig(), nil)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	assertList(t, report.Changed, []string{"GET /items"})
}

func TestProbeSourceDriftRejectsMissingInputs(t *testing.T) {
	stored := driftSchema(t, map[string]any{"/items": map[string]any{"get": map[string]any{"operationId": "list"}}})
	if _, err := ProbeSourceDrift(nil, stored, driftConfig(), nil); err == nil {
		t.Fatalf("nil fetched result must error")
	}
	if _, err := ProbeSourceDrift(driftResult(t, stored), nil, driftConfig(), nil); err == nil {
		t.Fatalf("nil stored snapshot must error")
	}
}

func assertList(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("list mismatch: got %v want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("list mismatch: got %v want %v", got, want)
		}
	}
}
