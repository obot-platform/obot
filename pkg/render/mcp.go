package render

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/obot/pkg/jwt"
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

func mcpServerTool(ctx context.Context, tokenService *jwt.TokenService, gptClient *gptscript.GPTScript, mcpServer v1.MCPServer, projectName, serverURL string, allowedTools []string) (gptscript.ToolDef, error) {
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

	serverConfig, missingRequiredNames, err := mcp.ToServerConfig(tokenService, mcpServer, serverURL, projectName, credEnv, allowedTools...)
	if err != nil {
		return gptscript.ToolDef{}, fmt.Errorf("failed to convert MCP server %s to server config: %w", mcpServer.Spec.Manifest.Name, err)
	}

	if len(missingRequiredNames) > 0 {
		return gptscript.ToolDef{}, &UnconfiguredMCPError{
			MCPName: mcpServer.Spec.Manifest.Name,
			Missing: missingRequiredNames,
		}
	}

	return serverToolWithCreds(mcpServer, serverConfig)
}

func serverToolWithCreds(mcpServer v1.MCPServer, serverConfig mcp.ServerConfig) (gptscript.ToolDef, error) {
	b, err := json.Marshal(serverConfig)
	if err != nil {
		return gptscript.ToolDef{}, fmt.Errorf("failed to marshal MCP Server %s config: %w", mcpServer.Spec.Manifest.Name, err)
	}

	name := mcpServer.Spec.Manifest.Name
	if name == "" {
		name = mcpServer.Name
	}

	return gptscript.ToolDef{
		Name:         name + "-bundle",
		Instructions: fmt.Sprintf("%s\n%s", types.MCPPrefix, string(b)),
	}, nil
}
