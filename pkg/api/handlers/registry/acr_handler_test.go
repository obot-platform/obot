package registry

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

// mockUser implements user.Info for testing
type mockUser struct {
	uid    string
	groups []string
	extra  map[string][]string
}

func (m *mockUser) GetName() string                       { return m.uid }
func (m *mockUser) GetUID() string                        { return m.uid }
func (m *mockUser) GetGroups() []string                   { return m.groups }
func (m *mockUser) GetExtra() map[string][]string         { return m.extra }
func (m *mockUser) GetAuthID() string                     { return "" }
func (m *mockUser) GetAuthentications() []string          { return nil }
func (m *mockUser) GetPersistentIdentity() (string, bool) { return "", false }

func TestHasWildcardSubject(t *testing.T) {
	handler := &ACRHandler{}

	tests := []struct {
		name     string
		acr      v1.AccessControlRule
		expected bool
	}{
		{
			name: "has wildcard selector",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeSelector, ID: "*"},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "has wildcard among other subjects",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user-123"},
							{Type: types.SubjectTypeSelector, ID: "*"},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "no wildcard selector",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user-123"},
							{Type: types.SubjectTypeGroup, ID: "group-456"},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "empty subjects",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{},
					},
				},
			},
			expected: false,
		},
		{
			name: "selector with non-wildcard ID",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeSelector, ID: "some-other-selector"},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.hasWildcardSubject(tt.acr)
			if result != tt.expected {
				t.Errorf("hasWildcardSubject() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUserMatchesSubjects(t *testing.T) {
	handler := &ACRHandler{}

	tests := []struct {
		name     string
		acr      v1.AccessControlRule
		user     user.Info
		expected bool
	}{
		{
			name: "user ID matches",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user-123"},
						},
					},
				},
			},
			user:     &mockUser{uid: "user-123"},
			expected: true,
		},
		{
			name: "user group matches",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeGroup, ID: "developers"},
						},
					},
				},
			},
			user: &mockUser{
				uid: "user-456",
				extra: map[string][]string{
					"auth_provider_groups": {"developers", "admins"},
				},
			},
			expected: true,
		},
		{
			name: "wildcard selector matches",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeSelector, ID: "*"},
						},
					},
				},
			},
			user:     &mockUser{uid: "any-user"},
			expected: true,
		},
		{
			name: "no match",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user-123"},
							{Type: types.SubjectTypeGroup, ID: "admins"},
						},
					},
				},
			},
			user: &mockUser{
				uid: "user-456",
				extra: map[string][]string{
					"auth_provider_groups": {"developers"},
				},
			},
			expected: false,
		},
		{
			name: "multiple subjects, one matches",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user-123"},
							{Type: types.SubjectTypeUser, ID: "user-456"},
							{Type: types.SubjectTypeGroup, ID: "admins"},
						},
					},
				},
			},
			user:     &mockUser{uid: "user-456"},
			expected: true,
		},
		{
			name: "empty subjects",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{},
					},
				},
			},
			user:     &mockUser{uid: "user-123"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal api.Context with just the user
			ctx := api.Context{
				User: tt.user,
			}
			result := handler.userMatchesSubjects(ctx, tt.acr)
			if result != tt.expected {
				t.Errorf("userMatchesSubjects() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsAuthorized(t *testing.T) {
	tests := []struct {
		name           string
		registryNoAuth bool
		acr            v1.AccessControlRule
		user           user.Info
		expected       bool
	}{
		{
			name:           "auth OFF - wildcard subject",
			registryNoAuth: true,
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeSelector, ID: "*"},
						},
					},
				},
			},
			user:     &mockUser{uid: "any-user"},
			expected: true,
		},
		{
			name:           "auth OFF - no wildcard subject",
			registryNoAuth: true,
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user-123"},
						},
					},
				},
			},
			user:     &mockUser{uid: "user-123"},
			expected: false,
		},
		{
			name:           "auth ON - user matches",
			registryNoAuth: false,
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user-123"},
						},
					},
				},
			},
			user:     &mockUser{uid: "user-123"},
			expected: true,
		},
		{
			name:           "auth ON - user doesn't match",
			registryNoAuth: false,
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user-123"},
						},
					},
				},
			},
			user:     &mockUser{uid: "user-456"},
			expected: false,
		},
		{
			name:           "auth ON - group matches",
			registryNoAuth: false,
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeGroup, ID: "developers"},
						},
					},
				},
			},
			user: &mockUser{
				uid: "user-123",
				extra: map[string][]string{
					"auth_provider_groups": {"developers"},
				},
			},
			expected: true,
		},
		{
			name:           "auth ON - wildcard matches",
			registryNoAuth: false,
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeSelector, ID: "*"},
						},
					},
				},
			},
			user:     &mockUser{uid: "any-user"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &ACRHandler{
				registryNoAuth: tt.registryNoAuth,
			}
			ctx := api.Context{
				User: tt.user,
			}
			result := handler.isAuthorized(ctx, tt.acr)
			if result != tt.expected {
				t.Errorf("isAuthorized() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestServerInACRResources(t *testing.T) {
	handler := &ACRHandler{}

	tests := []struct {
		name       string
		acr        v1.AccessControlRule
		serverName string
		expected   bool
	}{
		{
			name: "server explicitly listed",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Resources: []types.Resource{
							{Type: types.ResourceTypeMCPServer, ID: "mcp-server-abc123"},
						},
					},
				},
			},
			serverName: "mcp-server-abc123",
			expected:   true,
		},
		{
			name: "catalog entry explicitly listed",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Resources: []types.Resource{
							{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "filesystem"},
						},
					},
				},
			},
			serverName: "filesystem",
			expected:   true,
		},
		{
			name: "wildcard selector includes all",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Resources: []types.Resource{
							{Type: types.ResourceTypeSelector, ID: "*"},
						},
					},
				},
			},
			serverName: "any-server",
			expected:   true,
		},
		{
			name: "server not in list",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Resources: []types.Resource{
							{Type: types.ResourceTypeMCPServer, ID: "mcp-server-abc123"},
							{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "filesystem"},
						},
					},
				},
			},
			serverName: "other-server",
			expected:   false,
		},
		{
			name: "empty resources",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Resources: []types.Resource{},
					},
				},
			},
			serverName: "any-server",
			expected:   false,
		},
		{
			name: "multiple resources, one matches",
			acr: v1.AccessControlRule{
				Spec: v1.AccessControlRuleSpec{
					Manifest: types.AccessControlRuleManifest{
						Resources: []types.Resource{
							{Type: types.ResourceTypeMCPServer, ID: "server-1"},
							{Type: types.ResourceTypeMCPServer, ID: "server-2"},
							{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "filesystem"},
						},
					},
				},
			},
			serverName: "server-2",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.serverInACRResources(tt.acr, tt.serverName)
			if result != tt.expected {
				t.Errorf("serverInACRResources() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAuthGroupSet(t *testing.T) {
	tests := []struct {
		name     string
		user     user.Info
		expected map[string]struct{}
	}{
		{
			name: "single group",
			user: &mockUser{
				extra: map[string][]string{
					"auth_provider_groups": {"developers"},
				},
			},
			expected: map[string]struct{}{
				"developers": {},
			},
		},
		{
			name: "multiple groups",
			user: &mockUser{
				extra: map[string][]string{
					"auth_provider_groups": {"developers", "admins", "users"},
				},
			},
			expected: map[string]struct{}{
				"developers": {},
				"admins":     {},
				"users":      {},
			},
		},
		{
			name: "no groups",
			user: &mockUser{
				extra: map[string][]string{},
			},
			expected: map[string]struct{}{},
		},
		{
			name: "nil extra",
			user: &mockUser{
				extra: nil,
			},
			expected: map[string]struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := authGroupSet(tt.user)
			if len(result) != len(tt.expected) {
				t.Errorf("authGroupSet() length = %v, want %v", len(result), len(tt.expected))
				return
			}
			for group := range tt.expected {
				if _, ok := result[group]; !ok {
					t.Errorf("authGroupSet() missing group %v", group)
				}
			}
		})
	}
}

func TestNotFoundError(t *testing.T) {
	handler := &ACRHandler{}

	tests := []struct {
		name     string
		detail   string
		wantCode int
		wantMsg  string
	}{
		{
			name:     "simple message",
			detail:   "server not found",
			wantCode: 404,
			wantMsg:  `{"title":"Not Found","status":404,"detail":"server not found"}`,
		},
		{
			name:     "empty message",
			detail:   "",
			wantCode: 404,
			wantMsg:  `{"title":"Not Found","status":404,"detail":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.notFoundError(tt.detail)
			if err == nil {
				t.Fatal("notFoundError() returned nil")
			}

			httpErr, ok := err.(*types.ErrHTTP)
			if !ok {
				t.Fatalf("notFoundError() did not return *types.ErrHTTP")
			}

			if httpErr.Code != tt.wantCode {
				t.Errorf("notFoundError() code = %v, want %v", httpErr.Code, tt.wantCode)
			}

			if httpErr.Message != tt.wantMsg {
				t.Errorf("notFoundError() message = %v, want %v", httpErr.Message, tt.wantMsg)
			}
		})
	}
}

func TestNewACRHandler(t *testing.T) {
	serverURL := "https://obot.example.com"
	registryNoAuth := false

	handler := NewACRHandler(nil, serverURL, registryNoAuth)

	if handler == nil {
		t.Fatal("NewACRHandler() returned nil")
	}

	if handler.serverURL != serverURL {
		t.Errorf("NewACRHandler() serverURL = %v, want %v", handler.serverURL, serverURL)
	}

	if handler.registryNoAuth != registryNoAuth {
		t.Errorf("NewACRHandler() registryNoAuth = %v, want %v", handler.registryNoAuth, registryNoAuth)
	}

	if handler.mimeFetcher == nil {
		t.Error("NewACRHandler() mimeFetcher is nil")
	}
}
