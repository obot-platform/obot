package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	// lakeraDefaultLevel is the least confident result that counts without a project. Check
	// Point's default policy flags results down to l4_less_likely, which catches ordinary prompts
	// such as "Write a fictional name and email address for a test account".
	lakeraDefaultLevel = "l2_very_likely"
)

var (
	// lakeraLevels are Lakera's result levels, most confident first.
	lakeraLevels = []string{"l1_confident", "l2_very_likely", "l3_likely", "l4_less_likely"}
)

// lakeraChecker calls the Lakera Guard v2 guard API (Lakera is now part of Check Point).
// https://docs.lakera.ai/docs/api/guard
type lakeraChecker struct {
	client    *http.Client
	baseURL   string
	projectID string
	apiKey    string
	// minLevel is the least confident result level that counts, from 1 (l1_confident) to
	// 5 (l5_unlikely). It applies only without a project; a project's policy decides itself.
	minLevel int
}

type lakeraRequest struct {
	Messages  []lakeraMessage `json:"messages"`
	ProjectID string          `json:"project_id,omitempty"`
	Breakdown bool            `json:"breakdown"`
	Payload   bool            `json:"payload"`
}

type lakeraMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type lakeraResponse struct {
	Flagged   bool `json:"flagged"`
	Breakdown []struct {
		DetectorType string `json:"detector_type"`
		Detected     bool   `json:"detected"`
		Result       string `json:"result"`
		MessageID    *int   `json:"message_id"`
	} `json:"breakdown"`
	Payload []struct {
		Start        int      `json:"start"`
		End          int      `json:"end"`
		Text         string   `json:"text"`
		DetectorType string   `json:"detector_type"`
		Labels       []string `json:"labels"`
		MessageID    *int     `json:"message_id"`
	} `json:"payload"`
}

func init() {
	Register(ProviderInfo{
		Type:        "lakera",
		Label:       "Lakera Guard (Check Point)",
		Category:    "Integrations",
		Description: "Screens conversations with Lakera Guard, now part of Check Point, for prompt attacks, PII, content violations, unknown links and custom detectors. PII, profanity and custom regex matches are redacted.",
		Redacts:     true,
		Fields: []Field{
			{Name: "apiKey", Label: "API key", Type: FieldPassword, Required: true, Sensitive: true},
			{Name: "projectID", Label: "Project ID", Type: FieldText, Description: "The Lakera project whose policy screens the content, deciding which detectors run and how confident they must be. Leave empty to use the default policy with the minimum confidence below."},
			{Name: "minConfidence", Label: "Minimum confidence", Type: FieldSelect, Options: lakeraLevels, Default: lakeraDefaultLevel, Description: "Without a project, the least confident result that counts. Lakera's default policy flags results down to l4_less_likely, which includes some ordinary prompts. Ignored when a project is set."},
			{Name: "baseURL", Label: "Base URL", Type: FieldText, Destination: true, Default: "https://api.lakera.ai", Description: "Change for a regional or self-hosted deployment."},
		},
	}, func(config map[string]any, secrets map[string]string, deps Deps) (Checker, error) {
		minLevel := lakeraLevel(ConfigString(config, "minConfidence", lakeraDefaultLevel))
		if minLevel == 0 {
			return nil, fmt.Errorf("minimum confidence must be one of %s", strings.Join(lakeraLevels, ", "))
		}
		return &lakeraChecker{
			client:    providerHTTPClient(deps),
			baseURL:   strings.TrimRight(ConfigString(config, "baseURL", "https://api.lakera.ai"), "/"),
			projectID: ConfigString(config, "projectID", ""),
			apiKey:    secrets["apiKey"],
			minLevel:  minLevel,
		}, nil
	})
}

func (c *lakeraChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	indexes := checkableSegments(in.Segments)
	if len(indexes) == 0 {
		return Result{}, nil
	}

	body := lakeraRequest{ProjectID: c.projectID, Breakdown: true, Payload: true}
	for _, i := range indexes {
		body.Messages = append(body.Messages, lakeraMessage{Role: lakeraRole(in.Segments[i].Role), Content: in.Segments[i].Text})
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v2/guard", bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	var resp lakeraResponse
	if err := providerDo(c.client, req, "Lakera Guard", &resp); err != nil {
		return Result{}, err
	}
	if c.projectID == "" {
		resp = filterLakeraConfidence(resp, c.minLevel)
	}
	return lakeraResult(in.Segments, indexes, resp), nil
}

// lakeraLevel is a result level's rank, from 1 for l1_confident to 5 for l5_unlikely, or 0
// when it isn't one.
func lakeraLevel(result string) int {
	if len(result) < 2 || result[0] != 'l' || result[1] < '1' || result[1] > '5' {
		return 0
	}
	return int(result[1] - '0')
}

// filterLakeraConfidence drops detections less confident than minLevel, with the matches
// they located. It replaces Lakera's overall verdict, which follows the default policy,
// with one that follows the remaining detections.
func filterLakeraConfidence(resp lakeraResponse, minLevel int) lakeraResponse {
	if len(resp.Breakdown) == 0 {
		// Nothing to judge confidence by; keep Lakera's verdict.
		return resp
	}
	// Detections are per detector, and per message when they name one.
	type key struct {
		detector string
		message  int
	}
	var (
		keptMessage = map[key]bool{}
		// keptAll holds detectors confident without naming a message, which covers them all.
		keptAll  = map[string]bool{}
		keptAny  = map[string]bool{}
		reported = map[string]bool{}
	)
	breakdown := resp.Breakdown[:0:0]
	for _, b := range resp.Breakdown {
		reported[b.DetectorType] = true
		level := lakeraLevel(b.Result)
		if b.Detected && (level == 0 || level <= minLevel) {
			keptAny[b.DetectorType] = true
			if b.MessageID == nil {
				keptAll[b.DetectorType] = true
			} else {
				keptMessage[key{b.DetectorType, *b.MessageID}] = true
			}
		} else {
			b.Detected = false
		}
		breakdown = append(breakdown, b)
	}
	payload := resp.Payload[:0:0]
	for _, p := range resp.Payload {
		dt := p.DetectorType
		switch {
		case !reported[dt], keptAll[dt]:
			// A match from a detector the breakdown doesn't mention is kept, to be safe.
		case p.MessageID == nil && keptAny[dt]:
		case p.MessageID != nil && keptMessage[key{dt, *p.MessageID}]:
		default:
			continue
		}
		payload = append(payload, p)
	}
	resp.Breakdown, resp.Payload, resp.Flagged = breakdown, payload, len(keptAny) > 0
	return resp
}

// lakeraResult converts a guard response. Payload items locate matches by message_id, the
// index into the request's messages, and start and end, which are code point offsets into
// that message's content (Lakera's own integrations index Python strings with them).
func lakeraResult(segments []Segment, indexes []int, resp lakeraResponse) Result {
	fs := newMergedFindings()
	for _, b := range resp.Breakdown {
		if b.Detected {
			fs.add(Finding{Category: lakeraCategory(b.DetectorType), Message: lakeraMessageFor(b.DetectorType), Count: 1})
		}
	}

	// Spans attach to the finding for their detector, replacing the breakdown's single count.
	spans := map[string][]Span{}
	var order []string
	for _, p := range resp.Payload {
		label := lakeraLabel(p.DetectorType)
		var candidates []int
		switch {
		case p.MessageID != nil && *p.MessageID >= 0 && *p.MessageID < len(indexes):
			candidates = []int{indexes[*p.MessageID]}
		case p.MessageID == nil:
			// Without a message ID, find the message that holds the match.
			candidates = indexes
		}
		for _, seg := range candidates {
			if span, ok := codepointSpan(segments, seg, p.Start, p.End, p.Text, label); ok {
				if _, seen := spans[p.DetectorType]; !seen {
					order = append(order, p.DetectorType)
				}
				spans[p.DetectorType] = append(spans[p.DetectorType], span)
				break
			}
		}
	}
	findings := fs.list()
	for _, dt := range order {
		category, message := lakeraCategory(dt), lakeraMessageFor(dt)
		found := false
		for i := range findings {
			if findings[i].Category == category && findings[i].Message == message {
				findings[i].Spans = spans[dt]
				findings[i].Count = len(spans[dt])
				found = true
			}
		}
		if !found {
			findings = append(findings, Finding{Category: category, Message: message, Spans: spans[dt], Count: len(spans[dt])})
		}
	}

	if len(findings) == 0 && resp.Flagged {
		findings = []Finding{{Category: "flagged", Message: "Flagged by Lakera Guard"}}
	}
	return Result{Findings: findings}
}

// lakeraRole maps a segment role to a chat message role. Tool calls are the assistant's
// words; tool results use the tool role, which Lakera screens as untrusted content. System
// messages are sent as system messages, which Lakera treats as trusted and doesn't screen.
func lakeraRole(role string) string {
	switch role {
	case RoleSystem:
		return "system"
	case RoleAssistant, RoleToolCall:
		return "assistant"
	case RoleToolResult:
		return "tool"
	default:
		return "user"
	}
}

// lakeraCategory is the detector family, such as "prompt_attack" or "pii".
func lakeraCategory(detectorType string) string {
	if detectorType == "" {
		return "lakera"
	}
	category, _, _ := strings.Cut(detectorType, "/")
	return category
}

func lakeraMessageFor(detectorType string) string {
	if detectorType == "" {
		return "Flagged by Lakera Guard"
	}
	return detectorType + " detected"
}

// lakeraLabel is the redaction label for a detector, such as EMAIL for "pii/email".
func lakeraLabel(detectorType string) string {
	if i := strings.LastIndex(detectorType, "/"); i >= 0 {
		detectorType = detectorType[i+1:]
	}
	if detectorType == "" {
		return "SENSITIVE"
	}
	return strings.ToUpper(detectorType)
}
