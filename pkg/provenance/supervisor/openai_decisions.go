package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/provenance"
)

// OpenAIDecisionsDecider calls an OpenAI Decisions-API-shaped endpoint: the
// model is given a question and predefined options and picks one.
//
// The shape mirrors the DevDay 2026 preview and may need adjusting when the
// API is final. Assumed:
//
//	POST {base}/v1/decisions
//	Authorization: Bearer {apiKey}
//	{"model": "...", "question": "...", "context": "...", "guidance": "...",
//	 "options": [{"key": "...", "description": "..."}]}
//
//	200 {"option": "<key>", "rationale": "...", "confidence": 0..1, "model": "..."}
//
// The response is also accepted with "choice" in place of "option", and the
// chosen key must be one of the offered options.

const (
	// DefaultOpenAIDecisionsBaseURL is the API base when none is configured.
	DefaultOpenAIDecisionsBaseURL = "https://api.openai.com"
	// DefaultOpenAIDecisionsModel is the model when none is configured.
	DefaultOpenAIDecisionsModel = "gpt-5-mini"
	// decisionsPath is the endpoint relative to the base URL.
	decisionsPath = "/v1/decisions"
	// maxErrorBody bounds the error excerpt kept from a failed response.
	maxErrorBody = 512
)

// OpenAIDecisionsDecider implements provenance.Decider over the Decisions API.
type OpenAIDecisionsDecider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// OpenAIDecisionsOption configures an OpenAIDecisionsDecider.
type OpenAIDecisionsOption func(*OpenAIDecisionsDecider)

// decisionsRequest is the request body.
type decisionsRequest struct {
	Model    string              `json:"model"`
	Question string              `json:"question"`
	Context  string              `json:"context"`
	Guidance string              `json:"guidance,omitempty"`
	Options  []provenance.Choice `json:"options"`
}

// decisionsResponse is the response body.
type decisionsResponse struct {
	Option     string  `json:"option"`
	Choice     string  `json:"choice"`
	Rationale  string  `json:"rationale"`
	Confidence float64 `json:"confidence"`
	Model      string  `json:"model"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// WithHTTPClient sets the HTTP client, for tests and custom transports.
func WithHTTPClient(c *http.Client) OpenAIDecisionsOption {
	return func(d *OpenAIDecisionsDecider) { d.client = c }
}

// NewOpenAIDecisionsDecider creates a decider. Empty baseURL and model use the
// defaults.
func NewOpenAIDecisionsDecider(baseURL, apiKey, model string, opts ...OpenAIDecisionsOption) *OpenAIDecisionsDecider {
	d := &OpenAIDecisionsDecider{
		baseURL: strings.TrimRight(firstNonEmpty(baseURL, DefaultOpenAIDecisionsBaseURL), "/"),
		apiKey:  apiKey,
		model:   firstNonEmpty(model, DefaultOpenAIDecisionsModel),
		client:  &http.Client{Timeout: 60 * time.Second},
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Decide posts the question and validates the chosen option.
func (d *OpenAIDecisionsDecider) Decide(ctx context.Context, q provenance.Question) (provenance.Answer, error) {
	if len(q.Options) == 0 {
		return provenance.Answer{}, errors.New("a bounded decision needs at least one option")
	}
	body, err := json.Marshal(decisionsRequest{
		Model: d.model, Question: q.Question, Context: q.Context, Guidance: q.Guidance, Options: q.Options,
	})
	if err != nil {
		return provenance.Answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+decisionsPath, bytes.NewReader(body))
	if err != nil {
		return provenance.Answer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return provenance.Answer{}, fmt.Errorf("decisions API call failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return provenance.Answer{}, fmt.Errorf("reading decisions API response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		excerpt := string(data)
		if len(excerpt) > maxErrorBody {
			excerpt = excerpt[:maxErrorBody]
		}
		return provenance.Answer{}, fmt.Errorf("decisions API returned status %d: %s", resp.StatusCode, strings.TrimSpace(excerpt))
	}
	var out decisionsResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return provenance.Answer{}, fmt.Errorf("decoding decisions API response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return provenance.Answer{}, fmt.Errorf("decisions API error: %s", out.Error.Message)
	}
	chosen := firstNonEmpty(out.Option, out.Choice)
	key, ok := matchOption(chosen, q.Options)
	if !ok {
		return provenance.Answer{}, fmt.Errorf("the model chose %q, which is not one of the offered options", chosen)
	}
	return provenance.Answer{
		Option:     key,
		Rationale:  strings.TrimSpace(out.Rationale),
		Confidence: clamp01(out.Confidence),
		Model:      firstNonEmpty(out.Model, d.model),
	}, nil
}
