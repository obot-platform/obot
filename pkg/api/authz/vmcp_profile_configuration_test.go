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

// TestVMCPProfileConfigurationAuthorization checks that only administrators, and owners, who have every
// administrator's rule, set the configuration of a vMCP's profiles. Auditors, who read vMCPs, users the profile names,
// and agents, including the agent the profile is for, reach neither route.
func TestVMCPProfileConfigurationAuthorization(t *testing.T) {
	vmcp := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp1profiles"),
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
	routes := []agentTestRoute{
		{
			method: http.MethodPut,
			path:   "/api/vmcps/vmcp1profiles/profiles/vp1agent/configuration",
		},
		{
			method: http.MethodPost,
			path:   "/api/vmcps/vmcp1profiles/profiles/vp1agent/configuration/reset",
		},
		{
			method: http.MethodPut,
			path:   "/api/vmcps/vmcp1profiles/profiles/vp1everyone/configuration",
		},
		{
			method: http.MethodPost,
			path:   "/api/vmcps/vmcp1profiles/profiles/vp1everyone/configuration/reset",
		},
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
			name:   "auditor",
			caller: person(types.GroupAuditor),
		},
		{
			name:   "power user plus",
			caller: person(types.GroupPowerUserPlus, types.GroupPowerUser),
		},
		{
			// Every user matches the selector wildcard profile, which lets them read and connect to the vMCP only.
			name:   "user the profile names",
			caller: person(),
		},
		{
			name:   "agent the profile names",
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

	// An auditor still reads the vMCP, so the refusal is about writing profile configuration.
	if !authorizer.Authorize(httptest.NewRequest(http.MethodGet, "/api/vmcps/vmcp1profiles", nil), person(types.GroupAuditor)) {
		t.Fatal("an auditor cannot read the vMCP")
	}
}
