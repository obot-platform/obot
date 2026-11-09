package builtinmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

const (
	assistantModel    = string(types.DefaultModelAliasTypeLLM)
	assistantMaxSteps = 12
	assistantTimeout  = 3 * time.Minute
	assistantMaxTurns = 40
	toolResultLimit   = 12000
)

// ChatModel takes one turn of a tool-using conversation.
type ChatModel interface {
	ChatTurn(ctx context.Context, model string, messages []schemas.ChatMessage, tools []schemas.ChatTool) (*schemas.ChatMessage, string, error)
}

// AssistantEvent is one server-sent event of a streamed answer.
type AssistantEvent struct {
	// Type is thinking (a model turn started), tool_start, tool_end, delta (answer text),
	// done or error.
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	IsError   bool            `json:"isError,omitempty"`
	Reply     string          `json:"reply,omitempty"`
	Model     string          `json:"model,omitempty"`
	Steps     []AssistantStep `json:"steps,omitempty"`
	Message   string          `json:"message,omitempty"`
}

// StreamingChatModel is a ChatModel that can stream a turn's text as it is written.
type StreamingChatModel interface {
	ChatTurnStream(ctx context.Context, model string, messages []schemas.ChatMessage, tools []schemas.ChatTool, onText func(string)) (*schemas.ChatMessage, string, error)
}

// eventStream writes server-sent events, flushing each one.
type eventStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
	lock    sync.Mutex
}

// AssistantMessage is one message of the conversation the UI keeps.
type AssistantMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AssistantRequest struct {
	Messages []AssistantMessage `json:"messages"`
}

// AssistantStep is a tool the assistant called while answering.
type AssistantStep struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	IsError   bool            `json:"isError,omitempty"`
	Result    string          `json:"result,omitempty"`
}

type AssistantResponse struct {
	Reply string          `json:"reply"`
	Model string          `json:"model,omitempty"`
	Steps []AssistantStep `json:"steps"`
}

// Assistant is Obot AI: a chat that answers questions about Obot and acts on it with the
// tools of Obot's built-in MCP servers, as the signed-in user.
type Assistant struct {
	servers *Handler
	model   ChatModel
}

func NewAssistant(servers *Handler, model ChatModel) *Assistant {
	return &Assistant{servers: servers, model: model}
}

// Chat answers the conversation's last message. A request that accepts text/event-stream
// gets the answer as server-sent events as it is produced: each model turn, each tool call
// as it starts and ends, the answer's text as it is written, then the complete reply.
// Otherwise the complete reply is returned as JSON.
func (a *Assistant) Chat(req api.Context) error {
	var in AssistantRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if len(in.Messages) == 0 || in.Messages[len(in.Messages)-1].Role != "user" {
		return types.NewErrBadRequest("the conversation must end with a user message")
	}
	if len(in.Messages) > assistantMaxTurns {
		in.Messages = in.Messages[len(in.Messages)-assistantMaxTurns:]
	}
	if a.model == nil {
		return types.NewErrHTTP(503, "Obot AI needs the LLM gateway")
	}

	ctx, cancel := context.WithTimeout(req.Context(), assistantTimeout)
	defer cancel()

	if !strings.Contains(req.Request.Header.Get("Accept"), "text/event-stream") {
		out, err := a.run(ctx, req, in.Messages, nil)
		if err != nil {
			return err
		}
		return req.Write(out)
	}

	events, err := newEventStream(req.ResponseWriter)
	if err != nil {
		return err
	}
	out, err := a.run(ctx, req, in.Messages, events.send)
	if err != nil {
		events.send(AssistantEvent{Type: "error", Message: errorMessage(err)})
		return nil
	}
	events.send(AssistantEvent{Type: "done", Reply: out.Reply, Model: out.Model, Steps: out.Steps})
	return nil
}

// run answers the conversation, reporting progress to emit when it is set.
func (a *Assistant) run(ctx context.Context, req api.Context, conversation []AssistantMessage, emit func(AssistantEvent)) (AssistantResponse, error) {
	if emit == nil {
		emit = func(AssistantEvent) {}
	}
	// Administrators get the admin server's tools, which include everything a user's has
	// that matters to them; others get the user server's.
	admin := req.UserIsAdmin()
	server := a.servers.userServer(req)
	if admin {
		server = a.servers.adminServer(req)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return AssistantResponse{}, err
	}
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "obot-ai"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		return AssistantResponse{}, err
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return AssistantResponse{}, err
	}
	tools := make([]schemas.ChatTool, 0, len(listed.Tools))
	for _, t := range listed.Tools {
		tool, err := chatTool(t)
		if err != nil {
			return AssistantResponse{}, err
		}
		tools = append(tools, tool)
	}

	messages := []schemas.ChatMessage{textMessage(schemas.ChatMessageRoleSystem, a.systemPrompt(req, admin))}
	for _, m := range conversation {
		switch m.Role {
		case "user":
			messages = append(messages, textMessage(schemas.ChatMessageRoleUser, m.Content))
		case "assistant":
			messages = append(messages, textMessage(schemas.ChatMessageRoleAssistant, m.Content))
		}
	}

	out := AssistantResponse{Steps: []AssistantStep{}}
	for range assistantMaxSteps {
		emit(AssistantEvent{Type: "thinking"})
		reply, model, err := a.turn(ctx, messages, tools, func(text string) {
			emit(AssistantEvent{Type: "delta", Text: text})
		})
		out.Model = model
		if err != nil {
			return out, types.NewErrHTTP(502, "Obot AI could not answer: "+err.Error())
		}
		var calls []schemas.ChatAssistantMessageToolCall
		if reply.ChatAssistantMessage != nil {
			calls = reply.ToolCalls
		}
		if len(calls) == 0 {
			out.Reply = messageText(reply)
			return out, nil
		}
		messages = append(messages, *reply)
		for _, call := range calls {
			name := ""
			if call.Function.Name != nil {
				name = *call.Function.Name
			}
			start := AssistantEvent{Type: "tool_start", Tool: name}
			if json.Valid([]byte(call.Function.Arguments)) {
				start.Arguments = json.RawMessage(call.Function.Arguments)
			}
			emit(start)
			step, result := a.call(ctx, session, call)
			emit(AssistantEvent{Type: "tool_end", Tool: name, IsError: step.IsError})
			out.Steps = append(out.Steps, step)
			id := ""
			if call.ID != nil {
				id = *call.ID
			}
			toolMessage := textMessage(schemas.ChatMessageRoleTool, result)
			toolMessage.ChatToolMessage = &schemas.ChatToolMessage{ToolCallID: &id}
			messages = append(messages, toolMessage)
		}
	}
	out.Reply = "I stopped after too many steps without an answer. Try a narrower question."
	return out, nil
}

// turn takes one model turn, streaming its text to onText when the model can stream.
func (a *Assistant) turn(ctx context.Context, messages []schemas.ChatMessage, tools []schemas.ChatTool, onText func(string)) (*schemas.ChatMessage, string, error) {
	if streaming, ok := a.model.(StreamingChatModel); ok {
		return streaming.ChatTurnStream(ctx, assistantModel, messages, tools, onText)
	}
	return a.model.ChatTurn(ctx, assistantModel, messages, tools)
}

func newEventStream(w http.ResponseWriter) (*eventStream, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming is not supported")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return &eventStream{w: w, flusher: flusher}, nil
}

func (e *eventStream) send(ev AssistantEvent) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	e.lock.Lock()
	defer e.lock.Unlock()
	_, _ = fmt.Fprintf(e.w, "data: %s\n\n", b)
	e.flusher.Flush()
}

func errorMessage(err error) string {
	if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
		return httpErr.Message
	}
	return err.Error()
}

// call runs one tool call and returns the step to show and the result for the model.
func (a *Assistant) call(ctx context.Context, session *mcp.ClientSession, call schemas.ChatAssistantMessageToolCall) (AssistantStep, string) {
	name := ""
	if call.Function.Name != nil {
		name = *call.Function.Name
	}
	step := AssistantStep{Tool: name}
	args := map[string]any{}
	if raw := strings.TrimSpace(call.Function.Arguments); raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			step.IsError = true
			step.Result = "invalid arguments: " + err.Error()
			return step, step.Result
		}
		step.Arguments = json.RawMessage(raw)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		step.IsError = true
		step.Result = err.Error()
		return step, step.Result
	}
	var text strings.Builder
	for _, c := range result.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			text.WriteString(t.Text)
		}
	}
	full := text.String()
	if len(full) > toolResultLimit {
		full = full[:toolResultLimit] + "\n…(truncated)"
	}
	step.IsError = result.IsError
	step.Result = full
	return step, full
}

func (a *Assistant) systemPrompt(req api.Context, admin bool) string {
	role := "a user"
	if admin {
		role = "an administrator"
	}
	return fmt.Sprintf(`You are Obot AI, the assistant built into Obot, an AI governance platform. Obot gives an organization MCP servers (through vMCPs and a catalog), an LLM gateway, Claude environments, device management, and an AI Supervisor that watches agent activity and enforces Obot Policy.

You are talking to %s (%s). Today is %s.

Help them understand and run their AI enablement. The areas that matter most are:
- Cost: LLM spend and usage, and where it could be lower.
- Security and compliance: risky agent activity, policy decisions and approvals. Ask the Supervisor.
- Operations: Claude environment sessions and the MCP servers Obot runs.
- Adoption: who is using AI through Obot, what they use, and what they are asking for.

Rules:
- Answer from tool results. Never invent numbers, names or IDs; say when a tool gives you nothing.
- Prefer one or two well-chosen tool calls over many.
- Before anything that changes Obot, such as approving or denying a request, say what you will do and do it only when the user has clearly asked for it in this conversation.
- Be concise. Use short paragraphs or lists, and include IDs and links the user can act on unless a widget already shows them.

%s`,
		role, userName(req), time.Now().UTC().Format("Monday, January 2, 2006"), widgetPrompt(admin))
}

// widgetPrompt teaches the model the live views the AI page renders. The page loads each
// widget's data with the viewer's own permissions, so a widget never shows more than the viewer
// could see elsewhere in Obot.
func widgetPrompt(admin bool) string {
	widgets := []string{
		`{"type":"mcp_server_request","requestId":"<id>"}: one MCP server request as a card with its state; administrators can approve or deny a pending one on it.`,
	}
	if admin {
		widgets = append([]string{
			`{"type":"session","sessionId":"<id>"}: one agent session with its risk, labels and live event stream, as on its session page. Use it when the user asks what a session did or to see its events.`,
			`{"type":"sessions","sessionIds":["<id>", ...]}: a list of sessions, at most 10, in the order given, each with its risk, user, surfaces, labels and latest activity. Use it whenever your answer covers more than one session.`,
			`{"type":"approvals"}: everything waiting on the user's decision (agent actions the Supervisor is holding and pending MCP server requests), with Approve and Deny buttons the user clicks. Show it when the user asks what needs approval or wants to approve or deny things; it needs no tool call first.`,
		}, widgets...)
	}
	return "Widgets: the chat can show live views from Obot inside your answer. To show one, write a fenced code block " +
		"with the language obot-widget holding one JSON object on its own, for example:\n" +
		"```obot-widget\n{\"type\":\"mcp_server_request\",\"requestId\":\"msrq1abc\"}\n```\n" +
		"Available widgets:\n- " + strings.Join(widgets, "\n- ") + "\n" +
		"Use only IDs that tools returned in this conversation. Whenever the user asks to see, show or look at a " +
		"session or request, or asks what a session did, include its widget. A widget shows the details (risk, labels, " +
		"counters, events), so never list them in text as well; add a sentence or two on what matters instead. Don't " +
		"add widgets for things the user didn't ask about. " +
		"When the user wants to decide something themselves, prefer a widget with buttons over approving it with a tool."
}

func userName(req api.Context) string {
	if emails := req.User.GetExtra()["email"]; len(emails) > 0 && emails[0] != "" {
		return emails[0]
	}
	return req.User.GetName()
}

func textMessage(role schemas.ChatMessageRole, text string) schemas.ChatMessage {
	return schemas.ChatMessage{Role: role, Content: &schemas.ChatMessageContent{ContentStr: &text}}
}

func messageText(m *schemas.ChatMessage) string {
	if m == nil || m.Content == nil {
		return ""
	}
	if m.Content.ContentStr != nil {
		return *m.Content.ContentStr
	}
	var b strings.Builder
	for _, block := range m.Content.ContentBlocks {
		if block.Text != nil {
			b.WriteString(*block.Text)
		}
	}
	return b.String()
}

// chatTool describes an MCP tool to the model as a function.
func chatTool(t *mcp.Tool) (schemas.ChatTool, error) {
	description := t.Description
	tool := schemas.ChatTool{
		Type:     schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{Name: t.Name, Description: &description},
	}
	if t.InputSchema != nil {
		raw, err := json.Marshal(t.InputSchema)
		if err != nil {
			return tool, err
		}
		var params schemas.ToolFunctionParameters
		if err := json.Unmarshal(raw, &params); err != nil {
			return tool, fmt.Errorf("tool %s: %w", t.Name, err)
		}
		tool.Function.Parameters = &params
	}
	return tool, nil
}
