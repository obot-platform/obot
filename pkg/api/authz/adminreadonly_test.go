package authz

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
	// adminReadonlyExcludedReads are the admin GET routes that the admin read-only group may not use, and why.
	adminReadonlyExcludedReads = map[string]string{
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

	// Some handlers register their routes separately from the API and gateway routers.
	routerFiles = []string{
		"../router/router.go",
		"../handlers/llmgatewayadmin.go",
		"../handlers/mcpgateway/oauth/handler.go",
		"../../gateway/server/router.go",
		"../../gateway/server/llmproxy_legacy.go",
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
	readonly := &user.DefaultInfo{Name: "admin-readonly", UID: "1", Groups: (types.RoleBasic | types.RoleAdminReadonly).Groups()}

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

func TestAdminReadonlyPrefixGrantsCoverOnlyReviewedReads(t *testing.T) {
	const reviewedPath = "testdata/admin_readonly_prefix_reads.txt"
	reviewed, err := os.ReadFile(reviewedPath)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSpace(string(reviewed)), "\n")
	readonly := rulesFromStatic(map[string][]string{types.GroupAdminReadonly: adminReadonlyRules})[0].mux

	// Prefix rules admit future routes automatically. Record the exact registrations so
	// replacing one route with another under the same prefix still requires review.
	var got []string
	for _, route := range routerRoutes(t) {
		if route.method != http.MethodGet {
			continue
		}
		req := httptest.NewRequest(route.method, route.path, nil)
		_, pattern := readonly.Handler(req)
		if strings.HasSuffix(pattern, "/") || strings.HasSuffix(pattern, "...}") {
			got = append(got, route.pattern)
		}
	}
	slices.Sort(got)
	got = slices.Compact(got)
	for _, pattern := range got {
		if !slices.Contains(want, pattern) {
			t.Errorf("new read covered by a read-only admin prefix: %s", pattern)
		}
	}
	for _, pattern := range want {
		if pattern != "" && !slices.Contains(got, pattern) {
			t.Errorf("reviewed read is no longer covered by a read-only admin prefix: %s", pattern)
		}
	}
	if t.Failed() {
		t.Logf("Review the affected routes before updating %s", reviewedPath)
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
			readonly: types.RoleBasic | types.RoleAdminReadonly,
			baseline: types.RoleBasic,
		},
		{
			name:     "admin read-only with auditor",
			readonly: types.RoleBasic | types.RoleAdminReadonly | types.RoleAuditor,
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
			role: types.RoleBasic | types.RoleAdminReadonly,
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
			if tt.role == types.RoleBasic|types.RoleAdminReadonly {
				for _, group := range groups {
					if group == types.GroupAdmin || group == types.GroupPowerUserPlus || group == types.GroupPowerUser {
						t.Errorf("admin read-only has group %q", group)
					}
				}
			}
		})
	}
}

func TestVMCPConfigurationPresenceRequiresAdminRead(t *testing.T) {
	// Even members who can use this shared vMCP cannot inspect its stored configuration.
	vmcp := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp-shared"),
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{
			{Name: "members", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "member"}}},
		}}},
	}
	authorizer := newVMCPTestAuthorizer(vmcp)
	for _, role := range []types.Role{types.RoleBasic, types.RoleBasic | types.RoleAdminReadonly, types.RoleBasic | types.RoleAuditor, types.RoleBasic | types.RoleAuditor | types.RoleAdminReadonly, types.RoleAdmin, types.RoleOwner} {
		req := httptest.NewRequest(http.MethodGet, "/api/vmcps/vmcp-shared/components/component/configuration-presence", nil)
		caller := &user.DefaultInfo{UID: "member", Groups: role.Groups()}
		if got, want := authorizer.Authorize(req, caller), slices.Contains(caller.Groups, types.GroupAdminReadonly); got != want {
			t.Errorf("configuration presence access for role %v = %v, want %v", role, got, want)
		}
	}
}

func TestAuditorVMCPReads(t *testing.T) {
	authorizer := newVMCPTestAuthorizer()
	auditor := &user.DefaultInfo{UID: "1", Groups: (types.RoleBasic | types.RoleAuditor).Groups()}
	for _, path := range []string{
		"/api/vmcps",
		"/api/vmcps/vmcp-shared",
		"/api/vmcps/vmcp-shared/oauth-url",
		"/api/vmcps/vmcp-shared/oauth-status",
		"/api/vmcps/vmcp-shared/tools",
		"/api/vmcps/vmcp-shared/resources",
		"/api/vmcps/vmcp-shared/resources/test",
		"/api/vmcps/vmcp-shared/prompts",
		"/api/vmcps/vmcp-shared/prompts/test",
	} {
		if !authorizer.Authorize(httptest.NewRequest(http.MethodGet, path, nil), auditor) {
			t.Errorf("Auditor should retain GET %s", path)
		}
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
	readonly := &user.DefaultInfo{Name: "admin-readonly", UID: "1", Groups: (types.RoleBasic | types.RoleAdminReadonly).Groups()}

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
