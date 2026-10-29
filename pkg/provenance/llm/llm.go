// Package llm adapts LLM gateway requests and responses to the provenance
// session graph: which tool results a request feeds back into the model's
// context, which tool calls the model proposes in its response, who the agent
// is, and how to rewrite untrusted content when a session is quarantined.
//
// The helpers are pure: they read and rewrite the gateway's parsed request and
// response shapes and never talk to the engine. The guardrail plugins in
// pkg/guardrails call them and feed the engine.
package llm

import (
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/maximhq/bifrost/core/schemas"
)

const (
	// QuarantinePrefix opens a quarantined tool result.
	QuarantinePrefix = "[quarantined untrusted content: "
	quarantineLimit  = 200
)

var (
	// destinationKeys are argument names that commonly carry where a tool sends
	// data, checked in order.
	destinationKeys = []string{"url", "uri", "endpoint", "webhook", "host", "hostname", "to", "recipient", "recipients", "email", "address", "channel"}
	// quarantineWords are words whose lines are dropped from a quarantined
	// summary: the vocabulary of prompt injection.
	quarantineWords = []string{"ignore", "instruction", "system", "assistant"}
)

// ToolDefinition is a tool a request offers the model.
type ToolDefinition struct {
	Name        string
	Description string
}

// ToolResult is one tool result in a request's conversation.
type ToolResult struct {
	// CallID is the tool call the result answers.
	CallID string
	// Name is the tool that produced the result, found through the tool call
	// with the same CallID earlier in the conversation. It is empty when the
	// conversation does not carry that call.
	Name string
	// Text is the result's text content, blocks joined with newlines.
	Text string
}

// ProposedCall is a tool call the model proposes in a response.
type ProposedCall struct {
	CallID        string
	Name          string
	ArgumentsJSON string
}

// Turn is what one model request tells the session graph.
type Turn struct {
	// Model is the requested model, named "provider/model" when the provider
	// is known.
	Model    string
	Provider string
	// UserMessage is the text of the latest user message.
	UserMessage string
	// NewResults are the tool results that follow the last assistant message:
	// the results of the tool calls the model proposed in its previous turn,
	// which this request feeds back to it. Earlier results were fed back by
	// earlier requests.
	NewResults []ToolResult
	// Tools are the tool definitions the request declares.
	Tools []ToolDefinition
}

// resultRef is a tool result in a request with its text fields, so the
// result can be rewritten in place.
type resultRef struct {
	result ToolResult
	// fresh reports that the result follows the last assistant message.
	fresh bool
	texts []*string
}

// ToolDescription returns the declared description of a tool, or "".
func (t Turn) ToolDescription(name string) string {
	for _, tool := range t.Tools {
		if tool.Name == name {
			return tool.Description
		}
	}
	return ""
}

// ExtractTurn reads what a request tells the session graph. Only Chat
// Completions and Responses requests carry a conversation; Anthropic Messages
// requests arrive as Responses requests.
func ExtractTurn(req *schemas.BifrostRequest) Turn {
	var turn Turn
	if req == nil {
		return turn
	}
	provider, model, _ := req.GetRequestFields()
	turn.Model, turn.Provider = model, string(provider)
	if provider != "" {
		turn.Model = string(provider) + "/" + model
	}
	turn.Tools = toolDefinitions(req)
	refs, user := walkRequest(req)
	turn.UserMessage = user
	for _, ref := range refs {
		if ref.fresh {
			turn.NewResults = append(turn.NewResults, ref.result)
		}
	}
	return turn
}

// DeclaresTools reports whether a request offers the model any tools, so its
// response can propose tool calls.
func DeclaresTools(req *schemas.BifrostRequest) bool {
	if req == nil {
		return false
	}
	switch {
	case req.ChatRequest != nil:
		return req.ChatRequest.Params != nil && len(req.ChatRequest.Params.Tools) > 0
	case req.ResponsesRequest != nil:
		return req.ResponsesRequest.Params != nil && len(req.ResponsesRequest.Params.Tools) > 0
	}
	return false
}

func toolDefinitions(req *schemas.BifrostRequest) []ToolDefinition {
	var out []ToolDefinition
	switch {
	case req.ChatRequest != nil && req.ChatRequest.Params != nil:
		for _, t := range req.ChatRequest.Params.Tools {
			def := ToolDefinition{Name: string(t.Type)}
			if t.Function != nil {
				def.Name = t.Function.Name
				def.Description = deref(t.Function.Description)
			}
			out = append(out, def)
		}
	case req.ResponsesRequest != nil && req.ResponsesRequest.Params != nil:
		for _, t := range req.ResponsesRequest.Params.Tools {
			def := ToolDefinition{Name: deref(t.Name), Description: deref(t.Description)}
			if def.Name == "" {
				def.Name = string(t.Type)
			}
			out = append(out, def)
		}
	}
	return out
}

func walkRequest(req *schemas.BifrostRequest) ([]resultRef, string) {
	switch {
	case req.ChatRequest != nil:
		return walkChat(req.ChatRequest)
	case req.ResponsesRequest != nil:
		return walkResponses(req.ResponsesRequest)
	}
	return nil, ""
}

func walkChat(r *schemas.BifrostChatRequest) ([]resultRef, string) {
	names := map[string]string{}
	last := -1
	for i := range r.Input {
		m := &r.Input[i]
		if m.Role == schemas.ChatMessageRoleAssistant {
			last = i
		}
		if m.ChatAssistantMessage == nil {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.ID != nil && tc.Function.Name != nil {
				names[*tc.ID] = *tc.Function.Name
			}
		}
	}
	var (
		refs []resultRef
		user string
	)
	for i := range r.Input {
		m := &r.Input[i]
		switch m.Role {
		case schemas.ChatMessageRoleUser:
			if text := joinTexts(chatTexts(m.Content)); text != "" {
				user = text
			}
		case schemas.ChatMessageRoleTool:
			ref := resultRef{fresh: i > last, texts: chatTexts(m.Content)}
			if m.ChatToolMessage != nil && m.ToolCallID != nil {
				ref.result.CallID = *m.ToolCallID
			}
			ref.result.Name = names[ref.result.CallID]
			ref.result.Text = joinTexts(ref.texts)
			refs = append(refs, ref)
		}
	}
	return refs, user
}

func walkResponses(r *schemas.BifrostResponsesRequest) ([]resultRef, string) {
	names := map[string]string{}
	last := -1
	for i := range r.Input {
		m := &r.Input[i]
		if m.Role != nil && *m.Role == schemas.ResponsesInputMessageRoleAssistant {
			last = i
		}
		if m.Type == nil || m.ResponsesToolMessage == nil {
			continue
		}
		switch *m.Type {
		case schemas.ResponsesMessageTypeFunctionCall, schemas.ResponsesMessageTypeCustomToolCall:
			last = i
			if m.CallID != nil && m.Name != nil {
				names[*m.CallID] = *m.Name
			}
		}
	}
	var (
		refs []resultRef
		user string
	)
	for i := range r.Input {
		m := &r.Input[i]
		if m.Type != nil && m.ResponsesToolMessage != nil {
			switch *m.Type {
			case schemas.ResponsesMessageTypeFunctionCallOutput, schemas.ResponsesMessageTypeCustomToolCallOutput:
				ref := resultRef{fresh: i > last}
				if m.CallID != nil {
					ref.result.CallID = *m.CallID
				}
				ref.result.Name = names[ref.result.CallID]
				if m.Output != nil {
					ref.texts = appendText(ref.texts, m.Output.ResponsesToolCallOutputStr)
					for j := range m.Output.ResponsesFunctionToolCallOutputBlocks {
						ref.texts = appendText(ref.texts, m.Output.ResponsesFunctionToolCallOutputBlocks[j].Text)
					}
				}
				ref.result.Text = joinTexts(ref.texts)
				refs = append(refs, ref)
				continue
			}
		}
		if m.Type != nil && *m.Type != schemas.ResponsesMessageTypeMessage {
			continue
		}
		if m.Role != nil && *m.Role != schemas.ResponsesInputMessageRoleUser {
			continue
		}
		if text := joinTexts(responsesTexts(m.Content)); text != "" {
			user = text
		}
	}
	return refs, user
}

func chatTexts(c *schemas.ChatMessageContent) []*string {
	if c == nil {
		return nil
	}
	texts := appendText(nil, c.ContentStr)
	for i := range c.ContentBlocks {
		texts = appendText(texts, c.ContentBlocks[i].Text)
	}
	return texts
}

func responsesTexts(c *schemas.ResponsesMessageContent) []*string {
	if c == nil {
		return nil
	}
	texts := appendText(nil, c.ContentStr)
	for i := range c.ContentBlocks {
		texts = appendText(texts, c.ContentBlocks[i].Text)
	}
	return texts
}

func appendText(texts []*string, s *string) []*string {
	if s == nil || *s == "" {
		return texts
	}
	return append(texts, s)
}

func joinTexts(texts []*string) string {
	parts := make([]string, 0, len(texts))
	for _, t := range texts {
		parts = append(parts, *t)
	}
	return strings.Join(parts, "\n")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ExtractProposedToolCalls returns the tool calls a complete response
// proposes. A streamed response is accumulated by the gateway plugin instead.
// Anthropic tool_use blocks arrive as Responses function_call items.
func ExtractProposedToolCalls(resp *schemas.BifrostResponse) []ProposedCall {
	if resp == nil {
		return nil
	}
	var out []ProposedCall
	switch {
	case resp.ChatResponse != nil:
		for i := range resp.ChatResponse.Choices {
			c := &resp.ChatResponse.Choices[i]
			if c.ChatNonStreamResponseChoice == nil || c.Message == nil || c.Message.ChatAssistantMessage == nil {
				continue
			}
			for _, tc := range c.Message.ToolCalls {
				out = append(out, ProposedCall{CallID: deref(tc.ID), Name: deref(tc.Function.Name), ArgumentsJSON: tc.Function.Arguments})
			}
		}
	case resp.ResponsesResponse != nil:
		out = responsesCalls(resp.ResponsesResponse.Output)
	case resp.ResponsesStreamResponse != nil && resp.ResponsesStreamResponse.Response != nil:
		out = responsesCalls(resp.ResponsesStreamResponse.Response.Output)
	}
	return out
}

func responsesCalls(items []schemas.ResponsesMessage) []ProposedCall {
	var out []ProposedCall
	for i := range items {
		m := &items[i]
		if m.Type == nil || m.ResponsesToolMessage == nil {
			continue
		}
		switch *m.Type {
		case schemas.ResponsesMessageTypeFunctionCall:
			out = append(out, ProposedCall{CallID: deref(m.CallID), Name: deref(m.Name), ArgumentsJSON: deref(m.Arguments)})
		case schemas.ResponsesMessageTypeCustomToolCall:
			call := ProposedCall{CallID: deref(m.CallID), Name: deref(m.Name)}
			if m.ResponsesCustomToolCall != nil {
				call.ArgumentsJSON = m.ResponsesCustomToolCall.Input
			}
			out = append(out, call)
		}
	}
	return out
}

// ParseArguments parses a tool call's arguments for the engine to scan. Text
// that isn't a JSON object, such as a custom tool's input, is kept under
// "input". Empty arguments give nil.
func ParseArguments(argumentsJSON string) map[string]any {
	if strings.TrimSpace(argumentsJSON) == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil || args == nil {
		return map[string]any{"input": argumentsJSON}
	}
	return args
}

// Destination finds where a tool call sends data from its arguments: the host
// of a URL, or the domain of an email address. It returns "" when the
// arguments don't say.
func Destination(args map[string]any) string {
	for _, key := range destinationKeys {
		for k, v := range args {
			if !strings.EqualFold(k, key) {
				continue
			}
			if dest := destinationOf(v); dest != "" {
				return dest
			}
		}
	}
	return ""
}

func destinationOf(v any) string {
	switch v := v.(type) {
	case string:
		return hostOf(v)
	case []any:
		for _, item := range v {
			if dest := destinationOf(item); dest != "" {
				return dest
			}
		}
	}
	return ""
}

func hostOf(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
		return ""
	}
	if at := strings.LastIndex(s, "@"); at >= 0 && at < len(s)-1 {
		domain := s[at+1:]
		if end := strings.IndexAny(domain, " >,;"); end >= 0 {
			domain = domain[:end]
		}
		return strings.ToLower(domain)
	}
	host := s
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	host, _, _ = strings.Cut(host, ":")
	if host == "" || strings.ContainsAny(host, " \t\n") || !strings.Contains(host, ".") {
		return ""
	}
	return strings.ToLower(host)
}

// AgentFromUserAgent names the agent program from a request's User-Agent:
// claude_code, codex or cursor when recognized, otherwise the product token
// before the slash, lowercased. It also returns the version after the slash of
// the first product token, when there is one.
func AgentFromUserAgent(userAgent string) (agent, version string) {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return "", ""
	}
	first := strings.Fields(ua)[0]
	product, ver, _ := strings.Cut(first, "/")
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "claude-cli") || strings.Contains(lower, "claude-code") || strings.Contains(lower, "claude code"):
		agent = "claude_code"
	case strings.Contains(lower, "codex"):
		agent = "codex"
	case strings.Contains(lower, "cursor"):
		agent = "cursor"
	default:
		agent = strings.ToLower(product)
	}
	return agent, ver
}

// QuarantineSummary reduces untrusted content to a schema-constrained summary:
// its first 200 characters, after dropping every line that mentions ignoring,
// instructions, the system or the assistant. Already quarantined text is
// returned as is.
func QuarantineSummary(text string) string {
	if strings.HasPrefix(text, QuarantinePrefix) {
		return text
	}
	var kept []string
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		drop := false
		for _, w := range quarantineWords {
			if strings.Contains(lower, w) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, line)
		}
	}
	summary := strings.Join(kept, " ")
	if utf8.RuneCountInString(summary) > quarantineLimit {
		runes := []rune(summary)
		summary = string(runes[:quarantineLimit])
	}
	return QuarantinePrefix + summary + "]"
}

// Quarantine rewrites, in place, every tool result in a request whose tool
// untrusted reports as reading untrusted content, to its QuarantineSummary.
// It returns how many text fields it rewrote. Results whose tool the
// conversation doesn't name are left alone.
func Quarantine(req *schemas.BifrostRequest, untrusted func(tool string) bool) int {
	if req == nil {
		return 0
	}
	refs, _ := walkRequest(req)
	n := 0
	for _, ref := range refs {
		if ref.result.Name == "" || !untrusted(ref.result.Name) {
			continue
		}
		for _, text := range ref.texts {
			if strings.HasPrefix(*text, QuarantinePrefix) {
				continue
			}
			*text = QuarantineSummary(*text)
			n++
		}
	}
	return n
}
