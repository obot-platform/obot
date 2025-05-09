package render

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type UnconfiguredMCPError struct {
	MCPName string
	Missing []string
}

func (e *UnconfiguredMCPError) Error() string {
	return fmt.Sprintf("MCP server %s missing required configuration parameters: %s", e.MCPName, strings.Join(e.Missing, ", "))
}

func mcpServerTool(ctx context.Context, thread *v1.Thread, gptClient *gptscript.GPTScript, mcpServer v1.MCPServer, allowTools []string) (gptscript.ToolDef, error) {
	var credEnv map[string]string
	if len(mcpServer.Spec.Manifest.Env) != 0 || len(mcpServer.Spec.Manifest.Headers) != 0 {
		var credCtxs []string
		// Add any local MCP credentials from the thread scope.
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", thread.Name, mcpServer.Name))

		if parent := thread.Spec.ParentThreadName; parent != "" {
			// Add any local MCP credentials from the parent project scope.
			// For non-project child threads of an agent project, this will include local credentials from the agent project.
			// For non-project child threads of a chatbot project, this will include local credentials from the chatbot project.
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", parent, mcpServer.Name))

			if parent != mcpServer.Spec.ThreadName {
				// Add shared MCP credentials from the agent project to chatbot threads.
				credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", mcpServer.Spec.ThreadName, mcpServer.Name))
			}
		}

		cred, err := gptClient.RevealCredential(ctx, credCtxs, mcpServer.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return gptscript.ToolDef{}, fmt.Errorf("failed to reveal credential for MCP server %s: %w", mcpServer.Spec.Manifest.Name, err)
		}

		credEnv = cred.Env
	}

	return MCPServerToolWithCreds(mcpServer, credEnv, allowTools...)
}

func MCPServerToolWithCreds(mcpServer v1.MCPServer, credEnv map[string]string, allowedTools ...string) (gptscript.ToolDef, error) {
	serverConfig, missingRequiredNames := mcp.ToServerConfig(mcpServer, credEnv, allowedTools)

	if len(missingRequiredNames) > 0 {
		return gptscript.ToolDef{}, &UnconfiguredMCPError{
			MCPName: mcpServer.Spec.Manifest.Name,
			Missing: missingRequiredNames,
		}
	}

	b, err := json.Marshal(serverConfig)
	if err != nil {
		return gptscript.ToolDef{}, fmt.Errorf("failed to marshal MCP Server %s config: %w", mcpServer.Spec.Manifest.Name, err)
	}

	return gptscript.ToolDef{
		Name:         mcpServer.Spec.Manifest.Name,
		Instructions: fmt.Sprintf("%s\n%s", types.MCPPrefix, string(b)),
	}, nil
}
