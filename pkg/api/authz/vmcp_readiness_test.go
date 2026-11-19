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

// TestReadinessAuthorization checks the readiness routes: a vMCP's agent setup, and the vMCPs an agent can use and
// whether it is ready for each. Administrators and owners reach them, read-only administrators and auditors may read
// them and change nothing, and no other user does, nor does any agent, including the agent the readiness is about and
// the agent a profile names.
func TestReadinessAuthorization(t *testing.T) {
	vmcp := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp1readiness"),
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
	paths := []string{
		"/api/agents/7/vmcps",
		"/api/agents/7/vmcps/vmcp1readiness/readiness",
		"/api/vmcps/vmcp1readiness/agent-setup",
	}
	for _, tt := range []struct {
		name   string
		caller user.Info
		reads  bool
		writes bool
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
			for _, path := range paths {
				if got := authorizer.Authorize(httptest.NewRequest(http.MethodGet, path, nil), tt.caller); got != tt.reads {
					t.Errorf("GET %s: authorized = %v, want %v", path, got, tt.reads)
				}
				for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
					if got := authorizer.Authorize(httptest.NewRequest(method, path, nil), tt.caller); got != tt.writes {
						t.Errorf("%s %s: authorized = %v, want %v", method, path, got, tt.writes)
					}
				}
			}
		})
	}
}
