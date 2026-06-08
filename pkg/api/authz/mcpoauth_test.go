package authz

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMCPOAuthGroupAllowsOnlyMCPOAuthAndAnyGroupRoutes(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "msi1test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID: "mcpoauth-user-uid",
		},
	}).Build()
	authorizer := NewAuthorizer(storage, storage, false, nil, false)
	mcpoauthUser := &user.DefaultInfo{
		Name:   "mcpoauth-user",
		UID:    "mcpoauth-user-uid",
		Groups: []string{types.GroupMCPOAuth},
	}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "MCPOAuth static route",
			method: http.MethodGet,
			path:   "/oauth/userinfo",
		},
		{
			name:   "anyGroup static route",
			method: http.MethodGet,
			path:   "/api/healthz",
		},
		{
			name:   "anyGroup OAuth token route",
			method: http.MethodPost,
			path:   "/oauth/token",
		},
		{
			name:   "MCPOAuth registry route",
			method: http.MethodGet,
			path:   "/v0.1/servers",
		},
		{
			name:   "MCPOAuth MCP connect route",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if allowed := authorizer.Authorize(req, mcpoauthUser); !allowed {
				t.Fatalf("Authorize() = false, want true")
			}
		})
	}
}

func TestMCPOAuthGroupDeniesOtherGroupsAndUIRoutes(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, false, nil, false)
	mcpoauthUserWithOtherGroups := &user.DefaultInfo{
		Name: "mcpoauth-user",
		UID:  "mcpoauth-user-uid",
		Groups: []string{
			types.GroupMCPOAuth,
			types.GroupAuthenticated,
			types.GroupBasic,
			types.GroupPowerUser,
			types.GroupPowerUserPlus,
			types.GroupAuditor,
			types.GroupAdmin,
			types.GroupOwner,
			types.GroupAPIKey,
			MetricsGroup,
		},
		Extra: map[string][]string{
			types.APIKeySkillsAccessExtraKey: {"true"},
		},
	}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "authenticated route is denied",
			method: http.MethodGet,
			path:   "/api/me",
		},
		{
			name:   "basic route is denied",
			method: http.MethodGet,
			path:   "/api/models",
		},
		{
			name:   "admin route is denied",
			method: http.MethodPatch,
			path:   "/api/model-providers/provider-id",
		},
		{
			name:   "API key optional skill route is denied",
			method: http.MethodGet,
			path:   "/api/skills",
		},
		{
			name:   "metrics route is denied",
			method: http.MethodGet,
			path:   "/debug/metrics",
		},
		{
			name:   "UI root is denied",
			method: http.MethodGet,
			path:   "/",
		},
		{
			name:   "admin UI route is denied",
			method: http.MethodGet,
			path:   "/admin",
		},
		{
			name:   "non-UI auth route from another group is denied",
			method: http.MethodGet,
			path:   "/auth/mcp/composite/msi1test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if allowed := authorizer.Authorize(req, mcpoauthUserWithOtherGroups); allowed {
				t.Fatalf("Authorize() = true, want false")
			}
		})
	}
}

func TestMCPOAuthGroupDoesNotInheritStaticOrAPIResourceRulesFromOtherGroups(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, false, nil, false)
	mcpoauthUserWithAllGroups := &user.DefaultInfo{
		Name: "mcpoauth-user",
		UID:  "mcpoauth-user-uid",
		Groups: []string{
			types.GroupMCPOAuth,
			types.GroupAuthenticated,
			types.GroupBasic,
			types.GroupPowerUser,
			types.GroupPowerUserPlus,
			types.GroupAuditor,
			types.GroupAdmin,
			types.GroupOwner,
			types.GroupAPIKey,
			MetricsGroup,
		},
		Extra: map[string][]string{
			types.APIKeySkillsAccessExtraKey: {"true"},
		},
	}

	allowedStaticPatterns := map[string]struct{}{}
	for _, pattern := range staticRules[anyGroup] {
		allowedStaticPatterns[canonicalAuthzPattern(pattern)] = struct{}{}
	}
	for _, pattern := range staticRules[types.GroupMCPOAuth] {
		allowedStaticPatterns[canonicalAuthzPattern(pattern)] = struct{}{}
	}

	for group, patterns := range staticRules {
		if group == anyGroup || group == types.GroupMCPOAuth {
			continue
		}

		for _, pattern := range patterns {
			pattern := pattern
			if _, allowed := allowedStaticPatterns[canonicalAuthzPattern(pattern)]; allowed {
				continue
			}

			t.Run("static "+group+" "+pattern, func(t *testing.T) {
				req := requestForAuthzPattern(t, pattern)
				if allowed := authorizer.Authorize(req, mcpoauthUserWithAllGroups); allowed {
					t.Fatalf("Authorize(%s %s) = true, want false", req.Method, req.URL.Path)
				}
			})
		}
	}

	allowedAPIResourcePatterns := map[string]struct{}{}
	for pattern := range allowedStaticPatterns {
		allowedAPIResourcePatterns[pattern] = struct{}{}
	}
	for _, pattern := range apiResources[types.GroupMCPOAuth] {
		allowedAPIResourcePatterns[canonicalAuthzPattern(pattern)] = struct{}{}
	}

	for group, patterns := range apiResources {
		if group == types.GroupMCPOAuth {
			continue
		}

		for _, pattern := range patterns {
			pattern := pattern
			if _, allowed := allowedAPIResourcePatterns[canonicalAuthzPattern(pattern)]; allowed {
				continue
			}

			t.Run("api resource "+group+" "+pattern, func(t *testing.T) {
				req := requestForAuthzPattern(t, pattern)
				if allowed := authorizer.Authorize(req, mcpoauthUserWithAllGroups); allowed {
					t.Fatalf("Authorize(%s %s) = true, want false", req.Method, req.URL.Path)
				}
			})
		}
	}
}

func canonicalAuthzPattern(pattern string) string {
	fields := strings.Fields(pattern)
	if len(fields) == 2 && isHTTPMethod(fields[0]) {
		return fields[0] + " " + fields[1]
	}
	return pattern
}

func requestForAuthzPattern(t *testing.T, pattern string) *http.Request {
	t.Helper()

	fields := strings.Fields(pattern)
	method := http.MethodPatch
	path := pattern
	if len(fields) == 2 && isHTTPMethod(fields[0]) {
		method = fields[0]
		path = fields[1]
	}

	return httptest.NewRequest(method, sampleAuthzPath(path), nil)
}

func sampleAuthzPath(pattern string) string {
	for {
		start := strings.Index(pattern, "{")
		if start == -1 {
			return pattern
		}

		end := strings.Index(pattern[start:], "}")
		if end == -1 {
			return pattern
		}

		end += start
		pattern = pattern[:start] + sampleAuthzPathValue(pattern[start+1:end]) + pattern[end+1:]
	}
}

func sampleAuthzPathValue(name string) string {
	switch name {
	case "mcp_id", "mcp_server_instance_id":
		return "msi1test"
	case "mcpserver_id", "mcp_server_id":
		return "ms1test"
	case "namespace":
		return system.DefaultNamespace
	default:
		return "test"
	}
}

func isHTTPMethod(method string) bool {
	switch method {
	case http.MethodConnect,
		http.MethodDelete,
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
		http.MethodTrace:
		return true
	default:
		return false
	}
}
