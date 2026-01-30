package invoke

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsApprovedTool(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		approvedTools []string
		expected      bool
	}{
		{
			name:          "exact match",
			toolName:      "myTool",
			approvedTools: []string{"myTool"},
			expected:      true,
		},
		{
			name:          "no match",
			toolName:      "myTool",
			approvedTools: []string{"otherTool"},
			expected:      false,
		},
		{
			name:          "empty approved list",
			toolName:      "myTool",
			approvedTools: nil,
			expected:      false,
		},
		{
			name:          "wildcard matches all",
			toolName:      "anything",
			approvedTools: []string{"*"},
			expected:      true,
		},
		{
			name:          "prefix wildcard match",
			toolName:      "fooBar",
			approvedTools: []string{"foo*"},
			expected:      true,
		},
		{
			name:          "prefix wildcard no match",
			toolName:      "barBaz",
			approvedTools: []string{"foo*"},
			expected:      false,
		},
		{
			name:          "multiple entries match later",
			toolName:      "baz",
			approvedTools: []string{"foo", "bar", "baz"},
			expected:      true,
		},
		{
			name:          "empty tool name",
			toolName:      "",
			approvedTools: []string{"foo"},
			expected:      false,
		},
		{
			name:          "wildcard with empty tool name",
			toolName:      "",
			approvedTools: []string{"*"},
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isApprovedTool(tt.toolName, tt.approvedTools))
		})
	}
}
