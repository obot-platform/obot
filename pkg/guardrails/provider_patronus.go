package guardrails

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

const (
	patronusDefaultBaseURL = "https://api.patronus.ai"
)

// patronusChecker runs Patronus AI evaluators. The request and response shapes follow the
// EvaluateRequest and EvaluateResponse types of the official patronus Python SDK
// (patronus/api/api_types.py), since the API reference at docs.patronus.ai requires a login.
type patronusChecker struct {
	client     *http.Client
	url        string
	apiKey     string
	evaluators []patronusEvaluator
}

type patronusEvaluator struct {
	Evaluator string `json:"evaluator"`
	Criteria  string `json:"criteria,omitempty"`
	// ExplainStrategy is "never": explanations may quote the checked content, so they
	// aren't recorded, and skipping them makes evaluation faster.
	ExplainStrategy string `json:"explain_strategy"`
}

type patronusRequest struct {
	Evaluators           []patronusEvaluator `json:"evaluators"`
	EvaluatedModelInput  string              `json:"evaluated_model_input,omitempty"`
	EvaluatedModelOutput string              `json:"evaluated_model_output,omitempty"`
}

type patronusResponse struct {
	Results []struct {
		EvaluatorID      string `json:"evaluator_id"`
		Criteria         string `json:"criteria"`
		Status           string `json:"status"`
		ErrorMessage     string `json:"error_message"`
		EvaluationResult *struct {
			Pass     *bool    `json:"pass"`
			ScoreRaw *float64 `json:"score_raw"`
		} `json:"evaluation_result"`
	} `json:"results"`
}

func init() {
	Register(ProviderInfo{
		Type:        "patronus",
		Label:       "Patronus AI",
		Category:    "Integrations",
		Description: "Evaluates prompts and responses with Patronus AI evaluators, such as its toxicity and PII detectors or LLM judges with managed or custom criteria.",
		Fields: []Field{
			{Name: "apiKey", Label: "API key", Type: FieldPassword, Required: true, Sensitive: true},
			{Name: "evaluators", Label: "Evaluators", Type: FieldList, Required: true, Description: "One evaluator per entry, as evaluator or evaluator:criteria, e.g. judge:patronus:is-harmful-advice, pii or toxicity."},
			{Name: "baseURL", Label: "Base URL", Type: FieldText, Destination: true, Default: patronusDefaultBaseURL},
		},
	}, func(config map[string]any, secrets map[string]string, deps Deps) (Checker, error) {
		var evaluators []patronusEvaluator
		for _, raw := range ConfigStrings(config, "evaluators") {
			evaluator, criteria, _ := strings.Cut(strings.TrimSpace(raw), ":")
			if evaluator == "" {
				return nil, fmt.Errorf("invalid Patronus evaluator %q", raw)
			}
			evaluators = append(evaluators, patronusEvaluator{Evaluator: evaluator, Criteria: criteria, ExplainStrategy: "never"})
		}
		if len(evaluators) == 0 {
			return nil, fmt.Errorf("add at least one Patronus evaluator")
		}
		return &patronusChecker{
			client:     integrationClient(deps),
			url:        strings.TrimRight(ConfigString(config, "baseURL", patronusDefaultBaseURL), "/") + "/v1/evaluate",
			apiKey:     secrets["apiKey"],
			evaluators: evaluators,
		}, nil
	})
}

func (c *patronusChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	req := patronusRequest{Evaluators: c.evaluators}
	if in.Stage == StageOutput {
		// Evaluate the response, in the context of the last thing the user asked.
		req.EvaluatedModelOutput = joinSegments(in.Segments, nil)
		for _, s := range slices.Backward(in.History) {
			if s.Role == RoleUser && s.Text != "" {
				req.EvaluatedModelInput = s.Text
				break
			}
		}
		if req.EvaluatedModelOutput == "" {
			return Result{}, nil
		}
	} else {
		// Evaluate what the caller sent: user messages, and tool calls and results.
		req.EvaluatedModelInput = joinSegments(in.Segments, []string{RoleUser, RoleToolCall, RoleToolResult})
		if req.EvaluatedModelInput == "" {
			return Result{}, nil
		}
	}

	var resp patronusResponse
	if err := integrationPost(ctx, c.client, "Patronus AI", c.url, map[string]string{"X-API-KEY": c.apiKey}, req, &resp); err != nil {
		return Result{}, err
	}

	var result Result
	for _, r := range resp.Results {
		if r.Status != "" && r.Status != "success" {
			return Result{}, fmt.Errorf("evaluator %s failed in Patronus AI with status %q: %s", r.EvaluatorID, r.Status, r.ErrorMessage)
		}
		if r.EvaluationResult == nil || r.EvaluationResult.Pass == nil || *r.EvaluationResult.Pass {
			continue
		}
		// The explanation isn't recorded, as it may quote the checked content.
		f := Finding{Category: r.EvaluatorID, Message: "Failed Patronus evaluator " + r.EvaluatorID}
		if r.Criteria != "" {
			f.Category = r.Criteria
			f.Message += " (" + r.Criteria + ")"
		}
		if r.EvaluationResult.ScoreRaw != nil {
			f.Score = *r.EvaluationResult.ScoreRaw
		}
		result.Findings = append(result.Findings, f)
	}
	return result, nil
}

// joinSegments joins the text of the non-empty segments with the given roles, or of every
// non-empty segment when roles is nil, separated by blank lines.
func joinSegments(segments []Segment, roles []string) string {
	var parts []string
	for _, s := range segments {
		if s.Text != "" && (roles == nil || slices.Contains(roles, s.Role)) {
			parts = append(parts, s.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}
