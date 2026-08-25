package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

type scriptedFilterExecutor struct {
	calls []string
	run   func(context.Context, FilterCandidate, json.RawMessage) (FilterExecutionResult, error)
}

type memoryFilterDecisionRecorder struct {
	records []FilterDecisionRecord
	err     error
}

func (e *scriptedFilterExecutor) ExecuteFilter(ctx context.Context, candidate FilterCandidate, payload json.RawMessage) (FilterExecutionResult, error) {
	e.calls = append(e.calls, candidate.ResourceName)
	return e.run(ctx, candidate, payload)
}

func (r *memoryFilterDecisionRecorder) RecordFilterDecision(_ context.Context, record FilterDecisionRecord) error {
	r.records = append(r.records, record)
	return r.err
}

func TestFilterPipelineOrdersDeduplicatesChainsAndStops(t *testing.T) {
	executor := &scriptedFilterExecutor{run: func(_ context.Context, candidate FilterCandidate, payload json.RawMessage) (FilterExecutionResult, error) {
		switch candidate.ResourceName {
		case "filter-a":
			var message map[string]any
			if err := json.Unmarshal(payload, &message); err != nil {
				return FilterExecutionResult{}, err
			}
			message["value"] = "mutated"
			mutation, err := json.Marshal(message)
			if err != nil {
				return FilterExecutionResult{}, err
			}
			return FilterExecutionResult{
				Decision: FilterDecisionMutate,
				Message:  mutation,
				Reason:   "redacted",
			}, nil
		case "filter-b":
			if !strings.Contains(string(payload), `"value":"mutated"`) {
				t.Fatalf("second Filter did not receive mutation: %s", payload)
			}
			return FilterExecutionResult{
				Decision: FilterDecisionReject,
				Reason:   "blocked",
			}, nil
		default:
			t.Fatalf("unexpected Filter %q", candidate.ResourceName)
			return FilterExecutionResult{}, nil
		}
	}}
	recorder := new(memoryFilterDecisionRecorder)
	pipeline := NewFilterPipeline(executor, recorder)
	result := pipeline.Run(t.Context(), FilterPipelineRequest{
		Candidates: []FilterCandidate{
			{
				ResourceID:      "id-b",
				ResourceName:    "filter-b",
				DisplayName:     "Filter B",
				AllowedToMutate: true,
			},
			{
				ResourceID:      "id-a",
				ResourceName:    "filter-a",
				DisplayName:     "Filter A",
				AllowedToMutate: true,
			},
			{
				ResourceID:      "id-a",
				ResourceName:    "filter-a",
				DisplayName:     "Filter A duplicate selector",
				AllowedToMutate: true,
			},
			{
				ResourceID:      "id-c",
				ResourceName:    "filter-c",
				DisplayName:     "Filter C",
				AllowedToMutate: true,
			},
		},
		Payload:         json.RawMessage(`{"value":"original"}`),
		Source:          "mcp",
		Event:           "tools/call",
		Phase:           "request",
		SourceContext:   json.RawMessage(`{"name":"echo"}`),
		MutationAllowed: true,
		ValidateMutation: func(_, _, replacement json.RawMessage) error {
			if !json.Valid(replacement) {
				return errors.New("invalid JSON")
			}
			return nil
		},
	})

	if result.Decision != FilterDecisionReject || result.Reason != "blocked" {
		t.Fatalf("unexpected pipeline result: %#v", result)
	}
	if !slices.Equal(executor.calls, []string{"filter-a", "filter-b"}) {
		t.Fatalf("unexpected Filter order: %v", executor.calls)
	}
	if len(recorder.records) != 2 {
		t.Fatalf("got %d decision records, want 2", len(recorder.records))
	}
	if recorder.records[0].EvaluationID == "" || recorder.records[0].EvaluationID != recorder.records[1].EvaluationID {
		t.Fatalf("decision records did not share an evaluation ID: %#v", recorder.records)
	}
	if recorder.records[0].Sequence != 0 || recorder.records[0].Decision != FilterDecisionMutate || recorder.records[1].Sequence != 1 || recorder.records[1].Decision != FilterDecisionReject {
		t.Fatalf("unexpected recorded decisions: %#v", recorder.records)
	}
	if string(recorder.records[0].SourceContext) != `{"name":"echo"}` {
		t.Fatalf("source context was not retained for protected recording: %#v", recorder.records[0])
	}
}

func TestFilterPipelineNoMatchAcceptsWithoutExecutionOrRecording(t *testing.T) {
	executor := &scriptedFilterExecutor{run: func(context.Context, FilterCandidate, json.RawMessage) (FilterExecutionResult, error) {
		t.Fatal("executor was called")
		return FilterExecutionResult{}, nil
	}}
	recorder := new(memoryFilterDecisionRecorder)
	result := NewFilterPipeline(executor, recorder).Run(t.Context(), FilterPipelineRequest{
		Payload: json.RawMessage(`{"unchanged":true}`),
	})
	if result.Decision != FilterDecisionAccept || string(result.Payload) != `{"unchanged":true}` || result.EvaluationID != "" {
		t.Fatalf("unexpected no-match result: %#v", result)
	}
	if len(recorder.records) != 0 {
		t.Fatalf("recorded decisions for no-match event: %#v", recorder.records)
	}
}

func TestFilterPipelineTimesOutAndStops(t *testing.T) {
	executor := &scriptedFilterExecutor{run: func(ctx context.Context, _ FilterCandidate, _ json.RawMessage) (FilterExecutionResult, error) {
		<-ctx.Done()
		return FilterExecutionResult{}, ctx.Err()
	}}
	pipeline := NewFilterPipeline(executor, nil)
	pipeline.callTimeout = time.Millisecond
	result := pipeline.Run(t.Context(), FilterPipelineRequest{
		Candidates: []FilterCandidate{
			{
				ResourceName: "filter-a",
			},
			{
				ResourceName: "filter-b",
			},
		},
		Payload: json.RawMessage(`{}`),
	})
	if result.Decision != FilterDecisionReject || result.ErrorClass != FilterErrorTimeout || !slices.Equal(executor.calls, []string{"filter-a"}) {
		t.Fatalf("unexpected timeout result: result=%#v calls=%v", result, executor.calls)
	}
}

func TestFilterPipelineRejectsInvalidMutationAndRecorderFailure(t *testing.T) {
	executor := &scriptedFilterExecutor{run: func(context.Context, FilterCandidate, json.RawMessage) (FilterExecutionResult, error) {
		return FilterExecutionResult{
			Decision: FilterDecisionMutate,
			Message:  json.RawMessage(`{"changed":true}`),
		}, nil
	}}

	invalid := NewFilterPipeline(executor, nil).Run(t.Context(), FilterPipelineRequest{
		Candidates: []FilterCandidate{{
			ResourceName:    "filter-a",
			AllowedToMutate: true,
		}},
		Payload:         json.RawMessage(`{"changed":false}`),
		MutationAllowed: true,
		ValidateMutation: func(json.RawMessage, json.RawMessage, json.RawMessage) error {
			return errors.New("identity changed: sensitive detail")
		},
	})
	if invalid.Decision != FilterDecisionReject || invalid.ErrorClass != FilterErrorInvalidMutation || strings.Contains(invalid.Reason, "sensitive detail") {
		t.Fatalf("unexpected invalid-mutation result: %#v", invalid)
	}

	recorder := &memoryFilterDecisionRecorder{err: errors.New("database unavailable")}
	recorded := NewFilterPipeline(executor, recorder).Run(t.Context(), FilterPipelineRequest{
		Candidates: []FilterCandidate{{
			ResourceName: "filter-a",
		}},
		Payload: json.RawMessage(`{}`),
	})
	if recorded.Decision != FilterDecisionReject || recorded.ErrorClass != FilterErrorRecording || len(recorder.records) != 1 {
		t.Fatalf("unexpected recorder-failure result: result=%#v records=%#v", recorded, recorder.records)
	}
}

func TestFilterCandidatesForMCPKeepsDistinctFiltersAndPipelineDeduplicatesSelectors(t *testing.T) {
	hooks := Hooks{
		{
			Name: "tools/call",
			Targets: []HookTarget{
				{Target: "filter-b/check"},
				{Target: "filter-a/check"},
			},
		},
		{
			Name: "*",
			Targets: []HookTarget{
				{Target: "filter-a/check"},
			},
		},
	}
	candidates := FilterCandidatesForMCP(hooks, nil, "tools/call", nil)
	ordered := orderedFilterCandidates(candidates)
	if len(ordered) != 2 || ordered[0].ResourceName != "filter-a/check" || ordered[1].ResourceName != "filter-b/check" {
		t.Fatalf("unexpected candidates: %#v", ordered)
	}
}
