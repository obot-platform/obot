package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyToolOverrides(t *testing.T) {
	tests := []struct {
		name          string
		tools         []types.MCPServerTool
		toolOverrides []types.ToolOverride
		expected      []types.MCPServerTool
	}{
		{
			name: "no overrides - tools unchanged",
			tools: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
				{Name: "list-repos", Description: "Lists repositories"},
			},
			toolOverrides: nil,
			expected: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
				{Name: "list-repos", Description: "Lists repositories"},
			},
		},
		{
			name: "disable tool - filtered out",
			tools: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
				{Name: "delete-repo", Description: "Deletes a repository"},
			},
			toolOverrides: []types.ToolOverride{
				{Name: "create-issue", Enabled: true},
				{Name: "delete-repo", Enabled: false},
			},
			expected: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
				// delete-repo is filtered out because Enabled: false
			},
		},
		{
			name: "rename tool",
			tools: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
			},
			toolOverrides: []types.ToolOverride{
				{Name: "create-issue", OverrideName: "new-issue", Enabled: true},
			},
			expected: []types.MCPServerTool{
				{Name: "new-issue", Description: "Creates an issue"},
			},
		},
		{
			name: "override description",
			tools: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
			},
			toolOverrides: []types.ToolOverride{
				{Name: "create-issue", OverrideDescription: "Custom description", Enabled: true},
			},
			expected: []types.MCPServerTool{
				{Name: "create-issue", Description: "Custom description"},
			},
		},
		{
			name: "rename and override description",
			tools: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
			},
			toolOverrides: []types.ToolOverride{
				{
					Name:                "create-issue",
					OverrideName:        "new-issue",
					OverrideDescription: "Custom description",
					Enabled:             true,
				},
			},
			expected: []types.MCPServerTool{
				{Name: "new-issue", Description: "Custom description"},
			},
		},
		{
			name: "allowlist only overridden tools included",
			tools: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
				{Name: "delete-repo", Description: "Deletes a repository"},
				{Name: "list-repos", Description: "Lists repositories"},
			},
			toolOverrides: []types.ToolOverride{
				{Name: "create-issue", OverrideName: "new-issue", Enabled: true},
				{Name: "delete-repo", Enabled: false},
			},
			expected: []types.MCPServerTool{
				{Name: "new-issue", Description: "Creates an issue"},
				// list-repos is excluded because it's not in the override list
			},
		},
		{
			name: "allowlist with enabled only",
			tools: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
				{Name: "list-repos", Description: "Lists repositories"},
				{Name: "delete-repo", Description: "Deletes a repository"},
				{Name: "update-issue", Description: "Updates an issue"},
			},
			toolOverrides: []types.ToolOverride{
				{Name: "create-issue", Enabled: true},
				{Name: "list-repos", Enabled: true},
			},
			expected: []types.MCPServerTool{
				{Name: "create-issue", Description: "Creates an issue"},
				{Name: "list-repos", Description: "Lists repositories"},
				// delete-repo and update-issue are excluded because they're not in the override list
			},
		},
		{
			name:  "empty tools array",
			tools: []types.MCPServerTool{},
			toolOverrides: []types.ToolOverride{
				{Name: "some-tool", Enabled: false},
			},
			expected: []types.MCPServerTool{},
		},
		{
			name: "tools with params preserved",
			tools: []types.MCPServerTool{
				{
					Name:        "create-issue",
					Description: "Creates an issue",
					Params: map[string]string{
						"title": "Issue title",
						"body":  "Issue body",
					},
				},
			},
			toolOverrides: []types.ToolOverride{
				{Name: "create-issue", OverrideName: "new-issue", Enabled: true},
			},
			expected: []types.MCPServerTool{
				{
					Name:        "new-issue",
					Description: "Creates an issue",
					Params: map[string]string{
						"title": "Issue title",
						"body":  "Issue body",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyToolOverrides(tt.tools, tt.toolOverrides)
			require.Equal(t, len(tt.expected), len(result), "Tool count mismatch")
			for i := range tt.expected {
				assert.Equal(t, tt.expected[i].Name, result[i].Name, "Tool name mismatch at index %d", i)
				assert.Equal(t, tt.expected[i].Description, result[i].Description, "Tool description mismatch at index %d", i)
				assert.Equal(t, tt.expected[i].Params, result[i].Params, "Tool params mismatch at index %d", i)
			}
		})
	}
}
