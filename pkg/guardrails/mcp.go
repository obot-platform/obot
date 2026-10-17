package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// MCP methods whose content guardrail filters check.
	MCPMethodToolsCall     = "tools/call"
	MCPMethodResourcesRead = "resources/read"
	MCPMethodPromptsGet    = "prompts/get"
)

// MCPFilter is a guardrail filter: an MCP filter that Obot evaluates in-process with
// guardrails instead of calling a filter MCP server.
type MCPFilter struct {
	// ID and Name identify the filter; Name is used in block messages.
	ID     string
	Name   string
	Config types2.MCPGuardrailFilterConfig
}

// MCPMessage is an MCP request or response that a guardrail filter checks.
type MCPMessage struct {
	Identity Identity
	// Server is the MCP server's display name, and ServerID its ID.
	Server   string
	ServerID string
	Headers  map[string]string
	// Method is the request's method, and Name its tool or prompt name or resource URI.
	Method string
	Name   string
	// Stage is StageInput for a request and StageOutput for its response.
	Stage string
	// Params are the request's params, at both stages.
	Params json.RawMessage
	// Result is the response's result, at the output stage.
	Result json.RawMessage
}

// MCPDecision is the outcome of a guardrail filter.
type MCPDecision struct {
	// Applied reports whether the filter checked the message: it covers the message's
	// direction and caller, its condition held, and the message was sampled.
	Applied bool
	Blocked bool
	Message string
	// Params and Result are the redacted request params or response result; nil when
	// unchanged.
	Params json.RawMessage
	Result json.RawMessage
	// Errors describe guardrails that failed without blocking, because the filter fails open.
	Errors []string
}

// jsonText is one string in a JSON document, with a way to replace it.
type jsonText struct {
	get func() string
	set func(string)
}

// ResolveIdentity resolves whose budgets and guardrails apply to a request, and whether the
// caller is an admin. The caller's role is read from its obot_groups, not only its groups: a
// request made with an API key carries only the groups of the key's scopes, so an LLM-only
// key of an admin has no admin group.
func ResolveIdentity(_ context.Context, _ *gateway.Client, user kuser.Info) (Identity, bool, error) {
	userID := user.GetUID()
	groups := user.GetExtra()["auth_provider_groups"]
	isAdmin := false
	for _, g := range [][]string{user.GetGroups(), user.GetExtra()["obot_groups"]} {
		if slices.Contains(g, types2.GroupAdmin) || slices.Contains(g, types2.GroupOwner) {
			isAdmin = true
		}
	}
	return Identity{UserID: userID, Groups: groups}, isAdmin, nil
}

// filterPolicy converts a guardrail filter into a policy. A guardrail that doesn't exist fails
// like one that can't be evaluated, so a fail-closed filter blocks.
func filterPolicy(f MCPFilter, guardrails map[string]*compiledGuardrail) *compiledPolicy {
	c := f.Config
	applyTo := types.GuardrailApplyBoth
	switch c.Direction {
	case types2.MCPGuardrailDirectionRequest:
		applyTo = types.GuardrailApplyInput
	case types2.MCPGuardrailDirectionResponse:
		applyTo = types.GuardrailApplyOutput
	}
	action := c.Action
	if action != types2.MCPGuardrailActionRedact {
		// Filters only block or redact. Anything else, such as a log-only action saved before
		// it was removed, blocks rather than letting findings through unnoticed.
		action = types2.MCPGuardrailActionBlock
	}
	subjects := make([]types.GuardrailPolicySubject, 0, len(c.Subjects))
	for _, s := range c.Subjects {
		subjects = append(subjects, types.GuardrailPolicySubject{Type: s.Type, ID: s.ID})
	}
	r := &compiledPolicy{
		policy: types.GuardrailPolicy{
			ID:           f.ID,
			Name:         f.Name,
			Enabled:      true,
			Target:       types.GuardrailTargetMCP,
			ApplyTo:      applyTo,
			Action:       action,
			Condition:    c.Condition,
			SamplingRate: c.SamplingRate,
			FailClosed:   c.FailClosed,
			BlockMessage: c.BlockMessage,
			GuardrailIDs: c.GuardrailIDs,
			Subjects:     subjects,
		},
		allRedact: true,
	}
	for _, id := range c.GuardrailIDs {
		p, ok := guardrails[id]
		if !ok {
			p = &compiledGuardrail{guardrail: types.Guardrail{ID: id, Name: id}, err: fmt.Errorf("guardrail %s does not exist", id)}
		}
		r.guardrails = append(r.guardrails, p)
		r.allRedact = r.allRedact && p.redacts
	}
	return r
}

// mcpMessageHasContent reports whether a guardrail filter has anything to check in msg:
// tool call arguments and results, prompt arguments and messages, and resource contents
// (resources/read requests carry only a URI).
func mcpMessageHasContent(msg MCPMessage) bool {
	switch msg.Method {
	case MCPMethodToolsCall, MCPMethodPromptsGet:
		return true
	case MCPMethodResourcesRead:
		return msg.Stage == StageOutput
	}
	return false
}

// CheckMCPFilter runs a guardrail filter on an MCP request or response. Tool call arguments
// and results, resource contents, and prompt arguments and messages are checked; other
// methods carry no content to check and pass unchanged.
func (e *Engine) CheckMCPFilter(ctx context.Context, f MCPFilter, msg MCPMessage) MCPDecision {
	if !mcpMessageHasContent(msg) {
		// Nothing to check, so the filter doesn't apply: it neither runs, records, nor (when
		// its condition can't be evaluated for this message) blocks.
		return MCPDecision{}
	}
	_, guardrails, err := e.loadAll(ctx)
	if err != nil {
		// Unlike a guardrail failing (which the filter's fail-closed setting decides), this means
		// no guardrail can run at all. Block, as any filter that errors does and as model
		// guardrails do when their policies can't be loaded.
		slog.Error("failed to load guardrails; blocking", "filter", f.Name, "error", err)
		return MCPDecision{Applied: true, Blocked: true, Message: fmt.Sprintf("Blocked: the %q guardrail filter could not be evaluated.", f.Name)}
	}

	var params map[string]any
	if len(msg.Params) > 0 {
		if err := decodeJSON(msg.Params, &params); err != nil {
			// The server might still accept what can't be checked here, so don't forward it.
			return MCPDecision{Applied: true, Blocked: true, Message: fmt.Sprintf("Blocked: the %q guardrail filter could not read the request's params.", f.Name)}
		}
	}
	arguments, _ := params["arguments"].(map[string]any)
	if arguments == nil {
		arguments = map[string]any{}
	}
	headers := msg.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	vars := map[string]any{
		"mcp_server": msg.Server, "mcp_method": msg.Method, "mcp_tool": msg.Name,
		"mcp_arguments": arguments, "headers": headers,
	}

	var plan Plan
	e.addToPlan(&plan, filterPolicy(f, guardrails), types.GuardrailTargetMCP, msg.Stage, msg.Identity, vars)
	if plan.Empty() {
		return MCPDecision{}
	}

	var (
		doc     any
		texts   []jsonText
		role    = RoleToolCall
		history []Segment
	)
	switch msg.Stage {
	case StageInput:
		doc = params
		if params != nil && (msg.Method == MCPMethodToolsCall || msg.Method == MCPMethodPromptsGet) {
			texts = fieldTexts(params, "arguments")
		}
	case StageOutput:
		role = RoleToolResult
		if len(msg.Result) > 0 {
			if err := decodeJSON(msg.Result, &doc); err != nil {
				return MCPDecision{Applied: true, Blocked: true, Message: fmt.Sprintf("Blocked: the %q guardrail filter could not read the response.", f.Name)}
			}
		}
		switch msg.Method {
		case MCPMethodToolsCall:
			texts = toolResultTexts(&doc)
			if raw, ok := params["arguments"]; ok {
				if args, err := json.Marshal(raw); err == nil {
					history = []Segment{{Role: RoleToolCall, Name: msg.Name, Text: string(args)}}
				}
			}
		case MCPMethodResourcesRead:
			texts = resourceContentsTexts(doc)
		case MCPMethodPromptsGet:
			texts = promptMessagesTexts(doc)
		}
	}

	segments := make([]Segment, len(texts))
	for i, t := range texts {
		segments[i] = Segment{Role: role, Name: msg.Name, Text: t.get()}
	}
	d := e.Evaluate(ctx, plan, Request{
		Target: types.GuardrailTargetMCP, Stage: msg.Stage, Identity: msg.Identity, Vars: vars,
		Segments: segments, History: history,
	})
	out := MCPDecision{Applied: true, Blocked: d.Blocked, Message: d.Message}
	if !d.Blocked {
		// A fail-open filter lets the message through when a guardrail fails; say so, so the
		// failure shows in the MCP audit log.
		for _, det := range d.Detections {
			if det.Outcome == "error" {
				out.Errors = append(out.Errors, fmt.Sprintf("guardrail %q failed: %s; allowed because the filter fails open", det.GuardrailName, det.Error))
			}
		}
	}
	if d.Blocked || !d.Redacted {
		return out
	}
	for i, t := range texts {
		if i < len(d.Segments) {
			t.set(d.Segments[i].Text)
		}
	}
	changed, err := json.Marshal(doc)
	if err != nil {
		return MCPDecision{Applied: true, Blocked: true, Message: fmt.Sprintf("Blocked: the %q guardrail filter could not encode the redacted message.", f.Name)}
	}
	if msg.Stage == StageInput {
		out.Params = changed
	} else {
		out.Result = changed
	}
	return out
}

func decodeJSON(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(v)
}

// collectJSONText returns every string value (not key) in a JSON document, depth first.
func collectJSONText(v *any) []jsonText {
	return collectTexts(func() any { return *v }, func(n any) { *v = n })
}

// fieldTexts returns every string value in a field of a JSON object.
func fieldTexts(m map[string]any, key string) []jsonText {
	if _, ok := m[key]; !ok {
		return nil
	}
	return collectTexts(func() any { return m[key] }, func(n any) { m[key] = n })
}

func collectTexts(get func() any, set func(any)) []jsonText {
	var out []jsonText
	var walk func(get func() any, set func(any))
	walk = func(get func() any, set func(any)) {
		switch val := get().(type) {
		case string:
			if val != "" {
				out = append(out, jsonText{get: func() string { return get().(string) }, set: func(s string) { set(s) }})
			}
		case map[string]any:
			keys := make([]string, 0, len(val))
			for k := range val {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				walk(func() any { return val[k] }, func(n any) { val[k] = n })
			}
		case []any:
			for i := range val {
				walk(func() any { return val[i] }, func(n any) { val[i] = n })
			}
		}
	}
	walk(get, set)
	return out
}

// toolResultTexts returns the text of an MCP CallToolResult: its text content blocks, the
// text of embedded resources, and the strings in its structured content. Other shapes are
// checked string by string.
func toolResultTexts(v *any) []jsonText {
	result, ok := (*v).(map[string]any)
	if !ok {
		return collectJSONText(v)
	}
	var out []jsonText
	if content, ok := result["content"].([]any); ok {
		for _, item := range content {
			if block, ok := item.(map[string]any); ok {
				out = append(out, contentBlockTexts(block)...)
			}
		}
	}
	return append(out, fieldTexts(result, "structuredContent")...)
}

// resourceContentsTexts returns the text of an MCP ReadResourceResult's text contents.
// Binary (blob) contents are not checked.
func resourceContentsTexts(v any) []jsonText {
	result, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	contents, _ := result["contents"].([]any)
	var out []jsonText
	for _, item := range contents {
		if content, ok := item.(map[string]any); ok {
			out = append(out, stringField(content, "text")...)
		}
	}
	return out
}

// promptMessagesTexts returns the text of an MCP GetPromptResult's messages.
func promptMessagesTexts(v any) []jsonText {
	result, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	messages, _ := result["messages"].([]any)
	var out []jsonText
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch content := message["content"].(type) {
		case map[string]any:
			out = append(out, contentBlockTexts(content)...)
		case []any:
			for _, c := range content {
				if block, ok := c.(map[string]any); ok {
					out = append(out, contentBlockTexts(block)...)
				}
			}
		}
	}
	return out
}

// contentBlockTexts returns the text of an MCP content block: a text block, or an embedded
// text resource.
func contentBlockTexts(block map[string]any) []jsonText {
	switch block["type"] {
	case "text":
		return stringField(block, "text")
	case "resource":
		if res, ok := block["resource"].(map[string]any); ok {
			return stringField(res, "text")
		}
	}
	return nil
}

func stringField(m map[string]any, key string) []jsonText {
	if s, ok := m[key].(string); !ok || s == "" {
		return nil
	}
	return []jsonText{{get: func() string { return m[key].(string) }, set: func(s string) { m[key] = s }}}
}
