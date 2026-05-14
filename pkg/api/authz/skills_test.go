package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestSkillRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, false, nil, false)

	tests := []struct {
		name    string
		method  string
		path    string
		user    user.Info
		allowed bool
	}{
		{
			name:   "admin can access skill repositories",
			method: http.MethodGet,
			path:   "/api/skill-repositories",
			user: &user.DefaultInfo{
				Name:   "admin",
				Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "basic user cannot access skill repositories",
			method: http.MethodGet,
			path:   "/api/skill-repositories",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "basic user can access skills list",
			method: http.MethodGet,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "api key user with skills access can access skills list",
			method: http.MethodGet,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupAPIKey},
				Extra: map[string][]string{
					types.APIKeySkillsAccessExtraKey: {"true"},
				},
			},
			allowed: true,
		},
		{
			name:   "api key user with skills access can access skill detail",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupAPIKey},
				Extra: map[string][]string{
					types.APIKeySkillsAccessExtraKey: {"true"},
				},
			},
			allowed: true,
		},
		{
			name:   "api key user with skills access can download skill",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id/download",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupAPIKey},
				Extra: map[string][]string{
					types.APIKeySkillsAccessExtraKey: {"true"},
				},
			},
			allowed: true,
		},
		{
			name:   "api key user without skills access cannot access skills list",
			method: http.MethodGet,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupAPIKey},
			},
			allowed: false,
		},
		{
			name:   "api key user cannot POST skills",
			method: http.MethodPost,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupAPIKey},
			},
			allowed: false,
		},
		{
			name:   "auditor can access skills list",
			method: http.MethodGet,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill detail",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can download skill",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id/download",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill repositories",
			method: http.MethodGet,
			path:   "/api/skill-repositories",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill repository detail",
			method: http.MethodGet,
			path:   "/api/skill-repositories/some-repo-id",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill access rules",
			method: http.MethodGet,
			path:   "/api/skill-access-rules",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill access rule detail",
			method: http.MethodGet,
			path:   "/api/skill-access-rules/some-rule-id",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor cannot POST skill repositories",
			method: http.MethodPost,
			path:   "/api/skill-repositories",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "auditor cannot POST skill access rules",
			method: http.MethodPost,
			path:   "/api/skill-access-rules",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			assert.Equal(t, tt.allowed, authorizer.Authorize(req, tt.user))
		})
	}
}
