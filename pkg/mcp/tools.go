package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	"github.com/nanobot-ai/nanobot/pkg/mcp"
	otypes "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) ListTools(ctx context.Context, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) ([]mcp.Tool, error) {
	client, err := sm.ClientForMCPServer(ctx, userID, mcpServer, serverConfig)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP tools: %w", err)
	}

	return resp.Tools, nil
}

func ConvertTools(tools []mcp.Tool, allowedTools, unsupportedTools []string) ([]otypes.MCPServerTool, error) {
	allTools := allowedTools == nil || slices.Contains(allowedTools, "*")

	convertedTools := make([]otypes.MCPServerTool, 0, len(tools))
	for _, t := range tools {
		mcpTool := otypes.MCPServerTool{
			ID:          t.Name,
			Name:        t.Name,
			Description: t.Description,
			Enabled:     allTools && !slices.Contains(unsupportedTools, t.Name) || slices.Contains(allowedTools, t.Name),
			Unsupported: slices.Contains(unsupportedTools, t.Name),
		}

		if len(t.InputSchema) > 0 {
			var schema jsonschema.Schema

			schemaData, err := json.Marshal(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal input schema for tool %s: %w", t.Name, err)
			}

			if err = json.Unmarshal(schemaData, &schema); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool input schema: %w", err)
			}

			mcpTool.Params = make(map[string]string, len(schema.Properties))
			for name, param := range schema.Properties {
				if param != nil {
					mcpTool.Params[name] = param.Description
				}
			}
		}

		convertedTools = append(convertedTools, mcpTool)
	}

	return convertedTools, nil
}
