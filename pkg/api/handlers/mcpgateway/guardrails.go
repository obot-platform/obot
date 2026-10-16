package mcpgateway

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/mcp"
)

const (
	// guardrailHookTool is the tool recorded in audit logs for guardrail filters, which have
	// no filter server or tool.
	guardrailHookTool = "guardrail"
)

// mcpGuardrails runs the guardrail filters of one proxied MCP exchange in-process.
type mcpGuardrails struct {
	engine *guardrails.Engine
	// message holds what every check shares: the caller, the server and the headers.
	message guardrails.MCPMessage
}

// newMCPGuardrails returns what guardrail filters need to check a request's messages.
func newMCPGuardrails(engine *guardrails.Engine, identity guardrails.Identity, serverConfig mcp.ServerConfig, header http.Header) *mcpGuardrails {
	headers := map[string]string{}
	for k, v := range header {
		lower := strings.ToLower(k)
		if lower == "authorization" || lower == "cookie" || lower == "x-api-key" || lower == "proxy-authorization" {
			continue
		}
		headers[lower] = strings.Join(v, ",")
	}
	return &mcpGuardrails{
		engine: engine,
		message: guardrails.MCPMessage{
			Identity: identity,
			Server:   cmp.Or(serverConfig.MCPServerDisplayName, serverConfig.MCPServerName),
			ServerID: serverConfig.MCPServerName,
			Headers:  headers,
		},
	}
}

// run checks a message with a guardrail filter and answers like a filter server would: a
// block rejects the message and a redaction mutates it. It returns nil when the filter
// didn't apply. requestParams are the params of the request a response answers.
func (g *mcpGuardrails) run(ctx context.Context, target mcp.HookTarget, message mcp.Message, method, name, direction string, requestParams json.RawMessage) (*mcp.SessionMessageHook, error) {
	if g == nil || g.engine == nil {
		return nil, errors.New("guardrails are not available")
	}
	msg := g.message
	msg.Method = method
	msg.Name = name
	switch direction {
	case "request":
		msg.Stage = guardrails.StageInput
		msg.Params = message.Params
	case "response":
		if message.Error != nil || len(message.Result) == 0 {
			return nil, nil
		}
		msg.Stage = guardrails.StageOutput
		msg.Params = requestParams
		msg.Result = message.Result
	default:
		return nil, nil
	}

	decision := g.engine.CheckMCPFilter(ctx, guardrails.MCPFilter{
		ID:     target.Guardrail.FilterName,
		Name:   cmp.Or(target.Name, target.Guardrail.FilterName),
		Config: target.Guardrail.Config,
	}, msg)
	if !decision.Applied {
		return nil, nil
	}
	if decision.Blocked {
		return &mcp.SessionMessageHook{Accept: false, Message: &message, Reason: decision.Message}, nil
	}
	// Guardrails that failed without blocking, because the filter fails open, are named in the
	// reason, which the MCP audit log records as the filter's status message.
	failures := strings.Join(decision.Errors, "; ")
	if decision.Params == nil && decision.Result == nil {
		return &mcp.SessionMessageHook{Accept: true, Message: &message, Reason: failures}, nil
	}
	if decision.Params != nil {
		message.Params = decision.Params
	}
	if decision.Result != nil {
		message.Result = decision.Result
	}
	reason := "redacted by guardrail filter " + cmp.Or(target.Name, target.Guardrail.FilterName)
	if failures != "" {
		reason += "; " + failures
	}
	return &mcp.SessionMessageHook{
		Accept: true, Mutated: true, Message: &message, Reason: reason,
	}, nil
}
