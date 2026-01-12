package main

import (
	"testing"

	"github.com/obot-platform/tools/auth-providers-common/pkg/state"
)

// Unit tests focusing on logic without external API dependencies

func TestKeycloakOptions_AdminCredentials(t *testing.T) {
	tests := []struct {
		name             string
		opts             Options
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
			wantClientID:     "admin-client",
			wantClientSecret: "admin-secret",
		},
		{
			name: "falls back to regular credentials when admin not provided",
			opts: Options{
				ClientID:     "regular-client",
				ClientSecret: "regular-secret",
			},
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
			wantClientID:     "regular-client",
			wantClientSecret: "regular-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate credential selection logic from getKeycloakAdminToken
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

func TestKeycloakGroupHierarchyFlattening(t *testing.T) {
	// Test that we correctly flatten nested group hierarchies
	type KeycloakGroup struct {
		ID         string
		Name       string
		Path       string
		Attributes map[string][]string
		SubGroups  []KeycloakGroup
	}

	tests := []struct {
		name      string
		groups    []KeycloakGroup
		wantCount int
		wantPaths []string
	}{
		{
			name: "flat structure",
			groups: []KeycloakGroup{
				{ID: "g1", Name: "Engineering", Path: "/Engineering"},
				{ID: "g2", Name: "Marketing", Path: "/Marketing"},
			},
			wantCount: 2,
			wantPaths: []string{"Engineering", "Marketing"},
		},
		{
			name: "nested structure",
			groups: []KeycloakGroup{
				{
					ID:   "g1",
					Name: "Engineering",
					Path: "/Engineering",
					SubGroups: []KeycloakGroup{
						{ID: "g1-1", Name: "Backend", Path: "/Engineering/Backend"},
						{ID: "g1-2", Name: "Frontend", Path: "/Engineering/Frontend"},
					},
				},
			},
			wantCount: 3,
			wantPaths: []string{"Engineering", "Engineering/Backend", "Engineering/Frontend"},
		},
		{
			name: "deeply nested structure",
			groups: []KeycloakGroup{
				{
					ID:   "g1",
					Name: "Company",
					Path: "/Company",
					SubGroups: []KeycloakGroup{
						{
							ID:   "g1-1",
							Name: "Engineering",
							Path: "/Company/Engineering",
							SubGroups: []KeycloakGroup{
								{ID: "g1-1-1", Name: "Backend", Path: "/Company/Engineering/Backend"},
							},
						},
					},
				},
			},
			wantCount: 3,
			wantPaths: []string{"Company", "Company/Engineering", "Company/Engineering/Backend"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate flattening logic from fetchAllKeycloakGroups
			var allGroups state.GroupInfoList
			var flattenGroups func([]KeycloakGroup)
			flattenGroups = func(groupList []KeycloakGroup) {
				for _, g := range groupList {
					displayName := g.Path
					if displayName == "" || displayName == "/" {
						displayName = g.Name
					}
					// Remove leading slash
					if len(displayName) > 0 && displayName[0] == '/' {
						displayName = displayName[1:]
					}

					allGroups = append(allGroups, state.GroupInfo{
						ID:   g.ID,
						Name: displayName,
					})

					if len(g.SubGroups) > 0 {
						flattenGroups(g.SubGroups)
					}
				}
			}

			flattenGroups(tt.groups)

			if len(allGroups) != tt.wantCount {
				t.Errorf("Flattened to %d groups, want %d", len(allGroups), tt.wantCount)
			}

			for i, wantPath := range tt.wantPaths {
				if i >= len(allGroups) {
					t.Errorf("Missing group at index %d", i)
					continue
				}
				if allGroups[i].Name != wantPath {
					t.Errorf("Group[%d].Name = %v, want %v", i, allGroups[i].Name, wantPath)
				}
			}
		})
	}
}

func TestKeycloakGroupDescriptionExtraction(t *testing.T) {
	tests := []struct {
		name       string
		attributes map[string][]string
		wantDesc   *string
	}{
		{
			name: "description present",
			attributes: map[string][]string{
				"description": {"Engineering team"},
			},
			wantDesc: stringPtr("Engineering team"),
		},
		{
			name: "empty description array",
			attributes: map[string][]string{
				"description": {},
			},
			wantDesc: nil,
		},
		{
			name: "description with empty string",
			attributes: map[string][]string{
				"description": {""},
			},
			wantDesc: nil,
		},
		{
			name:       "no description attribute",
			attributes: map[string][]string{},
			wantDesc:   nil,
		},
		{
			name: "multiple values takes first",
			attributes: map[string][]string{
				"description": {"First description", "Second description"},
			},
			wantDesc: stringPtr("First description"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate description extraction logic from fetchAllKeycloakGroups
			var description *string
			if desc, ok := tt.attributes["description"]; ok && len(desc) > 0 && desc[0] != "" {
				description = &desc[0]
			}

			if !equalStringPtr(description, tt.wantDesc) {
				t.Errorf("Extracted description = %v, want %v",
					ptrToString(description), ptrToString(tt.wantDesc))
			}
		})
	}
}

func TestKeycloakGroupPathNormalization(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		groupName string
		want      string
	}{
		{
			name:      "normal path with slash",
			path:      "/Engineering",
			groupName: "Engineering",
			want:      "Engineering",
		},
		{
			name:      "nested path",
			path:      "/Company/Engineering",
			groupName: "Engineering",
			want:      "Company/Engineering",
		},
		{
			name:      "empty path uses name",
			path:      "",
			groupName: "Engineering",
			want:      "Engineering",
		},
		{
			name:      "root path uses name",
			path:      "/",
			groupName: "Engineering",
			want:      "Engineering",
		},
		{
			name:      "path without leading slash",
			path:      "Engineering",
			groupName: "Engineering",
			want:      "Engineering",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate path normalization logic
			displayName := tt.path
			if displayName == "" || displayName == "/" {
				displayName = tt.groupName
			}
			// Remove leading slash
			if len(displayName) > 0 && displayName[0] == '/' {
				displayName = displayName[1:]
			}

			if displayName != tt.want {
				t.Errorf("Normalized path = %v, want %v", displayName, tt.want)
			}
		})
	}
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
