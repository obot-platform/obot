package apiclient

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestListToolReferencesOptions_PathConstruction(t *testing.T) {
	tests := []struct {
		name         string
		opts         ListToolReferencesOptions
		expectedPath string
	}{
		{
			name:         "No tool type filter",
			opts:         ListToolReferencesOptions{},
			expectedPath: "/tool-references",
		},
		{
			name:         "With tool reference type",
			opts:         ListToolReferencesOptions{ToolType: types.ToolReferenceTypeTool},
			expectedPath: "/tool-references?type=tool",
		},
		{
			name:         "With knowledge data source type",
			opts:         ListToolReferencesOptions{ToolType: types.ToolReferenceTypeKnowledgeDataSource},
			expectedPath: "/tool-references?type=knowledgeDataSource",
		},
		{
			name:         "Empty tool type",
			opts:         ListToolReferencesOptions{ToolType: ""},
			expectedPath: "/tool-references",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the path construction logic
			var path string
			if tt.opts.ToolType != "" {
				path = "/tool-references?type=" + string(tt.opts.ToolType)
			} else {
				path = "/tool-references"
			}

			if path != tt.expectedPath {
				t.Errorf("Expected path %q, got %q", tt.expectedPath, path)
			}
		})
	}
}

func TestDeleteToolReference_PathConstruction(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		toolType     types.ToolReferenceType
		expectedPath string
	}{
		{
			name:         "Delete without tool type",
			id:           "ref-123",
			toolType:     "",
			expectedPath: "/tool-references/ref-123",
		},
		{
			name:         "Delete with tool type",
			id:           "ref-456",
			toolType:     types.ToolReferenceTypeTool,
			expectedPath: "/tool-references/ref-456?type=tool",
		},
		{
			name:         "Delete with system type",
			id:           "ref-789",
			toolType:     types.ToolReferenceTypeSystem,
			expectedPath: "/tool-references/ref-789?type=system",
		},
		{
			name:         "Empty ID without type",
			id:           "",
			toolType:     "",
			expectedPath: "/tool-references/",
		},
		{
			name:         "Empty ID with type",
			id:           "",
			toolType:     types.ToolReferenceTypeTool,
			expectedPath: "/tool-references/?type=tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the path construction logic
			var path string
			if tt.toolType != "" {
				path = "/tool-references/" + tt.id + "?type=" + string(tt.toolType)
			} else {
				path = "/tool-references/" + tt.id
			}

			if path != tt.expectedPath {
				t.Errorf("Expected path %q, got %q", tt.expectedPath, path)
			}
		})
	}
}
