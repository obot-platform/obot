package render

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
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

func mcpServerTool(ctx context.Context, gptClient *gptscript.GPTScript, mcpServer v1.MCPServer, projectName string, allowedTools []string) (gptscript.ToolDef, error) {
	var credEnv map[string]string
	if len(mcpServer.Spec.Manifest.Env) != 0 || len(mcpServer.Spec.Manifest.Headers) != 0 {
		// Add the credential context for the direct parent to pick up credentials specifically for this project.
		credCtxs := []string{fmt.Sprintf("%s-%s", projectName, mcpServer.Name)}
		if projectName != mcpServer.Spec.ThreadName {
			// Add shared MCP credentials from the agent project to chatbot threads.
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s-shared", mcpServer.Spec.ThreadName, mcpServer.Name))
		}

		cred, err := gptClient.RevealCredential(ctx, credCtxs, mcpServer.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return gptscript.ToolDef{}, fmt.Errorf("failed to reveal credential for MCP server %s: %w", mcpServer.Spec.Manifest.Name, err)
		}

		credEnv = cred.Env
	}

	serverConfig, missingRequiredNames := mcp.ToServerConfig(mcpServer, projectName, credEnv, allowedTools...)

	if len(missingRequiredNames) > 0 {
		return gptscript.ToolDef{}, &UnconfiguredMCPError{
			MCPName: mcpServer.Spec.Manifest.Name,
			Missing: missingRequiredNames,
		}
	}
	return mcp.ServerToolWithCreds(mcpServer, serverConfig)
}
