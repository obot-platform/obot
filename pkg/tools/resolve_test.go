package tools

import (
	"testing"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
)

func TestResolveToolReferenceName(t *testing.T) {
	tests := []struct {
		name         string
		toolType     types.ToolReferenceType
		isBundle     bool
		isCapability bool
		toolName     string
		subToolName  string
		want         string
	}{
		{
			name:        "tool type - bundle - not capability",
			toolType:    types.ToolReferenceTypeTool,
			isBundle:    true,
			isCapability: false,
			toolName:    "my-tool",
			subToolName: "",
			want:        "my-tool-bundle",
		},
		{
			name:        "tool type - bundle - is capability",
			toolType:    types.ToolReferenceTypeTool,
			isBundle:    true,
			isCapability: true,
			toolName:    "my-tool",
			subToolName: "",
			want:        "my-tool",
		},
		{
			name:        "tool type - not bundle - no subtool",
			toolType:    types.ToolReferenceTypeTool,
			isBundle:    false,
			isCapability: false,
			toolName:    "my-tool",
			subToolName: "",
			want:        "my-tool",
		},
		{
			name:        "tool type - not bundle - with subtool",
			toolType:    types.ToolReferenceTypeTool,
			isBundle:    false,
			isCapability: false,
			toolName:    "my-tool",
			subToolName: "SubTool",
			want:        "my-tool-subtool",
		},
		{
			name:        "tool type - not bundle - with subtool containing spaces",
			toolType:    types.ToolReferenceTypeTool,
			isBundle:    false,
			isCapability: false,
			toolName:    "My Tool",
			subToolName: "Sub Tool Name",
			want:        "my-tool-sub-tool-name",
		},
		{
			name:        "tool type - not bundle - with subtool containing underscores",
			toolType:    types.ToolReferenceTypeTool,
			isBundle:    false,
			isCapability: false,
			toolName:    "my_tool",
			subToolName: "sub_tool_name",
			want:        "my-tool-sub-tool-name",
		},
		{
			name:        "non-tool type - returns toolName as-is",
			toolType:    types.ToolReferenceType("agent"),
			isBundle:    true,
			isCapability: true,
			toolName:    "my-agent",
			subToolName: "ignored",
			want:        "my-agent",
		},
		{
			name:        "non-tool type - workspace",
			toolType:    types.ToolReferenceType("workspace"),
			isBundle:    false,
			isCapability: false,
			toolName:    "my-workspace",
			subToolName: "ignored",
			want:        "my-workspace",
		},
		{
			name:        "empty subtool name treated as no subtool",
			toolType:    types.ToolReferenceTypeTool,
			isBundle:    false,
			isCapability: false,
			toolName:    "my-tool",
			subToolName: "",
			want:        "my-tool",
		},
		{
			name:        "mixed spaces and underscores in names",
			toolType:    types.ToolReferenceTypeTool,
			isBundle:    false,
			isCapability: false,
			toolName:    "My_Tool Name",
			subToolName: "Sub_Tool Name",
			want:        "my-tool-name-sub-tool-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveToolReferenceName(tt.toolType, tt.isBundle, tt.isCapability, tt.toolName, tt.subToolName)
			if got != tt.want {
				t.Errorf("resolveToolReferenceName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		want  string
	}{
		{
			name:  "single name - no changes needed",
			names: []string{"tool"},
			want:  "tool",
		},
		{
			name:  "single name - uppercase",
			names: []string{"MyTool"},
			want:  "mytool",
		},
		{
			name:  "single name - with spaces",
			names: []string{"My Tool"},
			want:  "my-tool",
		},
		{
			name:  "single name - with underscores",
			names: []string{"my_tool"},
			want:  "my-tool",
		},
		{
			name:  "single name - mixed spaces and underscores",
			names: []string{"My_Tool Name"},
			want:  "my-tool-name",
		},
		{
			name:  "two names joined with dash",
			names: []string{"tool", "subtool"},
			want:  "tool-subtool",
		},
		{
			name:  "two names - uppercase",
			names: []string{"MyTool", "SubTool"},
			want:  "mytool-subtool",
		},
		{
			name:  "two names - with spaces",
			names: []string{"My Tool", "Sub Tool"},
			want:  "my-tool-sub-tool",
		},
		{
			name:  "two names - with underscores",
			names: []string{"my_tool", "sub_tool"},
			want:  "my-tool-sub-tool",
		},
		{
			name:  "three names",
			names: []string{"my", "tool", "name"},
			want:  "my-tool-name",
		},
		{
			name:  "empty strings included",
			names: []string{"", "tool", ""},
			want:  "-tool-",
		},
		{
			name:  "only spaces",
			names: []string{"   "},
			want:  "---",
		},
		{
			name:  "only underscores",
			names: []string{"___"},
			want:  "---",
		},
		{
			name:  "mixed case with special characters",
			names: []string{"MyTool_Name With Spaces"},
			want:  "mytool-name-with-spaces",
		},
		{
			name:  "already normalized",
			names: []string{"my-tool-name"},
			want:  "my-tool-name",
		},
		{
			name:  "consecutive spaces",
			names: []string{"tool  name"},
			want:  "tool--name",
		},
		{
			name:  "consecutive underscores",
			names: []string{"tool__name"},
			want:  "tool--name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.names...)
			if got != tt.want {
				t.Errorf("normalize(%v) = %v, want %v", tt.names, got, tt.want)
			}
		})
	}
}

func TestIsValidTool(t *testing.T) {
	tests := []struct {
		name string
		tool gptscript.Tool
		want bool
	}{
		{
			name: "valid tool - basic",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-tool",
					Type: "tool",
				},
			},
			want: true,
		},
		{
			name: "valid tool - empty type defaults to tool",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-tool",
					Type: "",
				},
			},
			want: true,
		},
		{
			name: "invalid tool - index false",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-tool",
					Type: "tool",
					MetaData: map[string]string{
						"index": "false",
					},
				},
			},
			want: false,
		},
		{
			name: "invalid tool - empty name",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "",
					Type: "tool",
				},
			},
			want: false,
		},
		{
			name: "invalid tool - non-tool type",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-context",
					Type: "context",
				},
			},
			want: false,
		},
		{
			name: "valid tool - with metadata but index not false",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-tool",
					Type: "tool",
					MetaData: map[string]string{
						"category": "helper",
					},
				},
			},
			want: true,
		},
		{
			name: "valid tool - index explicitly true",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-tool",
					Type: "tool",
					MetaData: map[string]string{
						"index": "true",
					},
				},
			},
			want: true,
		},
		{
			name: "invalid tool - empty name with empty type",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "",
					Type: "",
				},
			},
			want: false,
		},
		{
			name: "valid tool - name with spaces",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "My Tool Name",
					Type: "",
				},
			},
			want: true,
		},
		{
			name: "invalid tool - whitespace-only name",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "   ",
					Type: "tool",
				},
			},
			want: true, // whitespace is technically not empty
		},
		{
			name: "invalid tool - index false with empty type",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-tool",
					Type: "",
					MetaData: map[string]string{
						"index": "false",
					},
				},
			},
			want: false,
		},
		{
			name: "invalid tool - credential type",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-cred",
					Type: "credential",
				},
			},
			want: false,
		},
		{
			name: "invalid tool - context type",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-context",
					Type: "context",
				},
			},
			want: false,
		},
		{
			name: "valid tool - complex metadata",
			tool: gptscript.Tool{
				ToolDef: gptscript.ToolDef{
					Name: "my-tool",
					Type: "tool",
					MetaData: map[string]string{
						"category":    "Capability",
						"description": "A useful tool",
						"version":     "1.0.0",
						"index":       "true",
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidTool(tt.tool)
			if got != tt.want {
				t.Errorf("isValidTool() = %v, want %v", got, tt.want)
			}
		})
	}
}
