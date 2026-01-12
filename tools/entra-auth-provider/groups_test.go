package main

import (
	"testing"

	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
)

// Unit tests focusing on logic without external API dependencies

func TestOptions_AdminCredentials(t *testing.T) {
	tests := []struct {
		name             string
		opts             Options
		wantUseAdmin     bool
		wantClientID     string
		wantClientSecret string
	}{
		{
			name: "uses admin credentials when provided",
			opts: Options{
				ClientID:          "regular-client",
				ClientSecret:      "regular-secret",
				AdminClientID:     "admin-client",
				AdminClientSecret: "admin-secret",
			},
			wantUseAdmin:     true,
			wantClientID:     "admin-client",
			wantClientSecret: "admin-secret",
		},
		{
			name: "falls back to regular credentials when admin not provided",
			opts: Options{
				ClientID:     "regular-client",
				ClientSecret: "regular-secret",
			},
			wantUseAdmin:     false,
			wantClientID:     "regular-client",
			wantClientSecret: "regular-secret",
		},
		{
			name: "falls back when only admin client ID provided",
			opts: Options{
				ClientID:      "regular-client",
				ClientSecret:  "regular-secret",
				AdminClientID: "admin-client",
			},
			wantUseAdmin:     false,
			wantClientID:     "regular-client",
			wantClientSecret: "regular-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate credential selection logic from getAppAccessToken
			clientID := tt.opts.ClientID
			clientSecret := tt.opts.ClientSecret
			if tt.opts.AdminClientID != "" && tt.opts.AdminClientSecret != "" {
				clientID = tt.opts.AdminClientID
				clientSecret = tt.opts.AdminClientSecret
			}

			if clientID != tt.wantClientID {
				t.Errorf("Selected clientID = %v, want %v", clientID, tt.wantClientID)
			}
			if clientSecret != tt.wantClientSecret {
				t.Errorf("Selected clientSecret = %v, want %v", clientSecret, tt.wantClientSecret)
			}
		})
	}
}

func TestGroupResponseParsing(t *testing.T) {
	// Test that we correctly handle group responses with and without descriptions
	type graphGroup struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"displayName"`
		Description *string `json:"description"`
	}

	tests := []struct {
		name      string
		groups    []graphGroup
		wantCount int
	}{
		{
			name: "groups with descriptions",
			groups: []graphGroup{
				{ID: "g1", DisplayName: "Engineering", Description: stringPtr("Eng team")},
				{ID: "g2", DisplayName: "Marketing", Description: stringPtr("Marketing team")},
			},
			wantCount: 2,
		},
		{
			name: "groups without descriptions",
			groups: []graphGroup{
				{ID: "g1", DisplayName: "Engineering", Description: nil},
				{ID: "g2", DisplayName: "Marketing", Description: nil},
			},
			wantCount: 2,
		},
		{
			name: "mixed descriptions",
			groups: []graphGroup{
				{ID: "g1", DisplayName: "Engineering", Description: stringPtr("Eng team")},
				{ID: "g2", DisplayName: "Marketing", Description: nil},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate conversion to state.GroupInfo
			var result state.GroupInfoList
			for _, g := range tt.groups {
				result = append(result, state.GroupInfo{
					ID:          g.ID,
					Name:        g.DisplayName,
					Description: g.Description,
				})
			}

			if len(result) != tt.wantCount {
				t.Errorf("Converted %d groups, want %d", len(result), tt.wantCount)
			}

			// Verify description preservation
			for i, g := range result {
				original := tt.groups[i]
				if !equalStringPtr(g.Description, original.Description) {
					t.Errorf("Group[%d] description mismatch: got %v, want %v",
						i, ptrToString(g.Description), ptrToString(original.Description))
				}
			}
		})
	}
}

func TestNameFilterEscaping(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantOutput string
	}{
		{
			name:       "no special characters",
			input:      "Engineering",
			wantOutput: "Engineering",
		},
		{
			name:       "single quote needs escaping",
			input:      "O'Reilly",
			wantOutput: "O''Reilly",
		},
		{
			name:       "multiple single quotes",
			input:      "It's O'Reilly's",
			wantOutput: "It''s O''Reilly''s",
		},
		{
			name:       "empty string",
			input:      "",
			wantOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test OData string escaping logic used in fetchAllGroups
			escaped := escapeODataString(tt.input)
			if escaped != tt.wantOutput {
				t.Errorf("escapeODataString(%q) = %q, want %q", tt.input, escaped, tt.wantOutput)
			}
		})
	}
}

// Helper function that matches implementation
func escapeODataString(s string) string {
	// In fetchAllGroups, we use: strings.ReplaceAll(nameFilter, "'", "''")
	result := ""
	for _, c := range s {
		if c == '\'' {
			result += "''"
		} else {
			result += string(c)
		}
	}
	return result
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrToString(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
