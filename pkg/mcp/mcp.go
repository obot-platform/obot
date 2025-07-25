package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/jwt"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) GPTScriptTools(ctx context.Context, tokenService *jwt.TokenService, projectMCPServer v1.ProjectMCPServer, userID, mcpServerDisplayName, serverURL string, allowedTools []string) ([]gptscript.ToolDef, error) {
	if mcpServerDisplayName == "" {
		mcpServerDisplayName = projectMCPServer.Name
	}

	serverConfig, err := ProjectServerToConfig(tokenService, projectMCPServer, serverURL, userID, allowedTools...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert MCP server %s to server config: %w", mcpServerDisplayName, err)
	}

	client, err := sm.ClientForServer(ctx, userID, mcpServerDisplayName, projectMCPServer.Name, serverConfig)
	if err != nil {
		var uae nmcp.AuthRequiredErr
		if errors.As(err, &uae) {
			// If the MCP server needs OAuth, ignore it and let the chat continue.
			return nil, nil
		}
		return nil, fmt.Errorf("failed to create MCP client for server %s: %w", mcpServerDisplayName, err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		var uae nmcp.AuthRequiredErr
		if errors.As(err, &uae) {
			// If the MCP server needs OAuth, ignore it and let the chat continue.
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list tools for MCP server %s: %w", mcpServerDisplayName, err)
	}

	allToolsAllowed := allowedTools == nil || slices.Contains(allowedTools, "*")

	toolDefs := []gptscript.ToolDef{{ /* this is a placeholder for main tool */ }}
	var toolNames []string

	for _, tool := range tools.Tools {
		if !allToolsAllowed && !slices.Contains(allowedTools, tool.Name) {
			continue
		}
		if tool.Name == "" {
			// I dunno, bad tool?
			continue
		}

		var schema jsonschema.Schema

		schemaData, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool input schema: %w", err)
		}

		if err := json.Unmarshal(schemaData, &schema); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool input schema: %w", err)
		}

		annotations, err := json.Marshal(tool.Annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool annotations: %w", err)
		}

		toolDef := gptscript.ToolDef{
			Name:         tool.Name,
			Description:  tool.Description,
			Arguments:    &schema,
			Instructions: types.MCPInvokePrefix + tool.Name + " " + client.ID,
		}

		if string(annotations) != "{}" && string(annotations) != "null" {
			toolDef.MetaData = map[string]string{
				"mcp-tool-annotations": string(annotations),
			}
		}

		if tool.Annotations != nil && tool.Annotations.Title != "" && !slices.Contains(strings.Fields(tool.Annotations.Title), "as") {
			toolNames = append(toolNames, tool.Name+" as "+tool.Annotations.Title)
		} else {
			toolNames = append(toolNames, tool.Name)
		}

		toolDefs = append(toolDefs, toolDef)
	}

	main := gptscript.ToolDef{
		Name:        mcpServerDisplayName + "-bundle",
		Description: client.Session.InitializeResult.ServerInfo.Name,
		Export:      toolNames,
		MetaData: map[string]string{
			"bundle": "true",
		},
	}

	if client.Session.InitializeResult.Instructions != "" {
		data, _ := json.Marshal(map[string]any{
			"tools":        toolNames,
			"instructions": client.Session.InitializeResult.Instructions,
		})
		toolDefs = append(toolDefs, gptscript.ToolDef{
			Name: client.ID,
			Type: "context",
			Instructions: types.EchoPrefix + "\n" + `# START MCP SERVER INFO: ` + client.Session.InitializeResult.ServerInfo.Name + "\n" +
				`You have available the following tools from an MCP Server that has provided the following additional instructions` + "\n" +
				string(data) + "\n" +
				`# END MCP SERVER INFO` + "\n",
		})

		main.ExportContext = append(main.ExportContext, client.ID)
	}

	toolDefs[0] = main
	return toolDefs, nil
}
