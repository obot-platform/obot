package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	// crowdStrikePollInterval is how often an asynchronous (202 Accepted) result is polled.
	crowdStrikePollInterval = time.Second
	// crowdStrikeMaxPolls bounds polling; the caller's context deadline usually ends it first.
	crowdStrikeMaxPolls = 30
)

var (
	// crowdStrikeRedactingDetectors are the detectors whose matches AI Guard can redact in
	// place. Replacements are used only when every detection comes from one of them.
	crowdStrikeRedactingDetectors = []string{"confidential_and_pii_entity", "secret_and_key_entity", "custom_entity", "competitors", "malicious_entity"}
)

// crowdStrikeChecker calls CrowdStrike Falcon AIDR's AI Guard API (formerly Pangea AI Guard).
// https://aidr-docs.crowdstrike.com/docs/api/aidr
//
// Verified against the API reference and the official SDKs (github.com/crowdstrike/aidr-go):
// POST {baseURL}/v1/guard_chat_completions with a Bearer collector token and a body of
// {"guard_input": {"messages": [...]}, "event_type": "input"|"output"|"tool_input"|"tool_output",
// "app_id", "collector_instance_id"}, answered with {"status", "request_id", "result":
// {"blocked", "transformed", "guard_output", "detectors": {name: {"detected", "data"}}}}.
//
// Assumptions: guard_output has guard_input's shape, with messages in request order, which
// is how LiteLLM's integration reads it; a 202 Accepted response (status "Accepted", as
// in Pangea's API conventions) is polled at
// {baseURL}/request/{request_id}, the SDKs' GetAsyncRequest path. The policy that applies
// is chosen by the collector the token belongs to; there is no per-request recipe.
type crowdStrikeChecker struct {
	client              *http.Client
	baseURL             string
	token               string
	appID               string
	collectorInstanceID string
}

type crowdStrikeRequest struct {
	GuardInput          crowdStrikeInput `json:"guard_input"`
	EventType           string           `json:"event_type"`
	AppID               string           `json:"app_id,omitempty"`
	CollectorInstanceID string           `json:"collector_instance_id,omitempty"`
}

type crowdStrikeInput struct {
	Messages []crowdStrikeMessage `json:"messages"`
}

type crowdStrikeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type crowdStrikeResponse struct {
	Status    string `json:"status"`
	RequestID string `json:"request_id"`
	Result    *struct {
		Blocked     bool `json:"blocked"`
		Transformed bool `json:"transformed"`
		GuardOutput *struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		} `json:"guard_output"`
		Detectors map[string]json.RawMessage `json:"detectors"`
	} `json:"result"`
}

type crowdStrikeDetector struct {
	Detected bool `json:"detected"`
	Data     *struct {
		Entities []json.RawMessage `json:"entities"`
	} `json:"data"`
}

func init() {
	Register(ProviderInfo{
		Type:        "crowdstrike-aidr",
		Label:       "CrowdStrike Falcon AIDR",
		Category:    "Integrations",
		Description: "Screens prompts, responses and tool traffic with CrowdStrike Falcon AI Detection and Response (AI Guard) for malicious prompts, PII, secrets, malicious entities, topics and more. Content AI Guard redacts is replaced with its redacted text.",
		Redacts:     true,
		Fields: []Field{
			{Name: "apiToken", Label: "Collector API token", Type: FieldPassword, Required: true, Sensitive: true, Description: "Token of an AIDR collector; the collector's policy decides what is detected."},
			{Name: "baseURL", Label: "Base URL", Type: FieldText, Destination: true, Default: "https://api.crowdstrike.com/aidr/aiguard", Description: "Use https://api.eu-1.crowdstrike.com/aidr/aiguard for the EU cloud."},
			{Name: "appID", Label: "App ID", Type: FieldText, Description: "Optional application identifier recorded with each event."},
			{Name: "collectorInstanceID", Label: "Collector instance ID", Type: FieldText, Description: "Optional identifier of this Obot deployment, recorded with each event."},
		},
	}, func(config map[string]any, secrets map[string]string, deps Deps) (Checker, error) {
		return &crowdStrikeChecker{
			client:              providerHTTPClient(deps),
			baseURL:             strings.TrimRight(ConfigString(config, "baseURL", "https://api.crowdstrike.com/aidr/aiguard"), "/"),
			token:               secrets["apiToken"],
			appID:               ConfigString(config, "appID", ""),
			collectorInstanceID: ConfigString(config, "collectorInstanceID", ""),
		}, nil
	})
}

func (c *crowdStrikeChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	indexes := checkableSegments(in.Segments)
	if len(indexes) == 0 {
		return Result{}, nil
	}

	body := crowdStrikeRequest{
		EventType:           crowdStrikeEventType(in.Target, in.Stage),
		AppID:               c.appID,
		CollectorInstanceID: c.collectorInstanceID,
	}
	for _, i := range indexes {
		body.GuardInput.Messages = append(body.GuardInput.Messages, crowdStrikeMessage{Role: crowdStrikeRole(in.Segments[i].Role), Content: in.Segments[i].Text})
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/guard_chat_completions", bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return Result{}, err
	}
	// Long-running requests are answered 202 Accepted with status "Accepted" and a request ID.
	for polls := 0; strings.EqualFold(resp.Status, "accepted") && resp.RequestID != ""; polls++ {
		if polls >= crowdStrikeMaxPolls {
			return Result{}, fmt.Errorf("CrowdStrike AIDR did not return a result for request %s in time", resp.RequestID)
		}
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(crowdStrikePollInterval):
		}
		poll, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/request/"+url.PathEscape(resp.RequestID), nil)
		if err != nil {
			return Result{}, err
		}
		if resp, err = c.do(poll); err != nil {
			return Result{}, err
		}
	}
	if resp.Result == nil {
		return Result{}, fmt.Errorf("CrowdStrike AIDR returned no result (status %q)", resp.Status)
	}
	return crowdStrikeResult(in.Segments, indexes, resp), nil
}

func (c *crowdStrikeChecker) do(req *http.Request) (crowdStrikeResponse, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	var resp crowdStrikeResponse
	err := providerDo(c.client, req, "CrowdStrike AIDR", &resp)
	return resp, err
}

func crowdStrikeResult(segments []Segment, indexes []int, resp crowdStrikeResponse) Result {
	r := resp.Result
	names := make([]string, 0, len(r.Detectors))
	for name := range r.Detectors {
		names = append(names, name)
	}
	slices.Sort(names)

	var (
		fs = newMergedFindings()
		// redactable reports whether every detection is one AI Guard redacts in place.
		redactable = !r.Blocked
	)
	for _, name := range names {
		var d crowdStrikeDetector
		if err := json.Unmarshal(r.Detectors[name], &d); err != nil || !d.Detected {
			continue
		}
		count := 1
		if d.Data != nil && len(d.Data.Entities) > 0 {
			count = len(d.Data.Entities)
		}
		redactable = redactable && slices.Contains(crowdStrikeRedactingDetectors, name)
		fs.add(Finding{Category: name, Message: strings.ReplaceAll(name, "_", " ") + " detected", Count: count})
	}
	result := Result{Findings: fs.list()}
	if len(result.Findings) == 0 && r.Blocked {
		result.Findings = []Finding{{Category: "blocked", Message: "Blocked by CrowdStrike AIDR policy"}}
	}

	if !redactable || !r.Transformed || r.GuardOutput == nil || len(r.GuardOutput.Messages) != len(indexes) {
		return result
	}
	for n, m := range r.GuardOutput.Messages {
		text, ok := crowdStrikeContent(m.Content)
		seg := indexes[n]
		if !ok || text == segments[seg].Text {
			continue
		}
		if result.Replacements == nil {
			result.Replacements = map[int]string{}
		}
		result.Replacements[seg] = text
	}
	if len(result.Replacements) > 0 && len(result.Findings) == 0 {
		result.Findings = []Finding{{Category: "transformed", Message: "Redacted by CrowdStrike AIDR policy"}}
	}
	return result
}

// crowdStrikeContent reads a message's content, either a string or an array of text parts.
func crowdStrikeContent(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", false
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "" || p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String(), true
}

// crowdStrikeEventType maps a target and stage to an AI Guard event type: MCP tool arguments
// and results are tool input and output.
func crowdStrikeEventType(target, stage string) string {
	switch {
	case target == gatewaytypes.GuardrailTargetMCP && stage == StageOutput:
		return "tool_output"
	case target == gatewaytypes.GuardrailTargetMCP:
		return "tool_input"
	case stage == StageOutput:
		return "output"
	default:
		return "input"
	}
}

func crowdStrikeRole(role string) string {
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
