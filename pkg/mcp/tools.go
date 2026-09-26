package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	otypes "github.com/obot-platform/obot/apiclient/types"
)

func (sm *SessionManager) ListTools(ctx context.Context, serverConfig ServerConfig) ([]*gomcp.Tool, error) {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	resp, err := client.ListTools(ctx, &gomcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP tools: %w", err)
	}

	return resp.Tools, nil
}

func ConvertTools(tools []*gomcp.Tool, unsupportedTools []string) ([]otypes.MCPServerTool, error) {
	convertedTools := make([]otypes.MCPServerTool, 0, len(tools))
	for _, t := range tools {
		mcpTool := otypes.MCPServerTool{
			ID:          t.Name,
			Name:        t.Name,
			Description: t.Description,
			Enabled:     !slices.Contains(unsupportedTools, t.Name),
			Unsupported: slices.Contains(unsupportedTools, t.Name),
		}

		if t.InputSchema != nil {
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
