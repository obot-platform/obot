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

// TestVMCPPreconfiguredOAuthAuthorization checks that only administrators, and owners, who have every administrator's
// rule, authorize and revoke a vMCP component's preconfigured OAuth, and that read-only administrators and auditors,
// who read vMCPs, read its state and nothing more. Power users, users a profile names, who may reset their own
// component OAuth on the vMCP, and agents, including the agents a profile names, reach none of its routes.
func TestVMCPPreconfiguredOAuthAuthorization(t *testing.T) {
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
	const path = "/api/vmcps/vmcp1grants/components/component/preconfigured-oauth"
	for _, tt := range []struct {
		name string
		// reads and writes are whether the caller reads the state (GET) and authorizes and revokes (POST, DELETE).
		reads, writes bool
		caller        user.Info
	}{
		{
			name:   "administrator",
			caller: person(types.GroupAdmin),
			reads:  true,
			writes: true,
		},
		{
			name:   "owner",
			caller: person(types.GroupOwner),
			reads:  true,
			writes: true,
		},
		{
			name:   "read-only administrator",
			caller: person(types.GroupAdminReadonly),
			reads:  true,
		},
		{
			name:   "auditor",
			caller: person(types.GroupAuditor),
			reads:  true,
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
			for method, want := range map[string]bool{http.MethodGet: tt.reads, http.MethodPost: tt.writes, http.MethodDelete: tt.writes} {
				if got := authorizer.Authorize(httptest.NewRequest(method, path, nil), tt.caller); got != want {
					t.Errorf("%s %s: authorized = %v, want %v", method, path, got, want)
				}
			}
		})
	}

	// A user a profile names still resets their own OAuth for the component, which is not the vMCP's preconfigured OAuth.
	if !authorizer.Authorize(httptest.NewRequest(http.MethodDelete, "/api/vmcps/vmcp1grants/components/component/oauth", nil), person()) {
		t.Fatal("a user of the vMCP cannot reset their own component OAuth")
	}
}
