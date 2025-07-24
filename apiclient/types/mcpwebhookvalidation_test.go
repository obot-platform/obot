package types

import "testing"

func TestMCPFilters_Matches(t *testing.T) {
	tests := []struct {
		name          string
		filters       MCPFilters
		method        string
		identifier    string
		expectedMatch bool
	}{
		// Test nil/empty filters
		{
			name:          "nil filters matches everything",
			filters:       nil,
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name:          "empty filters doesn't match",
			filters:       MCPFilters{},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: false,
		},

		// Test wildcard method
		{
			name: "wildcard method matches all methods",
			filters: MCPFilters{
				{Method: "*"},
			},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name: "wildcard method matches initialized",
			filters: MCPFilters{
				{Method: "*"},
			},
			method:        "initialized",
			identifier:    "",
			expectedMatch: true,
		},

		// Test specific methods
		{
			name: "exact method match - initialized",
			filters: MCPFilters{
				{Method: "initialized"},
			},
			method:        "initialized",
			identifier:    "",
			expectedMatch: true,
		},
		{
			name: "exact method match - tools/list",
			filters: MCPFilters{
				{Method: "tools/list"},
			},
			method:        "tools/list",
			identifier:    "",
			expectedMatch: true,
		},
		{
			name: "exact method match - tools/call",
			filters: MCPFilters{
				{Method: "tools/call"},
			},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name: "exact method match - resources/list",
			filters: MCPFilters{
				{Method: "resources/list"},
			},
			method:        "resources/list",
			identifier:    "",
			expectedMatch: true,
		},
		{
			name: "exact method match - resources/read",
			filters: MCPFilters{
				{Method: "resources/read"},
			},
			method:        "resources/read",
			identifier:    "resource1",
			expectedMatch: true,
		},
		{
			name: "exact method match - prompts/list",
			filters: MCPFilters{
				{Method: "prompts/list"},
			},
			method:        "prompts/list",
			identifier:    "",
			expectedMatch: true,
		},
		{
			name: "exact method match - prompts/get",
			filters: MCPFilters{
				{Method: "prompts/get"},
			},
			method:        "prompts/get",
			identifier:    "prompt1",
			expectedMatch: true,
		},

		// Test method mismatch
		{
			name: "method mismatch",
			filters: MCPFilters{
				{Method: "tools/call"},
			},
			method:        "tools/list",
			identifier:    "",
			expectedMatch: false,
		},

		// Test identifiers with wildcard
		{
			name: "wildcard identifier matches any",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"*"}},
			},
			method:        "tools/call",
			identifier:    "any-tool",
			expectedMatch: true,
		},
		{
			name: "wildcard identifier with empty identifier",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"*"}},
			},
			method:        "tools/call",
			identifier:    "",
			expectedMatch: true,
		},

		// Test specific identifiers
		{
			name: "exact identifier match",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"tool1", "tool2"}},
			},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name: "identifier in list",
			filters: MCPFilters{
				{Method: "resources/read", Identifiers: []string{"resource1", "resource2"}},
			},
			method:        "resources/read",
			identifier:    "resource2",
			expectedMatch: true,
		},
		{
			name: "identifier not in list",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"tool1", "tool2"}},
			},
			method:        "tools/call",
			identifier:    "tool3",
			expectedMatch: false,
		},

		// Test empty identifier parameter
		{
			name: "empty identifier matches when identifiers specified",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"tool1"}},
			},
			method:        "tools/call",
			identifier:    "",
			expectedMatch: true,
		},

		// Test nil identifiers (matches everything)
		{
			name: "nil identifiers matches any identifier",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: nil},
			},
			method:        "tools/call",
			identifier:    "any-tool",
			expectedMatch: true,
		},
		{
			name: "nil identifiers matches empty identifier",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: nil},
			},
			method:        "tools/call",
			identifier:    "",
			expectedMatch: true,
		},

		// Test multiple filters - should match if any filter matches
		{
			name: "multiple filters - first matches",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"tool1"}},
				{Method: "resources/read", Identifiers: []string{"resource1"}},
			},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name: "multiple filters - second matches",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"tool1"}},
				{Method: "resources/read", Identifiers: []string{"resource1"}},
			},
			method:        "resources/read",
			identifier:    "resource1",
			expectedMatch: true,
		},
		{
			name: "multiple filters - none match",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"tool1"}},
				{Method: "resources/read", Identifiers: []string{"resource1"}},
			},
			method:        "prompts/get",
			identifier:    "prompt1",
			expectedMatch: false,
		},

		// Test edge cases
		{
			name: "method matches but identifier doesn't",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"tool1"}},
			},
			method:        "tools/call",
			identifier:    "tool2",
			expectedMatch: false,
		},
		{
			name: "mixed wildcard and specific identifiers",
			filters: MCPFilters{
				{Method: "tools/call", Identifiers: []string{"*", "tool1"}},
			},
			method:        "tools/call",
			identifier:    "any-tool",
			expectedMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filters.Matches(tt.method, tt.identifier)
			if result != tt.expectedMatch {
				t.Errorf("MCPFilters.Matches(%q, %q) = %v, expected %v", tt.method, tt.identifier, result, tt.expectedMatch)
			}
		})
	}
}

func TestMCPFilter_Matches(t *testing.T) {
	tests := []struct {
		name          string
		filter        MCPFilter
		method        string
		identifier    string
		expectedMatch bool
	}{
		// Test wildcard method
		{
			name:          "wildcard method matches any",
			filter:        MCPFilter{Method: "*"},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name:          "wildcard method with identifiers",
			filter:        MCPFilter{Method: "*", Identifiers: []string{"tool1"}},
			method:        "any/method",
			identifier:    "tool1",
			expectedMatch: true,
		},

		// Test exact method matching
		{
			name:          "exact method match",
			filter:        MCPFilter{Method: "tools/call"},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name:          "method mismatch",
			filter:        MCPFilter{Method: "tools/call"},
			method:        "tools/list",
			identifier:    "tool1",
			expectedMatch: false,
		},

		// Test identifier matching
		{
			name:          "wildcard identifier",
			filter:        MCPFilter{Method: "tools/call", Identifiers: []string{"*"}},
			method:        "tools/call",
			identifier:    "any-tool",
			expectedMatch: true,
		},
		{
			name:          "exact identifier match",
			filter:        MCPFilter{Method: "tools/call", Identifiers: []string{"tool1"}},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name:          "identifier in list",
			filter:        MCPFilter{Method: "tools/call", Identifiers: []string{"tool1", "tool2"}},
			method:        "tools/call",
			identifier:    "tool2",
			expectedMatch: true,
		},
		{
			name:          "identifier not in list",
			filter:        MCPFilter{Method: "tools/call", Identifiers: []string{"tool1", "tool2"}},
			method:        "tools/call",
			identifier:    "tool3",
			expectedMatch: false,
		},
		{
			name:          "empty identifier matches when in list",
			filter:        MCPFilter{Method: "tools/call", Identifiers: []string{"tool1"}},
			method:        "tools/call",
			identifier:    "",
			expectedMatch: true,
		},

		// Test nil identifiers (matches everything)
		{
			name:          "nil identifiers matches any",
			filter:        MCPFilter{Method: "tools/call", Identifiers: nil},
			method:        "tools/call",
			identifier:    "any-tool",
			expectedMatch: true,
		},
		{
			name:          "nil identifiers matches empty",
			filter:        MCPFilter{Method: "tools/call", Identifiers: nil},
			method:        "tools/call",
			identifier:    "",
			expectedMatch: true,
		},

		// Test all supported methods
		{
			name:          "initialized method",
			filter:        MCPFilter{Method: "initialized"},
			method:        "initialized",
			identifier:    "",
			expectedMatch: true,
		},
		{
			name:          "tools/list method",
			filter:        MCPFilter{Method: "tools/list"},
			method:        "tools/list",
			identifier:    "",
			expectedMatch: true,
		},
		{
			name:          "tools/call method",
			filter:        MCPFilter{Method: "tools/call"},
			method:        "tools/call",
			identifier:    "tool1",
			expectedMatch: true,
		},
		{
			name:          "resources/list method",
			filter:        MCPFilter{Method: "resources/list"},
			method:        "resources/list",
			identifier:    "",
			expectedMatch: true,
		},
		{
			name:          "resources/read method",
			filter:        MCPFilter{Method: "resources/read"},
			method:        "resources/read",
			identifier:    "resource1",
			expectedMatch: true,
		},
		{
			name:          "prompts/list method",
			filter:        MCPFilter{Method: "prompts/list"},
			method:        "prompts/list",
			identifier:    "",
			expectedMatch: true,
		},
		{
			name:          "prompts/get method",
			filter:        MCPFilter{Method: "prompts/get"},
			method:        "prompts/get",
			identifier:    "prompt1",
			expectedMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.Matches(tt.method, tt.identifier)
			if result != tt.expectedMatch {
				t.Errorf("MCPFilter.Matches(%q, %q) = %v, expected %v", tt.method, tt.identifier, result, tt.expectedMatch)
			}
		})
	}
}
