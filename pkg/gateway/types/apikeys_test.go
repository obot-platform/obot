package types

import (
	"slices"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
)

func TestAPIKeyScopesGroups(t *testing.T) {
	tests := []struct {
		name           string
		scopes         APIKeyScopes
		user           *User
		expectContains []string
		expectMissing  []string
	}{
		{
			name:           "empty scopes only authenticate the key",
			scopes:         APIKeyScopes{},
			expectContains: []string{apitypes.GroupAuthenticated},
			expectMissing:  []string{apitypes.GroupMCP, apitypes.GroupSkills, apitypes.GroupLLM, apitypes.GroupPublishedArtifacts, apitypes.GroupAPI},
		},
		{
			name:           "MCP server IDs grant MCP group",
			scopes:         APIKeyScopes{MCPServerIDs: []string{"server-a"}},
			expectContains: []string{apitypes.GroupMCP, apitypes.GroupAuthenticated},
			expectMissing:  []string{apitypes.GroupSkills, apitypes.GroupLLM, apitypes.GroupPublishedArtifacts},
		},
		{
			name:           "skills scope grants skills group",
			scopes:         APIKeyScopes{CanAccessSkills: true},
			expectContains: []string{apitypes.GroupSkills, apitypes.GroupAuthenticated},
			expectMissing:  []string{apitypes.GroupMCP, apitypes.GroupLLM, apitypes.GroupPublishedArtifacts},
		},
		{
			name:           "LLM scope grants LLM group",
			scopes:         APIKeyScopes{CanAccessLLMProxy: true},
			expectContains: []string{apitypes.GroupLLM, apitypes.GroupAuthenticated},
			expectMissing:  []string{apitypes.GroupMCP, apitypes.GroupSkills, apitypes.GroupPublishedArtifacts},
		},
		{
			name:           "published artifact scope grants published artifacts group",
			scopes:         APIKeyScopes{CanAccessPublishedArtifacts: true},
			expectContains: []string{apitypes.GroupPublishedArtifacts, apitypes.GroupAuthenticated},
			expectMissing:  []string{apitypes.GroupMCP, apitypes.GroupSkills, apitypes.GroupLLM},
		},
		{
			name: "combined scoped key gets each requested capability",
			scopes: APIKeyScopes{
				CanAccessSkills:             true,
				CanAccessLLMProxy:           true,
				CanAccessPublishedArtifacts: true,
				MCPServerIDs:                []string{"*"},
			},
			expectContains: []string{apitypes.GroupSkills, apitypes.GroupLLM, apitypes.GroupPublishedArtifacts, apitypes.GroupMCP, apitypes.GroupAuthenticated},
		},
		{
			name:           "API scope with nil user grants no groups",
			scopes:         APIKeyScopes{CanAccessAPI: true},
			expectContains: nil,
			expectMissing:  []string{apitypes.GroupAuthenticated, apitypes.GroupMCP, apitypes.GroupSkills, apitypes.GroupLLM, apitypes.GroupPublishedArtifacts},
		},
		{
			name:           "API scope inherits user role groups",
			scopes:         APIKeyScopes{CanAccessAPI: true, CanAccessSkills: true, MCPServerIDs: []string{"server-a"}},
			user:           &User{Role: apitypes.RoleBasic},
			expectContains: append(apitypes.RoleBasic.Groups(), apitypes.GroupMCP, apitypes.GroupSkills, apitypes.GroupLLM, apitypes.GroupPublishedArtifacts),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := tt.scopes.Groups(tt.user)
			for _, expected := range tt.expectContains {
				if !slices.Contains(groups, expected) {
					t.Fatalf("Groups() missing %q, got %v", expected, groups)
				}
			}
			for _, unexpected := range tt.expectMissing {
				if slices.Contains(groups, unexpected) {
					t.Fatalf("Groups() unexpectedly contains %q, got %v", unexpected, groups)
				}
			}
		})
	}
}
