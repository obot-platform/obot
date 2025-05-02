package render

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"
	"github.com/gptscript-ai/gptscript/pkg/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func mcpServerTool(ctx context.Context, gptClient *gptscript.GPTScript, mcpServer v1.MCPServer) (gptscript.ToolDef, error) {
	var credEnv map[string]string
	if len(mcpServer.Spec.Manifest.Env) != 0 {
		cred, err := gptClient.RevealCredential(ctx, []string{fmt.Sprintf("%s-%s", mcpServer.Spec.ThreadName, mcpServer.Name)}, mcpServer.Name)
		if err != nil {
			return gptscript.ToolDef{}, fmt.Errorf("MCP Server %s missing required credential: %w", mcpServer.Spec.Manifest.Name, err)
		}

		credEnv = cred.Env
	}

	return MCPServerToolWithCreds(mcpServer, credEnv)
}

func MCPServerToolWithCreds(mcpServer v1.MCPServer, credEnv map[string]string) (gptscript.ToolDef, error) {
	serverConfig := gmcp.ServerConfig{
		DisableInstruction: false,
		Command:            mcpServer.Spec.Manifest.Command,
		Args:               mcpServer.Spec.Manifest.Args,
		Env:                make([]string, 0, len(mcpServer.Spec.Manifest.Env)),
		Scope:              mcpServer.Spec.ThreadName,
	}

	for _, env := range mcpServer.Spec.Manifest.Env {
		val, ok := credEnv[env.Name]
		if !ok && env.Required {
			return gptscript.ToolDef{}, fmt.Errorf("MCP Server %s missing required environment variable %s", mcpServer.Spec.Manifest.Name, env.Name)
		}

		serverConfig.Env = append(serverConfig.Env, fmt.Sprintf("%s=%s", env.Name, val))
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
