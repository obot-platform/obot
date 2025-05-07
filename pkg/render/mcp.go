package render

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"
	"github.com/gptscript-ai/gptscript/pkg/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type UnconfiguredMCPError struct {
	MCPName string
	Missing []string
}

func (e *UnconfiguredMCPError) Error() string {
	return fmt.Sprintf("MCP server %s missing required configuration parameters: %s", e.MCPName, strings.Join(e.Missing, ", "))
}

func mcpServerTool(ctx context.Context, gptClient *gptscript.GPTScript, mcpServer v1.MCPServer, allowTools []string) (gptscript.ToolDef, error) {
	var credEnv map[string]string
	if len(mcpServer.Spec.Manifest.Env) != 0 || len(mcpServer.Spec.Manifest.Headers) != 0 {
		cred, err := gptClient.RevealCredential(ctx, []string{fmt.Sprintf("%s-%s", mcpServer.Spec.ThreadName, mcpServer.Name)}, mcpServer.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return gptscript.ToolDef{}, fmt.Errorf("failed to reveal credential for MCP server %s: %w", mcpServer.Spec.Manifest.Name, err)
		}

		credEnv = cred.Env
	}

	return MCPServerToolWithCreds(mcpServer, credEnv, allowTools...)
}

func MCPServerToolWithCreds(mcpServer v1.MCPServer, credEnv map[string]string, allowedTools ...string) (gptscript.ToolDef, error) {
	serverConfig := gmcp.ServerConfig{
		DisableInstruction: false,
		Command:            mcpServer.Spec.Manifest.Command,
		Args:               mcpServer.Spec.Manifest.Args,
		Env:                make([]string, 0, len(mcpServer.Spec.Manifest.Env)),
		URL:                mcpServer.Spec.Manifest.URL,
		Headers:            make([]string, 0, len(mcpServer.Spec.Manifest.Headers)),
		Scope:              mcpServer.Spec.ThreadName,
		AllowedTools:       allowedTools,
	}

	var missingRequiredNames []string
	for _, env := range mcpServer.Spec.Manifest.Env {
		val, ok := credEnv[env.Key]
		if !ok && env.Required {
			name := env.Name
			if name == "" {
				name = env.Key
			}
			missingRequiredNames = append(missingRequiredNames, name)
			continue
		}

		serverConfig.Env = append(serverConfig.Env, fmt.Sprintf("%s=%s", env.Key, val))
	}

	for _, header := range mcpServer.Spec.Manifest.Headers {
		val, ok := credEnv[header.Key]
		if !ok && header.Required {
			name := header.Name
			if name == "" {
				name = header.Key
			}
			missingRequiredNames = append(missingRequiredNames, name)
			continue
		}

		serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
	}

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
