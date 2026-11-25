package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	agentVMCPGranted   = "vmcp1granted"
	agentVMCPWildcard  = "vmcp1agents"
	agentVMCPEveryone  = "vmcp1everyone"
	agentVMCPUsersOnly = "vmcp1users"
	agentVMCPOther     = "vmcp1otheragent"
	agentVMCPPersonal  = "vmcp1personal"
	agentVMCPEmpty     = "vmcp1empty"
	agentInstance      = "vmcpi1agent"
	agentComponent     = "ms1agentcomponent"
	agentConnection    = "msi1agentconnection"
	agentSharedServer  = "ms1agentshared"
	agentStandalone    = "ms1agentstandalone"
	agentStandaloneMSI = "msi1agentstandalone"
	agentMultiUser     = "ms1multiuser"
)

func agentVMCP(name string, subjects ...types.Subject) *v1.VMCP {
	profiles := make([]types.VMCPProfile, 0, len(subjects))
	for _, subject := range subjects {
		profiles = append(profiles, types.VMCPProfile{
			ID:          "vp1" + name + subject.ID,
			Subjects:    []types.Subject{subject},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		})
	}
	return &v1.VMCP{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{
				{
					ID:              "dedicated",
					ForceSingleUser: true,
				},
				{
					ID: "shared",
				},
			},
			Profiles: profiles,
		}},
	}
}

// newAgentVMCPTestAuthorizer returns an authorizer over a vMCP that names agent 7, and over vMCPs and MCP resources that
// grant it nothing: a vMCP for every user, one with a stored agent wildcard, which names no agent, one for users only,
// one for another agent, a personal vMCP of user 7, one without components, and servers and connections owned by the
// agent outside any vMCP, which nothing creates but which must not let it in either.
func newAgentVMCPTestAuthorizer(t *testing.T) *Authorizer {
	t.Helper()

	personal := agentVMCP(agentVMCPPersonal)
	personal.Spec.UserID = "7"
	empty := agentVMCP(agentVMCPEmpty, types.Subject{Type: types.SubjectTypeAgent, ID: "7"})
	empty.Spec.Manifest.Components = nil
	storage := newMCPIDIsAuthorizedTestStorage(
		agentVMCP(agentVMCPGranted, types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
		agentVMCP(agentVMCPWildcard, types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
		agentVMCP(agentVMCPEveryone, types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}),
		agentVMCP(agentVMCPUsersOnly, types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}, types.Subject{Type: types.SubjectTypeUser, ID: "7"}),
		agentVMCP(agentVMCPOther, types.Subject{Type: types.SubjectTypeAgent, ID: "8"}),
		personal,
		empty,
		&v1.VMCPInstance{
			Name:      agentInstance,
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPInstanceSpec{
				UserID:   "agent:7",
				Manifest: types.VMCPInstanceManifest{VMCPID: agentVMCPGranted},
			},
		},
		&v1.MCPServer{
			Name:      agentComponent,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				UserID:          "agent:7",
				VMCPInstanceID:  agentInstance,
				VMCPComponentID: "dedicated",
			},
		},
		&v1.MCPServer{
			Name:      agentSharedServer,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				VMCPID:          agentVMCPGranted,
				VMCPComponentID: "shared",
			},
		},
		&v1.MCPServerInstance{
			Name:      agentConnection,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:          "agent:7",
				MCPServerName:   agentSharedServer,
				VMCPInstanceID:  agentInstance,
				VMCPComponentID: "shared",
			},
		},
		&v1.MCPServer{
			Name:      agentStandalone,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				UserID: "agent:7",
			},
		},
		&v1.MCPServer{
			Name:      agentMultiUser,
			Namespace: system.DefaultNamespace,
		},
		&v1.MCPServerInstance{
			Name:      agentStandaloneMSI,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:        "agent:7",
				MCPServerName: agentMultiUser,
			},
		},
	)
	return NewAuthorizer(nil, storage, storage, false, nil, nil, nil)
}

// TestAgentVMCPConnectAuthorization is the authorization matrix of an agent's MCP connections: an agent connects to a
// shared vMCP that a profile grants it, through its own instance too, and reaches components only through the
// composite loopback of its own connection. It never reaches a personal vMCP, a vMCP no profile grants it, or an MCP
// resource outside a vMCP, even one it owns.
func TestAgentVMCPConnectAuthorization(t *testing.T) {
	authorizer := newAgentVMCPTestAuthorizer(t)
	agentKey := agentKeyPrincipal(gatewaytypes.APIKeyScopes{MCPServerIDs: []string{"*"}})
	loopback := func(id uint, authorized ...string) user.Info {
		return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: id}, []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, map[string][]string{
			"authorized_mcp_ids": authorized,
		})
	}
	for _, tt := range []struct {
		name   string
		caller user.Info
		path   string
		want   bool
	}{
		{
			name:   "vMCP granted to the agent",
			caller: agentKey,
			path:   "/mcp-connect/" + agentVMCPGranted,
			want:   true,
		},
		{
			// Validation refuses the agent wildcard, but a vMCP stored before agents were named only by ID may hold it.
			name:   "vMCP with a stored agent wildcard",
			caller: agentKey,
			path:   "/mcp-connect/" + agentVMCPWildcard,
		},
		{
			name:   "vMCP granted to every user",
			caller: agentKey,
			path:   "/mcp-connect/" + agentVMCPEveryone,
		},
		{
			name:   "the agent's own instance",
			caller: agentKey,
			path:   "/mcp-connect/" + agentInstance,
			want:   true,
		},
		{
			name:   "vMCP granted to users only",
			caller: agentKey,
			path:   "/mcp-connect/" + agentVMCPUsersOnly,
		},
		{
			name:   "vMCP granted to another agent",
			caller: agentKey,
			path:   "/mcp-connect/" + agentVMCPOther,
		},
		{
			// The personal vMCP of the user with the agent's numeric ID.
			name:   "personal vMCP",
			caller: agentKey,
			path:   "/mcp-connect/" + agentVMCPPersonal,
		},
		{
			name:   "vMCP without components",
			caller: agentKey,
			path:   "/mcp-connect/" + agentVMCPEmpty,
		},
		{
			name:   "component server without the composite loopback",
			caller: agentKey,
			path:   "/mcp-connect/" + agentComponent,
		},
		{
			name:   "component server through the composite loopback",
			caller: loopback(7, agentComponent, agentConnection, agentInstance),
			path:   "/mcp-connect/" + agentComponent,
			want:   true,
		},
		{
			name:   "shared component connection through the composite loopback",
			caller: loopback(7, agentComponent, agentConnection, agentInstance),
			path:   "/mcp-connect/" + agentConnection,
			want:   true,
		},
		{
			// Each component is reached through the caller's own connection, never a shared server directly.
			name:   "shared component server through the composite loopback",
			caller: loopback(7, agentSharedServer),
			path:   "/mcp-connect/" + agentSharedServer,
		},
		{
			name:   "another agent's component server through its composite loopback",
			caller: loopback(8, agentComponent, agentConnection, agentInstance),
			path:   "/mcp-connect/" + agentComponent,
		},
		{
			name:   "agent-owned server outside a vMCP",
			caller: loopback(7, agentStandalone),
			path:   "/mcp-connect/" + agentStandalone,
		},
		{
			name:   "agent-owned connection outside a vMCP",
			caller: loopback(7, agentStandaloneMSI),
			path:   "/mcp-connect/" + agentStandaloneMSI,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := authorizer.Authorize(httptest.NewRequest(http.MethodPost, tt.path, nil), tt.caller); got != tt.want {
				t.Fatalf("authorized = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAgentNeverReadsPersonalVMCPs checks the vMCP visibility model for an agent and the user with its numeric ID.
func TestAgentNeverReadsPersonalVMCPs(t *testing.T) {
	personal := agentVMCP(agentVMCPPersonal)
	personal.Spec.UserID = "7"
	agent := agentKeyPrincipal(gatewaytypes.APIKeyScopes{MCPServerIDs: []string{"*"}})
	if UserCanReadVMCP(agent, personal) || UserCanConnectVMCP(agent, personal) || UserCanManageVMCP(agent, personal) {
		t.Fatal("an agent reads the personal vMCP of the user with its ID")
	}
	owner := principaltest.AsUser(&user.DefaultInfo{UID: "7"})
	if !UserCanReadVMCP(owner, personal) || !UserCanConnectVMCP(owner, personal) {
		t.Fatal("the owner cannot read their personal vMCP")
	}
}

// TestAgentConnectFollowsProfileAccess checks that removing an agent from a vMCP's profiles refuses its next connect,
// through the vMCP and through the instance it created, without reissuing its key.
func TestAgentConnectFollowsProfileAccess(t *testing.T) {
	authorizer := newAgentVMCPTestAuthorizer(t)
	agent := agentKeyPrincipal(gatewaytypes.APIKeyScopes{MCPServerIDs: []string{"*"}})
	for _, path := range []string{"/mcp-connect/" + agentVMCPGranted, "/mcp-connect/" + agentInstance} {
		if !authorizer.Authorize(httptest.NewRequest(http.MethodPost, path, nil), agent) {
			t.Fatalf("%s: the granted agent was denied", path)
		}
	}

	var vmcp v1.VMCP
	if err := authorizer.uncached.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: agentVMCPGranted}, &vmcp); err != nil {
		t.Fatal(err)
	}
	vmcp.Spec.Manifest.Profiles[0].Subjects = []types.Subject{{Type: types.SubjectTypeAgent, ID: "8"}}
	if err := authorizer.uncached.Update(t.Context(), &vmcp); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/mcp-connect/" + agentVMCPGranted, "/mcp-connect/" + agentInstance} {
		if authorizer.Authorize(httptest.NewRequest(http.MethodPost, path, nil), agent) {
			t.Fatalf("%s: the agent was authorized after losing its profile", path)
		}
	}
}

// TestAgentsDoNotManageVMCPInstances checks that an agent never reaches the vMCP instance API, not even for its own
// instance: it has no API scope, and it cannot configure, reveal or test a connection.
func TestAgentsDoNotManageVMCPInstances(t *testing.T) {
	authorizer := newAgentVMCPTestAuthorizer(t)
	agent := agentKeyPrincipal(gatewaytypes.APIKeyScopes{
		CanAccessSkills:   true,
		CanAccessLLMProxy: true,
		MCPServerIDs:      []string{"*"},
	})
	for _, route := range []agentTestRoute{
		{method: http.MethodGet, path: "/api/vmcp-instances"},
		{method: http.MethodPost, path: "/api/vmcp-instances"},
		{method: http.MethodGet, path: "/api/vmcp-instances/" + agentInstance},
		{method: http.MethodPut, path: "/api/vmcp-instances/" + agentInstance},
		{method: http.MethodDelete, path: "/api/vmcp-instances/" + agentInstance},
		{method: http.MethodPost, path: "/api/vmcp-instances/" + agentInstance + "/configure"},
		{method: http.MethodPost, path: "/api/vmcp-instances/" + agentInstance + "/reveal"},
		{method: http.MethodPost, path: "/api/vmcp-instances/" + agentInstance + "/deconfigure"},
		{method: http.MethodPost, path: "/api/vmcp-instances/" + agentInstance + "/tester/chat"},
		{method: http.MethodPost, path: "/api/vmcps/" + agentVMCPGranted + "/launch"},
		{method: http.MethodPost, path: "/api/vmcps/" + agentVMCPGranted + "/check-oauth"},
		{method: http.MethodGet, path: "/api/vmcps/" + agentVMCPGranted + "/oauth-url"},
		{method: http.MethodPost, path: "/api/vmcps/" + agentVMCPGranted + "/reveal"},
		{method: http.MethodGet, path: "/api/oauth/vmcp/" + agentVMCPGranted},
	} {
		if authorizer.Authorize(agentTestRequest(route), agent) {
			t.Errorf("%s %s: the agent was authorized", route.method, route.path)
		}
	}
}

// TestCheckMCPIDAccessConfinesAgentsToVMCPs checks the access check that API handlers share with the gateway: for an
// agent, it accepts shared vMCPs that grant it and nothing outside a vMCP, even what the agent owns, and no system
// MCP server.
func TestCheckMCPIDAccessConfinesAgentsToVMCPs(t *testing.T) {
	authorizer := newAgentVMCPTestAuthorizer(t)
	if err := authorizer.uncached.Create(t.Context(), &v1.SystemMCPServer{Name: "sms1hooks", Namespace: system.DefaultNamespace}); err != nil {
		t.Fatal(err)
	}
	agent := agentKeyPrincipal(gatewaytypes.APIKeyScopes{MCPServerIDs: []string{"*"}})
	for _, tt := range []struct {
		mcpID string
		want  bool
	}{
		{
			mcpID: agentVMCPGranted,
			want:  true,
		},
		{
			mcpID: agentStandalone,
		},
		{
			mcpID: agentStandaloneMSI,
		},
		{
			mcpID: "sms1hooks",
		},
	} {
		t.Run(tt.mcpID, func(t *testing.T) {
			got, err := CheckMCPIDAccess(t.Context(), authorizer.uncached, nil, agent, tt.mcpID)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("access = %v, want %v", got, tt.want)
			}
		})
	}
}
