package authz

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
	// adminReadonlyExcludedReads are the admin GET routes that the admin read-only group may not use, and why.
	adminReadonlyExcludedReads = map[string]string{
		"GET /api/mdm/configurations/{id}/download/{slug}":                     "installers can enroll devices",
		"GET /api/setup/explicit-role-emails":                                  "setup is for owners",
		"GET /api/setup/temp-user":                                             "setup is for owners",
		"GET /api/all-mcps/servers/{mcp_server_id}/tools":                      "resolves a connection, which can launch the server",
		"GET /api/all-mcps/servers/{mcp_server_id}/resources":                  "resolves a connection, which can launch the server",
		"GET /api/all-mcps/servers/{mcp_server_id}/resources/{resource_uri}":   "resolves a connection, which can launch the server",
		"GET /api/all-mcps/servers/{mcp_server_id}/prompts":                    "resolves a connection, which can launch the server",
		"GET /api/all-mcps/servers/{mcp_server_id}/prompts/{prompt_name}":      "resolves a connection, which can launch the server",
		"GET /api/mcp-servers/{mcp_server_id}/oauth-url":                       "resolves the caller's connection to start OAuth",
		"GET /api/mcp-catalogs/{catalog_id}/servers/{mcp_server_id}/oauth-url": "resolves the caller's connection to start OAuth",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth-url": "resolves the caller's connection to start OAuth",
		"GET /api/vmcps/{vmcp_id}/oauth-url":                                   "resolves the caller's connection to the vMCP",
		"GET /api/vmcps/{vmcp_id}/oauth-status":                                "resolves the caller's connection to the vMCP",
		"GET /api/vmcps/{vmcp_id}/tools":                                       "resolves the caller's connection to the vMCP",
		"GET /api/vmcps/{vmcp_id}/resources":                                   "resolves the caller's connection to the vMCP",
		"GET /api/vmcps/{vmcp_id}/resources/{resource_uri}":                    "resolves the caller's connection to the vMCP",
		"GET /api/vmcps/{vmcp_id}/prompts":                                     "resolves the caller's connection to the vMCP",
		"GET /api/vmcps/{vmcp_id}/prompts/{prompt_name}":                       "resolves the caller's connection to the vMCP",
		"GET /api/system-mcp-servers/{id}/tools":                               "tool discovery launches the system server",
	}

	// routerFiles are the files that register the API's routes: the API's own router and the gateway's.
	routerFiles = []string{
		"../router/router.go",
		"../../gateway/server/router.go",
	}
)

type routerRoute struct {
	pattern string
	method  string
	path    string
}

// routerRoutes returns the API routes registered in the routers, with each wildcard replaced by a sample value.
func routerRoutes(t *testing.T) []routerRoute {
	t.Helper()

	var src []byte
	for _, file := range routerFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read router %s: %v", file, err)
		}
		src = append(append(src, content...), '\n')
	}

	var (
		routes   []routerRoute
		wildcard = regexp.MustCompile(`\{[^}]+\}`)
	)
	for _, match := range regexp.MustCompile(`HandleFunc\("([A-Z]+) +(/api/[^"]*)"`).FindAllStringSubmatch(string(src), -1) {
		path := strings.TrimSuffix(match[2], "{$}")
		path = wildcard.ReplaceAllStringFunc(path, func(w string) string {
			if strings.HasSuffix(w, "...}") {
				return "x/y"
			}
			return "x"
		})
		routes = append(routes, routerRoute{
			pattern: match[1] + " " + match[2],
			method:  match[1],
			path:    path,
		})
	}
	if len(routes) == 0 {
		t.Fatal("found no routes in the router")
	}
	return routes
}

func TestAdminReadonlyRulesAreReads(t *testing.T) {
	for _, rule := range adminReadonlyRules {
		if !strings.HasPrefix(rule, "GET ") {
			t.Errorf("admin read-only rule %q is not a GET", rule)
		}
	}
}

// TestAdminReadonlyRulesCoverAdminReads fails when an admin GET route is neither granted to the admin read-only group
// nor listed in adminReadonlyExcludedReads, so that new admin routes are classified deliberately.
func TestAdminReadonlyRulesCoverAdminReads(t *testing.T) {
	admin := rulesFromStatic(map[string][]string{types.GroupAdmin: adminAndOwnerRules})[0].mux
	authorizer := newVMCPTestAuthorizer()
	readonly := &user.DefaultInfo{Name: "admin-readonly", UID: "1", Groups: types.RoleAdminReadonly.Groups()}

	for _, route := range routerRoutes(t) {
		if route.method != http.MethodGet {
			continue
		}
		req := httptest.NewRequest(route.method, route.path, nil)
		if _, pattern := admin.Handler(req); pattern == "" {
			continue
		}

		allowed := authorizer.Authorize(req, readonly)
		if _, excluded := adminReadonlyExcludedReads[route.pattern]; excluded {
			if allowed {
				t.Errorf("%s is excluded from admin read-only but is allowed", route.pattern)
			}
		} else if !allowed {
			t.Errorf("%s is an admin read but is not allowed for admin read-only; add it to adminReadonlyRules or adminReadonlyExcludedReads", route.pattern)
		}
	}
}

// TestAdminReadonlyCannotMutate checks that the admin read-only role may make no change that a basic user may not.
func TestAdminReadonlyCannotMutate(t *testing.T) {
	authorizer := newVMCPTestAuthorizer()
	pairs := []struct {
		name     string
		readonly types.Role
		baseline types.Role
	}{
		{
			name:     "admin read-only",
			readonly: types.RoleAdminReadonly,
			baseline: types.RoleBasic,
		},
		{
			name:     "admin read-only with auditor",
			readonly: types.RoleAdminReadonly | types.RoleAuditor,
			baseline: types.RoleBasic | types.RoleAuditor,
		},
	}

	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			readonly := &user.DefaultInfo{Name: "readonly", UID: "1", Groups: pair.readonly.Groups()}
			baseline := &user.DefaultInfo{Name: "baseline", UID: "1", Groups: pair.baseline.Groups()}
			for _, route := range routerRoutes(t) {
				if route.method == http.MethodGet {
					continue
				}
				req := httptest.NewRequest(route.method, route.path, nil)
				if authorizer.Authorize(req, readonly) && !authorizer.Authorize(req, baseline) {
					t.Errorf("%s is allowed for %s but not for its baseline role", route.pattern, pair.name)
				}
			}
		})
	}
}

func TestAdminReadonlyGroups(t *testing.T) {
	tests := []struct {
		name string
		role types.Role
		want bool
	}{
		{
			name: "owner",
			role: types.RoleOwner,
			want: true,
		},
		{
			name: "admin",
			role: types.RoleAdmin,
			want: true,
		},
		{
			name: "admin read-only",
			role: types.RoleAdminReadonly,
			want: true,
		},
		{
			name: "basic auditor",
			role: types.RoleBasic | types.RoleAuditor,
			want: false,
		},
		{
			name: "power user plus",
			role: types.RolePowerUserPlus,
			want: false,
		},
		{
			name: "basic",
			role: types.RoleBasic,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := tt.role.Groups()
			got := false
			for _, group := range groups {
				if group == types.GroupAdminReadonly {
					got = true
				}
			}
			if got != tt.want {
				t.Errorf("admin read-only group = %v, want %v (groups %v)", got, tt.want, groups)
			}
			if tt.role == types.RoleAdminReadonly {
				for _, group := range groups {
					if group == types.GroupAdmin || group == types.GroupPowerUserPlus || group == types.GroupPowerUser {
						t.Errorf("admin read-only has group %q", group)
					}
				}
			}
		})
	}
}

// TestAdminReadonlyConnectionReadsFollowUserAccess checks that GETs which resolve a connection are not granted to the
// admin read-only group: they fall back to the access the user has as a user.
func TestAdminReadonlyConnectionReadsFollowUserAccess(t *testing.T) {
	sharedWith := func(name, userID string) *v1.VMCP {
		return &v1.VMCP{
			ObjectMeta: objectMetaForAuthzTest(name),
			Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
				Name:     "team",
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: userID}},
			}}}},
		}
	}
	personal := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp-personal"),
		Spec:       v1.VMCPSpec{UserID: "owner"},
	}
	authorizer := newVMCPTestAuthorizer(sharedWith("vmcp-shared-mine", "1"), sharedWith("vmcp-shared-other", "2"), personal)
	readonly := &user.DefaultInfo{Name: "admin-readonly", UID: "1", Groups: types.RoleAdminReadonly.Groups()}

	tests := []struct {
		name    string
		path    string
		allowed bool
	}{
		{
			name:    "reads another user's personal vMCP",
			path:    "/api/vmcps/vmcp-personal",
			allowed: true,
		},
		{
			name: "cannot list tools of another user's personal vMCP",
			path: "/api/vmcps/vmcp-personal/tools",
		},
		{
			name: "cannot get an OAuth URL for another user's personal vMCP",
			path: "/api/vmcps/vmcp-personal/oauth-url",
		},
		{
			name: "cannot get OAuth status for another user's personal vMCP",
			path: "/api/vmcps/vmcp-personal/oauth-status",
		},
		{
			name: "cannot list tools of a shared vMCP without a matching profile",
			path: "/api/vmcps/vmcp-shared-other/tools",
		},
		{
			name:    "lists tools of a shared vMCP with a matching profile",
			path:    "/api/vmcps/vmcp-shared-mine/tools",
			allowed: true,
		},
		{
			name:    "reads system server details",
			path:    "/api/system-mcp-servers/x/details",
			allowed: true,
		},
		{
			name: "cannot list system server tools",
			path: "/api/system-mcp-servers/x/tools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if got := authorizer.Authorize(req, readonly); got != tt.allowed {
				t.Errorf("Authorize(GET %s) = %v, want %v", tt.path, got, tt.allowed)
			}
		})
	}
}
