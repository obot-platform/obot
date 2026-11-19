package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

// TestVMCPAgentOAuthAuthorization checks that only administrators, and owners, who have every administrator's rule,
// authorize and revoke a vMCP's agent OAuth grants. Read-only administrators and auditors, who read vMCPs, users a
// profile names, who may reset their own component OAuth on the vMCP, and agents, including the agents a profile names,
// reach neither route.
func TestVMCPAgentOAuthAuthorization(t *testing.T) {
	vmcp := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp1grants"),
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{{ID: "component"}},
			Profiles: []types.VMCPProfile{
				{
					ID:          "vp1agent",
					Name:        "agent",
					Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				},
				{
					ID:          "vp1everyone",
					Name:        "everyone",
					Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				},
			},
		}},
	}
	authorizer := newVMCPTestAuthorizer(vmcp)
	person := func(groups ...string) user.Info {
		return principaltest.AsUser(&user.DefaultInfo{Name: "101", UID: "101", Groups: append([]string{types.GroupAuthenticated, types.GroupAPI}, groups...)})
	}
	agent := agentKeyPrincipal(gatewaytypes.APIKeyScopes{
		CanAccessSkills:   true,
		CanAccessLLMProxy: true,
		MCPServerIDs:      []string{"*"},
	})
	var routes []agentTestRoute
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		routes = append(routes, agentTestRoute{
			method: method,
			path:   "/api/vmcps/vmcp1grants/agent-oauth/component",
		})
	}
	for _, tt := range []struct {
		name    string
		caller  user.Info
		allowed bool
	}{
		{
			name:    "administrator",
			caller:  person(types.GroupAdmin),
			allowed: true,
		},
		{
			name:    "owner",
			caller:  person(types.GroupOwner),
			allowed: true,
		},
		{
			name:   "read-only administrator",
			caller: person(types.GroupAdminReadonly),
		},
		{
			name:   "auditor",
			caller: person(types.GroupAuditor),
		},
		{
			name:   "power user plus",
			caller: person(types.GroupPowerUserPlus, types.GroupPowerUser),
		},
		{
			name:   "user a profile names",
			caller: person(),
		},
		{
			name:   "agent a profile names",
			caller: agent,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, route := range routes {
				if got := authorizer.Authorize(httptest.NewRequest(route.method, route.path, nil), tt.caller); got != tt.allowed {
					t.Errorf("%s %s: authorized = %v, want %v", route.method, route.path, got, tt.allowed)
				}
			}
		})
	}

	// A user a profile names still resets their own OAuth for the component, which is not the vMCP's agent grant.
	if !authorizer.Authorize(httptest.NewRequest(http.MethodDelete, "/api/vmcps/vmcp1grants/components/component/oauth", nil), person()) {
		t.Fatal("a user of the vMCP cannot reset their own component OAuth")
	}
}
