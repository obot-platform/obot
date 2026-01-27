package render

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestStringAppend(t *testing.T) {
	tests := []struct {
		name     string
		first    string
		second   []string
		expected string
	}{
		{
			name:     "both empty",
			first:    "",
			second:   []string{},
			expected: "",
		},
		{
			name:     "first empty with single second",
			first:    "",
			second:   []string{"second"},
			expected: "second",
		},
		{
			name:     "first empty with multiple second",
			first:    "",
			second:   []string{"second", "third"},
			expected: "second\n\nthird",
		},
		{
			name:     "first non-empty with empty second",
			first:    "first",
			second:   []string{},
			expected: "first",
		},
		{
			name:     "first non-empty with single second",
			first:    "first",
			second:   []string{"second"},
			expected: "first\n\nsecond",
		},
		{
			name:     "first non-empty with multiple second",
			first:    "first",
			second:   []string{"second", "third"},
			expected: "first\n\nsecond\n\nthird",
		},
		{
			name:     "first non-empty with nil second",
			first:    "first",
			second:   nil,
			expected: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringAppend(tt.first, tt.second...)
			if result != tt.expected {
				t.Errorf("stringAppend(%q, %v) = %q, want %q", tt.first, tt.second, result, tt.expected)
			}
		})
	}
}

func TestMergeWebsiteKnowledge(t *testing.T) {
	tests := []struct {
		name               string
		websiteKnowledge   []*types.WebsiteKnowledge
		expectedSiteTool   string
		expectedSitesCount int
	}{
		{
			name:               "all nil",
			websiteKnowledge:   []*types.WebsiteKnowledge{nil, nil},
			expectedSiteTool:   "",
			expectedSitesCount: 0,
		},
		{
			name: "single knowledge with site tool",
			websiteKnowledge: []*types.WebsiteKnowledge{
				{
					SiteTool: "mytool",
					Sites: []types.WebsiteDefinition{
						{Site: "https://example.com"},
					},
				},
			},
			expectedSiteTool:   "mytool",
			expectedSitesCount: 1,
		},
		{
			name: "multiple knowledge with last override",
			websiteKnowledge: []*types.WebsiteKnowledge{
				{
					SiteTool: "tool1",
					Sites: []types.WebsiteDefinition{
						{Site: "https://example1.com"},
					},
				},
				{
					SiteTool: "tool2",
					Sites: []types.WebsiteDefinition{
						{Site: "https://example2.com"},
					},
				},
			},
			expectedSiteTool:   "tool2",
			expectedSitesCount: 2,
		},
		{
			name: "filter empty sites",
			websiteKnowledge: []*types.WebsiteKnowledge{
				{
					SiteTool: "mytool",
					Sites: []types.WebsiteDefinition{
						{Site: "https://example.com"},
						{Site: ""},
						{Site: "  "},
						{Site: "https://example2.com"},
					},
				},
			},
			expectedSiteTool:   "mytool",
			expectedSitesCount: 2,
		},
		{
			name: "mixed nil and non-nil",
			websiteKnowledge: []*types.WebsiteKnowledge{
				nil,
				{
					SiteTool: "tool1",
					Sites: []types.WebsiteDefinition{
						{Site: "https://example.com"},
					},
				},
				nil,
			},
			expectedSiteTool:   "tool1",
			expectedSitesCount: 1,
		},
		{
			name: "empty site tool uses first non-empty",
			websiteKnowledge: []*types.WebsiteKnowledge{
				{
					SiteTool: "",
					Sites: []types.WebsiteDefinition{
						{Site: "https://example1.com"},
					},
				},
				{
					SiteTool: "tool2",
					Sites: []types.WebsiteDefinition{
						{Site: "https://example2.com"},
					},
				},
			},
			expectedSiteTool:   "tool2",
			expectedSitesCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeWebsiteKnowledge(tt.websiteKnowledge...)
			if result.SiteTool != tt.expectedSiteTool {
				t.Errorf("mergeWebsiteKnowledge() SiteTool = %q, want %q", result.SiteTool, tt.expectedSiteTool)
			}
			if len(result.Sites) != tt.expectedSitesCount {
				t.Errorf("mergeWebsiteKnowledge() Sites count = %d, want %d", len(result.Sites), tt.expectedSitesCount)
			}
		})
	}
}

func TestManifestToTool(t *testing.T) {
	tests := []struct {
		name               string
		manifest           types.WorkflowManifest
		taskInvoke         string
		id                 string
		expectedName       string
		expectedDescPrefix string
		expectedChat       bool
	}{
		{
			name: "basic manifest",
			manifest: types.WorkflowManifest{
				Name:        "MyTask",
				Description: "Does something",
			},
			taskInvoke:         "sys.task.invoke",
			id:                 "task-123",
			expectedName:       "Task MyTask",
			expectedDescPrefix: "Task: Does something",
			expectedChat:       true,
		},
		{
			name: "manifest with spaces in name",
			manifest: types.WorkflowManifest{
				Name:        "  MyTask  ",
				Description: "Does something",
			},
			taskInvoke:         "sys.task.invoke",
			id:                 "task-456",
			expectedName:       "Task MyTask",
			expectedDescPrefix: "Task: Does something",
			expectedChat:       true,
		},
		{
			name: "manifest without description",
			manifest: types.WorkflowManifest{
				Name:        "MyTask",
				Description: "",
			},
			taskInvoke:         "sys.task.invoke",
			id:                 "task-789",
			expectedName:       "Task MyTask",
			expectedDescPrefix: "Invokes task named MyTask",
			expectedChat:       true,
		},
		{
			name: "manifest with params",
			manifest: types.WorkflowManifest{
				Name:        "TaskWithParams",
				Description: "Task with parameters",
				Params: map[string]string{
					"param1": "First parameter",
					"param2": "Second parameter",
				},
			},
			taskInvoke:         "sys.task.invoke",
			id:                 "task-abc",
			expectedName:       "Task TaskWithParams",
			expectedDescPrefix: "Task: Task with parameters",
			expectedChat:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manifestToTool(tt.manifest, tt.taskInvoke, tt.id)

			if result.Name != tt.expectedName {
				t.Errorf("manifestToTool() Name = %q, want %q", result.Name, tt.expectedName)
			}

			if !contains(result.Description, tt.expectedDescPrefix) {
				t.Errorf("manifestToTool() Description = %q, want to contain %q", result.Description, tt.expectedDescPrefix)
			}

			if result.Chat != tt.expectedChat {
				t.Errorf("manifestToTool() Chat = %v, want %v", result.Chat, tt.expectedChat)
			}

			if len(result.Tools) != 1 || result.Tools[0] != tt.taskInvoke {
				t.Errorf("manifestToTool() Tools = %v, want [%s]", result.Tools, tt.taskInvoke)
			}

			// Check that instructions contain the task invoke and id
			if !contains(result.Instructions, tt.taskInvoke) {
				t.Errorf("manifestToTool() Instructions = %q, want to contain %q", result.Instructions, tt.taskInvoke)
			}
			if !contains(result.Instructions, tt.id) {
				t.Errorf("manifestToTool() Instructions = %q, want to contain %q", result.Instructions, tt.id)
			}
		})
	}
}
