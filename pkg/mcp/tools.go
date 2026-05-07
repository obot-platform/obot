package mcp

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/obot-platform/nanobot/pkg/mcp"
	otypes "github.com/obot-platform/obot/apiclient/types"
)

func (sm *SessionManager) ListTools(ctx context.Context, serverConfig ServerConfig) ([]mcp.Tool, error) {
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

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

// ApplyToolOverrides applies ToolOverrides to a component's tool array,
// filtering out disabled tools and applying name/description overrides.
// If overrides are present, they act as an allowlist - only tools explicitly listed are included.
// toolPrefix, if non-empty, is prepended to every returned tool's Name so previews
// match what the composite server will expose via nanobot at runtime.
func ApplyToolOverrides(tools []otypes.MCPServerTool, toolOverrides []otypes.ToolOverride, toolPrefix string) []otypes.MCPServerTool {
	// Build lookup map: toolName -> ToolOverride
	overrideMap := make(map[string]otypes.ToolOverride, len(toolOverrides))
	for _, override := range toolOverrides {
		overrideMap[override.Name] = override
	}

	var (
		hasOverrides     = len(toolOverrides) > 0
		transformedTools = make([]otypes.MCPServerTool, 0, len(tools))
	)
	for _, tool := range tools {
		override, hasOverride := overrideMap[tool.Name]
		if hasOverrides && (!hasOverride || !override.Enabled) {
			// Omit the tool from the final tool set.
			// Overrides have been set for the component and the tool either:
			// - isn't present in the component's overrides (is likely net-new and wasn't available when the overrides were generated)
			// - is explicitly disabled
			continue
		}

		// Apply overrides and tool prefix if provided
		tool.Name = toolPrefix + cmp.Or(override.OverrideName, tool.Name)
		tool.Description = cmp.Or(override.OverrideDescription, tool.Description)

		transformedTools = append(transformedTools, tool)
	}

	return transformedTools
}
