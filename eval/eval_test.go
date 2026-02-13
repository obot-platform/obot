package eval

import (
	"os"
	"strings"
	"testing"
)

// TestEvalNanobotWorkflow runs all nanobot evals when OBOT_EVAL_BASE_URL is set.
// Set OBOT_EVAL_BASE_URL (and OBOT_EVAL_AUTH_HEADER for authenticated APIs) to run against a live instance.
// Set OBOT_EVAL_REAL_ONLY=1 to run only real scenarios (exclude in-process mock cases).
// Example: OBOT_EVAL_BASE_URL=http://localhost:8080 OBOT_EVAL_AUTH_HEADER="Bearer <token>" go test -v ./eval/...
func TestEvalNanobotWorkflow(t *testing.T) {
	cases := AllCases()
	if os.Getenv("OBOT_EVAL_REAL_ONLY") == "1" {
		cases = RealOnlyCases()
	}
	results, err := RunFromEnv(cases)
	if err != nil {
		t.Fatalf("run evals: %v", err)
	}
	if results == nil {
		t.Skip("OBOT_EVAL_BASE_URL not set; skipping evals")
		return
	}

	passed := PassCount(results)
	for _, r := range results {
		t.Logf("[%s] pass=%v duration=%v msg=%s", r.Name, r.Pass, r.Duration, r.Message)
		if len(r.Trajectory) > 0 {
			for _, step := range r.Trajectory {
				t.Logf("  step: %s", step)
			}
		}
	}

	// Print chat UI URL(s) at the end so you can verify the nanobot chat in the browser.
	var viewURLs []string
	for _, r := range results {
		if idx := strings.Index(r.Message, "View in UI:"); idx >= 0 {
			url := strings.TrimSpace(r.Message[idx+len("View in UI:"):])
			// Trim trailing sentence if present
			if end := strings.Index(url, " "); end > 0 {
				url = url[:end]
			}
			if url != "" && (strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
				viewURLs = append(viewURLs, url)
			}
		}
	}
	if len(viewURLs) > 0 {
		t.Logf("---")
		t.Logf("Chat UI URL(s) (open in browser to verify nanobot chat):")
		for _, u := range viewURLs {
			t.Logf("  %s", u)
		}
		t.Logf("---")
	}

	if passed < len(results) {
		t.Errorf("evals: %d/%d passed", passed, len(results))
	}
}

// TestEvalMockToolOutput runs only the mock MCP tool-output eval (no Obot required).
func TestEvalMockToolOutput(t *testing.T) {
	cases := []Case{
		{Name: "nanobot_mock_tool_output", Description: "Mock MCP echo tool deterministic output", Run: runMockToolOutput},
	}
	results, err := RunAll(cases, "http://localhost:8080", "")
	if err != nil {
		t.Fatalf("run evals: %v", err)
	}
	if len(results) != 1 || !results[0].Pass {
		if len(results) > 0 {
			t.Errorf("mock tool output eval failed: %s", results[0].Message)
		} else {
			t.Error("no result from mock eval")
		}
	}
}

// TestEvalWorkflowContentPublishing runs only the content-publishing workflow response eval.
// Set OBOT_EVAL_CAPTURED_RESPONSE or OBOT_EVAL_CAPTURED_RESPONSE_FILE with the nanobot response text.
// No Obot instance or auth required.
func TestEvalWorkflowContentPublishing(t *testing.T) {
	cases := []Case{
		{Name: "nanobot_workflow_content_publishing_eval", Description: "Evaluate captured workflow response", Run: runWorkflowContentPublishingEval},
	}
	// Use a placeholder base URL; the workflow case only reads captured response.
	results, err := RunAll(cases, "http://localhost:8080", "")
	if err != nil {
		t.Fatalf("run evals: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.Pass && r.Message == "no captured response: set OBOT_EVAL_CAPTURED_RESPONSE or OBOT_EVAL_CAPTURED_RESPONSE_FILE" {
		t.Skip("no captured response set; skipping workflow eval")
		return
	}
	if !r.Pass {
		t.Errorf("workflow content publishing eval failed: %s", r.Message)
	}
}

// TestEvalWriteResults writes results to a file when OBOT_EVAL_RESULTS_JSON is set.
// Run with: OBOT_EVAL_BASE_URL=... OBOT_EVAL_AUTH_HEADER=... OBOT_EVAL_RESULTS_JSON=results.json go test -v -run TestEvalNanobotWorkflow ./eval
// Then optionally: OBOT_EVAL_RESULTS_JSON=results.json go test -v -run TestEvalWriteResults ./eval
func TestEvalWriteResults(t *testing.T) {
	path := os.Getenv("OBOT_EVAL_RESULTS_JSON")
	if path == "" {
		t.Skip("OBOT_EVAL_RESULTS_JSON not set")
		return
	}
	results, err := RunFromEnv(AllCases())
	if err != nil {
		t.Fatalf("run evals: %v", err)
	}
	if results == nil {
		t.Skip("OBOT_EVAL_BASE_URL not set")
		return
	}
	if err := WriteResultsJSON(results, path); err != nil {
		t.Fatalf("write results: %v", err)
	}
	t.Logf("wrote %d results to %s", len(results), path)
}
