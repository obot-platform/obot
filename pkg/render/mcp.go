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

	return MCPServerToolWithCreds(mcpServer, thread.Name, credEnv, allowTools...)
}

func MCPServerToolWithCreds(mcpServer v1.MCPServer, projectThreadName string, credEnv map[string]string, allowedTools ...string) (gptscript.ToolDef, error) {
	serverConfig := gmcp.ServerConfig{
		DisableInstruction: false,
		Command:            mcpServer.Spec.Manifest.Command,
		Args:               mcpServer.Spec.Manifest.Args,
		Env:                make([]string, 0, len(mcpServer.Spec.Manifest.Env)),
		URL:                mcpServer.Spec.Manifest.URL,
		Headers:            make([]string, 0, len(mcpServer.Spec.Manifest.Headers)),
		Scope:              projectThreadName,
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

	name := mcpServer.Spec.Manifest.Name
	if name == "" {
		name = mcpServer.Name
	}

	return gptscript.ToolDef{
		Name:         name,
		Instructions: fmt.Sprintf("%s\n%s", types.MCPPrefix, string(b)),
	}, nil
}
