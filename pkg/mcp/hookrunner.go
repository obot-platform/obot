package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

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

type hookSessionTransport struct {
	base http.RoundTripper
}

// SessionManagerHookRunner invokes hook tools through Obot's MCP session manager.
type SessionManagerHookRunner struct {
	clientForServer  func(context.Context, ServerConfig) (hookMCPClient, error)
	invalidateClient func(ServerConfig, hookMCPClient)
}

func (t *hookSessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Discard JSON-RPC bodies so the SDK records ErrSessionMissing and skips DELETE on close.
	if req.Method == http.MethodPost && req.Header.Get("Mcp-Session-Id") != "" && resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		resp.Body = http.NoBody
		resp.ContentLength = 0
		resp.Header.Del("Content-Length")
	}

	return resp, nil
}

func NewHookRunner(sessionManager *SessionManager) *SessionManagerHookRunner {
	return &SessionManagerHookRunner{
		clientForServer: func(ctx context.Context, server ServerConfig) (hookMCPClient, error) {
			return sessionManager.clientForServerWithScope(ctx, hookClientScope, server)
		},
		invalidateClient: func(server ServerConfig, client hookMCPClient) {
			sessions, ok := sessionManager.sessions.Load(server.MCPServerName)
			if !ok {
				return
			}

			// CloseClient removes whichever client is cached, possibly a concurrent replacement.
			// CompareAndDelete evicts only the failed client so that replacement stays usable.
			if sessions.(*sync.Map).CompareAndDelete(clientID(server, hookClientScope), client) {
				_ = client.(*Client).Close()
			}
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

	var result *gomcp.CallToolResult
	for attempt := range 2 {
		client, err := r.clientForServer(ctx, server)
		if err != nil {
			return nil, fmt.Errorf("failed to get client for MCP hook server %q: %w", serverName, err)
		}

		result, err = client.CallTool(ctx, &gomcp.CallToolParams{Name: toolName, Arguments: input})
		if err == nil {
			break
		}

		// Only explicit session loss permits replay: other failures may follow execution.
		if errors.Is(err, gomcp.ErrSessionMissing) && r.invalidateClient != nil {
			r.invalidateClient(server, client)
			if attempt == 0 {
				continue
			}
		}

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
