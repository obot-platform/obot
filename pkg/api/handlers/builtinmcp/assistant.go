package builtinmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
		return err
	}
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "obot-ai"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		return err
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	tools := make([]schemas.ChatTool, 0, len(listed.Tools))
	for _, t := range listed.Tools {
		tool, err := chatTool(t)
		if err != nil {
			return err
		}
		tools = append(tools, tool)
	}

	messages := []schemas.ChatMessage{textMessage(schemas.ChatMessageRoleSystem, a.systemPrompt(req, admin))}
	for _, m := range in.Messages {
		switch m.Role {
		case "user":
			messages = append(messages, textMessage(schemas.ChatMessageRoleUser, m.Content))
		case "assistant":
			messages = append(messages, textMessage(schemas.ChatMessageRoleAssistant, m.Content))
		}
	}

	out := AssistantResponse{Steps: []AssistantStep{}}
	for range assistantMaxSteps {
		reply, model, err := a.model.ChatTurn(ctx, assistantModel, messages, tools)
		out.Model = model
		if err != nil {
			return types.NewErrHTTP(502, "Obot AI could not answer: "+err.Error())
		}
		var calls []schemas.ChatAssistantMessageToolCall
		if reply.ChatAssistantMessage != nil {
			calls = reply.ToolCalls
		}
		if len(calls) == 0 {
			out.Reply = messageText(reply)
			return req.Write(out)
		}
		messages = append(messages, *reply)
		for _, call := range calls {
			step, result := a.call(ctx, session, call)
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
	return req.Write(out)
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
- Be concise. Use short paragraphs or lists and include IDs and links the user can act on.`,
		role, userName(req), time.Now().UTC().Format("Monday, January 2, 2006"))
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
