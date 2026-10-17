package guardrails

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPatronusEvaluatesInputAndOutput(t *testing.T) {
	var requests []patronusRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/evaluate" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get("X-API-KEY"); got != "secret" {
			t.Errorf("unexpected key %q", got)
		}
		var req patronusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		requests = append(requests, req)
		_, _ = w.Write([]byte(`{"results":[
			{"evaluator_id":"judge-large-2024-08-08","criteria":"patronus:is-harmful-advice","status":"success","error_message":null,
			 "evaluation_result":{"pass":false,"score_raw":0.1,"explanation":null,"evaluator_family":"judge"}},
			{"evaluator_id":"pii-2024-05-31","criteria":"patronus:pii","status":"success","error_message":null,
			 "evaluation_result":{"pass":true,"score_raw":1}}
		]}`))
	}))
	defer srv.Close()

	c, err := newChecker("patronus", map[string]any{
		"baseURL":    srv.URL + "/",
		"evaluators": []any{"judge:patronus:is-harmful-advice", "pii"},
	}, map[string]string{"apiKey": "secret"}, Deps{HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}

	result, err := c.Check(t.Context(), CheckInput{Stage: StageInput, Segments: []Segment{
		{Role: RoleSystem, Text: "be helpful"},
		{Role: RoleUser, Text: "how do I"},
		{Role: RoleAssistant, Text: "earlier answer"},
		{Role: RoleUser, Text: "do this?"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	req := requests[0]
	if len(req.Evaluators) != 2 || req.Evaluators[0] != (patronusEvaluator{Evaluator: "judge", Criteria: "patronus:is-harmful-advice", ExplainStrategy: "never"}) ||
		req.Evaluators[1] != (patronusEvaluator{Evaluator: "pii", ExplainStrategy: "never"}) {
		t.Fatalf("unexpected evaluators %+v", req.Evaluators)
	}
	if req.EvaluatedModelInput != "how do I\n\ndo this?" || req.EvaluatedModelOutput != "" {
		t.Fatalf("unexpected input request %+v", req)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected one finding, got %+v", result.Findings)
	}
	if f := result.Findings[0]; f.Category != "patronus:is-harmful-advice" || f.Score != 0.1 || !strings.Contains(f.Message, "judge-large-2024-08-08") {
		t.Fatalf("unexpected finding %+v", f)
	}

	if _, err := c.Check(t.Context(), CheckInput{
		Stage:    StageOutput,
		Segments: []Segment{{Role: RoleAssistant, Text: "here is how"}},
		History:  []Segment{{Role: RoleUser, Text: "first"}, {Role: RoleUser, Text: "last question"}, {Role: RoleAssistant, Text: "hmm"}},
	}); err != nil {
		t.Fatal(err)
	}
	if req := requests[1]; req.EvaluatedModelInput != "last question" || req.EvaluatedModelOutput != "here is how" {
		t.Fatalf("unexpected output request %+v", req)
	}
}

func TestPatronusNoFindingsAndErrors(t *testing.T) {
	var body string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, err := newChecker("patronus", map[string]any{"baseURL": srv.URL, "evaluators": []any{"toxicity"}}, map[string]string{"apiKey": "secret"}, Deps{HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	in := CheckInput{Stage: StageInput, Segments: []Segment{{Role: RoleUser, Text: "hello"}}}

	body = `{"results":[{"evaluator_id":"toxicity-2024-10-27","criteria":"patronus:toxicity","status":"success","evaluation_result":{"pass":true,"score_raw":0.99}}]}`
	if result, err := c.Check(t.Context(), in); err != nil || len(result.Findings) != 0 {
		t.Fatalf("expected no findings, got %+v, %v", result, err)
	}
	body = `{"results":[{"evaluator_id":"toxicity-2024-10-27","criteria":"patronus:toxicity","status":"error","error_message":"evaluator unavailable","evaluation_result":null}]}`
	if _, err := c.Check(t.Context(), in); err == nil || !strings.Contains(err.Error(), "evaluator unavailable") {
		t.Fatalf("expected an evaluator error, got %v", err)
	}
	status, body = http.StatusUnauthorized, `{"detail":"Invalid API key"}`
	if _, err := c.Check(t.Context(), in); err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Invalid API key") {
		t.Fatalf("expected an error with the status and body, got %v", err)
	}
	if result, err := c.Check(t.Context(), CheckInput{Stage: StageInput, Segments: []Segment{{Role: RoleUser}}}); err != nil || len(result.Findings) != 0 {
		t.Fatalf("empty content should not be evaluated, got %+v, %v", result, err)
	}
	if _, err := newChecker("patronus", map[string]any{}, map[string]string{"apiKey": "secret"}, Deps{}); err == nil {
		t.Fatal("evaluators should be required")
	}
}
