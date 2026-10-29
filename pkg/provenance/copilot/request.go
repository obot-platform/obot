// Package copilot adapts Microsoft Copilot Studio's external threat detection
// hook ("runtime monitoring", public preview) to the provenance engine.
//
// Before a Copilot Studio agent invokes a tool it POSTs the planner context
// (the user's message, recent chat history, previous tool outputs), the tool
// definition, the input values and conversation metadata to the configured
// endpoint, and expects {"blockAction": bool} back within 1,000 ms. No answer
// means allow. The documented request shape (api-version 2025-05-01) is:
//
//	{
//	  "plannerContext": {
//	    "userMessage": "...", "thought": "...",
//	    "chatHistory": [{"id", "role": "user|assistant", "content", "timestamp"}],
//	    "previousToolOutputs": [{"toolId", "toolName", "outputs": [{"name", "value", ...}], "timestamp"}]
//	  },
//	  "toolDefinition": {"id", "type", "name", "description", "inputParameters": [...], "outputParameters": [...]},
//	  "inputValues": {"<param>": <value>},
//	  "conversationMetadata": {
//	    "agent": {"id", "tenantId", "environmentId", "version", "isPublished"},
//	    "user": {"id", "tenantId"}, "trigger": {"id", "schemaName"},
//	    "conversationId", "planId", "planStepId", "parentAgentComponentId"
//	  }
//	}
//
// The parser is tolerant: keys are matched ignoring case, underscores and
// dashes, common aliases are accepted (prompt/userMessage/query,
// conversationId/sessionId, previousToolOutputs/previousToolsOutputs, ...),
// a plan may carry several actions ("actions", "plan", "steps", "tools"),
// and unknown fields are ignored, as Microsoft asks of providers.
package copilot

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/pkg/provenance"
)

// Request is one analyze-tool-execution call, normalized.
type Request struct {
	// ConversationID is the Copilot Studio conversation; it is the session's
	// client identifier.
	ConversationID string `json:"conversationId,omitempty"`
	PlanID         string `json:"planId,omitempty"`
	PlanStepID     string `json:"planStepId,omitempty"`
	TenantID       string `json:"tenantId,omitempty"`
	AgentID        string `json:"agentId,omitempty"`
	AgentName      string `json:"agentName,omitempty"`
	// EnvironmentID is the Power Platform environment the agent runs in.
	EnvironmentID string `json:"environmentId,omitempty"`
	// UserID is the user's Microsoft Entra object ID.
	UserID string `json:"userId,omitempty"`
	// UserEmail is the user's email or UPN, when the payload carries one (the
	// documented shape does not).
	UserEmail string `json:"userEmail,omitempty"`
	// Prompt is the user's message the planner is acting on.
	Prompt string `json:"prompt,omitempty"`
	// Thought is the planner's explanation for choosing the tool.
	Thought     string         `json:"thought,omitempty"`
	History     []Message      `json:"history,omitempty"`
	ToolOutputs []ToolOutput   `json:"toolOutputs,omitempty"`
	Actions     []PlanAction   `json:"actions,omitempty"`
	Correlation string         `json:"correlationId,omitempty"`
	Raw         map[string]any `json:"-"`
}

// Message is one chat history entry.
type Message struct {
	ID      string `json:"id,omitempty"`
	Role    string `json:"role"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content"`
}

// ToolOutput is the output of a tool the agent ran earlier in the
// conversation, flattened to text.
type ToolOutput struct {
	ToolID   string `json:"toolId,omitempty"`
	ToolName string `json:"toolName,omitempty"`
	Content  string `json:"content"`
}

// PlanAction is one tool invocation the planner intends.
type PlanAction struct {
	ID          string `json:"id,omitempty"`
	Type        string `json:"type,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Connector is the connector, plugin or MCP server the tool belongs to,
	// when the payload says.
	Connector string         `json:"connector,omitempty"`
	Inputs    map[string]any `json:"inputs,omitempty"`
}

// object is a decoded JSON object with tolerant key lookup.
type object map[string]any

// UnmarshalJSON parses the documented shape and its aliases.
func (r *Request) UnmarshalJSON(b []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*r = Parse(raw)
	return nil
}

// Parse normalizes a decoded payload.
func Parse(raw map[string]any) Request {
	root := object(raw)
	planner := root.obj("plannerContext", "planner", "context")
	meta := root.obj("conversationMetadata", "metadata", "conversation")
	agent := firstObject(meta.obj("agent"), root.obj("agent", "bot"))
	user := firstObject(meta.obj("user"), root.obj("user"))

	r := Request{
		Raw:            raw,
		Prompt:         first(planner.str("userMessage", "prompt", "query"), root.str("userMessage", "prompt", "query", "message")),
		Thought:        first(planner.str("thought"), root.str("thought")),
		ConversationID: first(meta.str("conversationId", "sessionId"), root.str("conversationId", "sessionId")),
		PlanID:         first(meta.str("planId"), root.str("planId")),
		PlanStepID:     first(meta.str("planStepId"), root.str("planStepId")),
		AgentID:        first(agent.str("id", "agentId", "botId"), meta.str("agentId", "botId"), root.str("agentId", "botId")),
		AgentName:      first(agent.str("name", "displayName", "agentName"), meta.str("agentName", "botName"), root.str("agentName", "botName")),
		EnvironmentID:  first(agent.str("environmentId"), meta.str("environmentId"), root.str("environmentId")),
		TenantID:       first(agent.str("tenantId"), user.str("tenantId"), meta.str("tenantId"), root.str("tenantId")),
		UserID:         first(user.str("id", "userId", "objectId", "oid"), meta.str("userId"), root.str("userId")),
		UserEmail:      first(user.str("email", "userPrincipalName", "upn", "mail"), meta.str("userEmail", "userPrincipalName"), root.str("userEmail", "userPrincipalName", "upn", "email")),
		Correlation:    root.str("correlationId"),
	}

	for _, item := range firstList(planner.list("chatHistory", "history", "messages"), root.list("chatHistory", "history", "messages")) {
		m := toObject(item)
		if m == nil {
			continue
		}
		r.History = append(r.History, Message{
			ID:      m.str("id"),
			Role:    strings.ToLower(m.str("role", "author", "from")),
			Name:    m.str("name", "toolName", "tool"),
			Content: textOf(m.get("content", "text", "message")),
		})
	}
	for _, item := range firstList(planner.list("previousToolOutputs", "previousToolsOutputs", "toolOutputs"), root.list("previousToolOutputs", "previousToolsOutputs", "toolOutputs")) {
		m := toObject(item)
		if m == nil {
			continue
		}
		r.ToolOutputs = append(r.ToolOutputs, ToolOutput{
			ToolID:   m.str("toolId", "id"),
			ToolName: m.str("toolName", "name", "tool"),
			Content:  outputText(m.get("outputs", "output", "result", "results", "value", "content")),
		})
	}

	if tool := root.obj("toolDefinition", "tool", "action"); tool != nil {
		r.Actions = append(r.Actions, parseAction(tool, root.get("inputValues", "inputs", "input", "parameters", "arguments")))
	}
	for _, key := range []string{"actions", "plan", "planSteps", "steps", "tools", "toolInvocations", "toolCalls"} {
		switch v := root.get(key).(type) {
		case []any:
			for _, item := range v {
				if m := toObject(item); m != nil {
					r.Actions = append(r.Actions, parseAction(m, nil))
				}
			}
		case map[string]any:
			for _, item := range object(v).list("steps", "actions", "tools") {
				if m := toObject(item); m != nil {
					r.Actions = append(r.Actions, parseAction(m, nil))
				}
			}
		}
	}
	return r
}

func parseAction(m object, inputs any) PlanAction {
	a := PlanAction{
		ID:          m.str("id", "toolId", "actionId"),
		Type:        m.str("type", "kind"),
		Name:        m.str("name", "toolName", "actionName", "displayName", "operationId"),
		Description: m.str("description", "summary"),
		Connector:   first(m.str("connector", "connectorName", "server", "mcpServer", "connectionReferenceName", "plugin", "source"), m.obj("connector", "server", "plugin").str("name", "displayName", "id")),
	}
	if a.Name == "" {
		a.Name = a.ID
	}
	if inputs == nil {
		inputs = m.get("inputValues", "inputs", "input", "parameters", "arguments", "args")
	}
	switch v := inputs.(type) {
	case nil:
	case map[string]any:
		a.Inputs = v
	default:
		a.Inputs = map[string]any{"input": v}
	}
	return a
}

// normKey folds case, underscores and dashes so conversationId,
// conversation_id and Conversation-ID are one key.
func normKey(k string) string {
	return strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(k))
}

// get returns the first non-nil value among keys.
func (o object) get(keys ...string) any {
	if o == nil {
		return nil
	}
	for _, key := range keys {
		if v, ok := o[key]; ok && v != nil {
			return v
		}
	}
	for _, key := range keys {
		want := normKey(key)
		for k, v := range o {
			if v != nil && normKey(k) == want {
				return v
			}
		}
	}
	return nil
}

func (o object) str(keys ...string) string {
	for _, key := range keys {
		switch v := o.get(key).(type) {
		case string:
			if v != "" {
				return v
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(v)
		}
	}
	return ""
}

func (o object) obj(keys ...string) object {
	for _, key := range keys {
		if m, ok := o.get(key).(map[string]any); ok {
			return m
		}
	}
	return nil
}

func (o object) list(keys ...string) []any {
	for _, key := range keys {
		if l, ok := o.get(key).([]any); ok {
			return l
		}
	}
	return nil
}

func toObject(v any) object {
	m, _ := v.(map[string]any)
	return m
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstObject(values ...object) object {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func firstList(values ...[]any) []any {
	for _, v := range values {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}

// textOf renders a message content value as text: a string as is, a list of
// content parts or an object flattened.
func textOf(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	}
	return strings.TrimSpace(provenance.FlattenArguments(v))
}

// outputText renders tool outputs as "name: value" lines. The documented
// shape is a list of {name, description, type, value}; a single object, a map
// of values or a string are accepted too.
func outputText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		var b strings.Builder
		for _, item := range t {
			if s := outputText(item); s != "" {
				b.WriteString(s)
				b.WriteByte('\n')
			}
		}
		return strings.TrimSpace(b.String())
	case map[string]any:
		o := object(t)
		if val := o.get("value"); val != nil {
			text := textOf(val)
			if name := o.str("name"); name != "" {
				return name + ": " + text
			}
			return text
		}
		return strings.TrimSpace(provenance.FlattenArguments(t))
	}
	return strings.TrimSpace(provenance.FlattenArguments(v))
}
