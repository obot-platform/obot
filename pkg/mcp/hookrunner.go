package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	hookClientScope = "Obot MCP Hook"
)

// HookServerConfigs maps the server portion of a hook target (server/tool) to
// the MCP server configuration used to call that hook.
type HookServerConfigs map[string]ServerConfig

type hookMCPClient interface {
	CallTool(context.Context, *gomcp.CallToolParams) (*gomcp.CallToolResult, error)
}

// SessionManagerHookRunner invokes hook tools through Obot's MCP session manager.
type SessionManagerHookRunner struct {
	clientForServer func(context.Context, ServerConfig) (hookMCPClient, error)
}

func NewHookRunner(sessionManager *SessionManager) *SessionManagerHookRunner {
	return &SessionManagerHookRunner{
		clientForServer: func(ctx context.Context, server ServerConfig) (hookMCPClient, error) {
			return sessionManager.clientForServerWithScope(ctx, hookClientScope, server)
		},
	}
}

func (r *SessionManagerHookRunner) RunHook(ctx context.Context, servers HookServerConfigs, input SessionMessageHook, target string) (*SessionMessageHook, error) {
	serverName, toolName, ok := strings.Cut(target, "/")
	if !ok || serverName == "" || toolName == "" {
		return nil, fmt.Errorf("invalid MCP hook target %q: expected server/tool", target)
	}

	server, ok := servers[serverName]
	if !ok {
		return nil, fmt.Errorf("MCP hook server %q is not configured", serverName)
	}
	if r == nil || r.clientForServer == nil {
		return nil, errors.New("MCP hook runner is not configured")
	}

	client, err := r.clientForServer(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for MCP hook server %q: %w", serverName, err)
	}
	result, err := client.CallTool(ctx, &gomcp.CallToolParams{Name: toolName, Arguments: input})
	if err != nil {
		return nil, fmt.Errorf("failed to call MCP hook %q: %w", target, err)
	}
	if result == nil {
		return nil, fmt.Errorf("MCP hook %q returned no result", target)
	}
	if result.IsError {
		return nil, mcpHookToolError(target, result)
	}

	structuredContent := result.StructuredContent
	if structuredContent == nil && len(result.Content) == 1 {
		if text, ok := result.Content[0].(*gomcp.TextContent); ok && text.Text != "" {
			var object map[string]any
			if json.Unmarshal([]byte(text.Text), &object) == nil {
				structuredContent = object
			}
		}
	}
	if structuredContent == nil {
		return nil, nil
	}

	data, err := json.Marshal(structuredContent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP hook %q response: %w", target, err)
	}
	var output SessionMessageHook
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("failed to decode MCP hook %q response: %w", target, err)
	}
	return &output, nil
}

func mcpHookToolError(target string, result *gomcp.CallToolResult) error {
	for _, content := range result.Content {
		if text, ok := content.(*gomcp.TextContent); ok && strings.TrimSpace(text.Text) != "" {
			return fmt.Errorf("MCP hook %q returned an error: %s", target, text.Text)
		}
	}
	if result.StructuredContent != nil {
		if data, err := json.Marshal(result.StructuredContent); err == nil {
			return fmt.Errorf("MCP hook %q returned an error: %s", target, data)
		}
	}
	return fmt.Errorf("MCP hook %q returned an error", target)
}
