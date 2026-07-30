package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestCheckUI(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		pattern  string
		user     user.Info
		expected bool
	}{
		{
			name:    "regular user can access a UI route",
			method:  http.MethodGet,
			path:    "/mcp-servers",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: types.RoleBasic.Groups(),
			},
			expected: true,
		},
		{
			name:    "HEAD request can access a UI route",
			method:  http.MethodHead,
			path:    "/_app/immutable/app.js",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			expected: true,
		},
		{
			name:    "non-read request cannot use the UI fallback",
			method:  http.MethodPost,
			path:    "/somewhere",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: types.RoleBasic.Groups(),
			},
			expected: false,
		},
		{
			name:    "request without a selected route is rejected",
			method:  http.MethodGet,
			path:    "/somewhere",
			pattern: "",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: types.RoleBasic.Groups(),
			},
			expected: false,
		},
		{
			name:    "explicit backend route cannot use the UI fallback",
			method:  http.MethodGet,
			path:    "/future-backend/123",
			pattern: "GET /future-backend/{id}",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: types.RoleBasic.Groups(),
			},
			expected: false,
		},
		{
			name:    "unauthenticated user can access admin landing page",
			method:  http.MethodGet,
			path:    "/admin",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			expected: true,
		},
		{
			name:    "unauthenticated user can access admin assets",
			method:  http.MethodGet,
			path:    "/admin/assets/app.js",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			expected: true,
		},
		{
			name:    "admin can access admin subroute",
			method:  http.MethodGet,
			path:    "/admin/users",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "admin",
				Groups: types.RoleAdmin.Groups(),
			},
			expected: true,
		},
		{
			name:    "owner can access admin subroute",
			method:  http.MethodGet,
			path:    "/admin/users",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "owner",
				Groups: types.RoleOwner.Groups(),
			},
			expected: true,
		},
		{
			name:    "auditor can access admin subroute",
			method:  http.MethodGet,
			path:    "/admin/audit-logs",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: types.RoleAuditor.Groups(),
			},
			expected: true,
		},
		{
			name:    "regular user cannot access admin subroute",
			method:  http.MethodGet,
			path:    "/admin/users",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: types.RoleBasic.Groups(),
			},
			expected: false,
		},
		{
			name:    "unauthenticated user cannot access admin subroute",
			method:  http.MethodGet,
			path:    "/admin/auth-providers",
			pattern: "/",
			user: &user.DefaultInfo{
				Name:   "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			expected: false,
		},
	}

	authorizer := &Authorizer{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Pattern = tt.pattern

			result := authorizer.checkUI(req, newUser(tt.user))
			if result != tt.expected {
				t.Errorf("checkUI() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCheckUIUsesServeMuxRouteSelection(t *testing.T) {
	authorizer := &Authorizer{}
	currentUser := newUser(&user.DefaultInfo{
		Name:   "user",
		Groups: types.RoleBasic.Groups(),
	})

	check := func(w http.ResponseWriter, req *http.Request) {
		if !authorizer.checkUI(req, currentUser) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}

	tests := []struct {
		name            string
		method          string
		path            string
		expectedStatus  int
		expectedPattern string
	}{
		{
			name:            "UI path selects fallback and is allowed",
			method:          http.MethodGet,
			path:            "/some/new/ui/path",
			expectedStatus:  http.StatusNoContent,
			expectedPattern: "/",
		},
		{
			name:            "new backend route selects its own pattern and is rejected",
			method:          http.MethodGet,
			path:            "/future-backend/123",
			expectedStatus:  http.StatusForbidden,
			expectedPattern: "GET /future-backend/{id}",
		},
		{
			name:            "POST to fallback is rejected",
			method:          http.MethodPost,
			path:            "/some/new/ui/path",
			expectedStatus:  http.StatusForbidden,
			expectedPattern: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var selectedPattern string
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)

			recordPattern := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				selectedPattern = req.Pattern
				check(w, req)
			})
			testMux := http.NewServeMux()
			testMux.Handle("GET /future-backend/{id}", recordPattern)
			testMux.Handle("/", recordPattern)
			testMux.ServeHTTP(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tt.expectedStatus)
			}
			if selectedPattern != tt.expectedPattern {
				t.Errorf("selected pattern = %q, want %q", selectedPattern, tt.expectedPattern)
			}
		})
	}
}

func TestPublicImageAuthorizationDoesNotDependOnUIFallback(t *testing.T) {
	authorizer := &Authorizer{
		rules: defaultRules(false, false),
	}
	userInfo := &user.DefaultInfo{
		Name:   "anonymous",
		Groups: []string{UnauthenticatedGroup},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/image/image-123", nil)
	req.Pattern = "GET /api/image/{id}"

	if authorizer.checkUI(req, newUser(userInfo)) {
		t.Fatal("API image route was incorrectly authorized as UI traffic")
	}
	if !authorizer.Authorize(req, userInfo) {
		t.Fatal("public API image route was not authorized by the normal route rules")
	}
}
