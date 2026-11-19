package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// recordingAuthorizer records the authorizations it is asked to start.
type recordingAuthorizer struct {
	started [][2]string
}

func (r *recordingAuthorizer) StartAgentOAuthGrant(_ api.Context, vmcpID, componentID string) (types.VMCPAgentOAuthAuthorization, error) {
	r.started = append(r.started, [2]string{vmcpID, componentID})
	return types.VMCPAgentOAuthAuthorization{AuthURL: "https://provider.example.com/authorize?state=s"}, nil
}

// agentOAuthTestVMCP returns a stored vMCP with a remote component, with a profile for agent 7 and one for agent 8.
func agentOAuthTestVMCP() *v1.VMCP {
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	return &v1.VMCP{Name: "vmcp1agentoauth", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{
			ID:   "remote",
			Name: "remote",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://remote.example.com/mcp"},
			}},
		}},
		Profiles: []types.VMCPProfile{
			profile("vp1seven", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			profile("vp1eight", types.Subject{Type: types.SubjectTypeAgent, ID: "8"}),
		},
	}}}
}

// callAgentOAuth calls the route that authorizes (POST) or revokes (DELETE) a vMCP's agent OAuth grant for a component.
func callAgentOAuth(t *testing.T, handler *VMCPAgentOAuthHandler, storage *vmcpTestStorage, gatewayClient *gateway.Client, method, vmcpID, componentID string) (string, error) {
	t.Helper()
	request := httptest.NewRequest(method, "/api/vmcps/"+vmcpID+"/agent-oauth/"+componentID, nil)
	request.SetPathValue("vmcp_id", vmcpID)
	request.SetPathValue("component_id", componentID)
	recorder := httptest.NewRecorder()
	call := handler.Authorize
	if method == http.MethodDelete {
		call = handler.Revoke
	}
	err := call(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           vmcpAdminForTest(),
	})
	return recorder.Body.String(), err
}

// agentSetup calls the route that reports a vMCP's agent setup.
func agentSetup(t *testing.T, handler *VMCPAgentOAuthHandler, storage *vmcpTestStorage, gatewayClient *gateway.Client, vmcpID string) types.VMCPAgentSetup {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/vmcps/"+vmcpID+"/agent-setup", nil)
	request.SetPathValue("vmcp_id", vmcpID)
	recorder := httptest.NewRecorder()
	require.NoError(t, handler.Setup(api.Context{ResponseWriter: recorder, Request: request, Storage: storage, GatewayClient: gatewayClient, User: vmcpAdminForTest()}))
	var setup types.VMCPAgentSetup
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &setup))
	return setup
}

// TestVMCPAgentOAuthAPI checks the routes that authorize and revoke a vMCP's agent OAuth grant: authorizing starts the
// administrator's authorization of the vMCP's grant for the component and returns where to open it, and revoking
// deletes the one grant every agent shares and reports the vMCP's agent setup, which names who authorized a grant and
// never its tokens, and in which the component then needs authorizing again.
func TestVMCPAgentOAuthAPI(t *testing.T) {
	vmcp := agentOAuthTestVMCP()
	// The vMCP controller's probe found that the component asks for OAuth.
	vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{"remote": {
		TargetHash: vmcpconfig.ComponentOAuthTargetHash(*vmcp, vmcp.Spec.Manifest.Components[0]),
		Required:   v1.VMCPComponentOAuthRequired,
	}}
	storage := newVMCPStatusTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	grants := vmcpconfig.NewAgentOAuthGrantStore(gatewayClient)
	authorizer := &recordingAuthorizer{}
	handler := NewVMCPAgentOAuthHandler(authorizer, grants)

	body, err := callAgentOAuth(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "remote")
	require.NoError(t, err)
	var authorization types.VMCPAgentOAuthAuthorization
	require.NoError(t, json.Unmarshal([]byte(body), &authorization))
	require.Equal(t, "https://provider.example.com/authorize?state=s", authorization.AuthURL)
	require.Equal(t, [][2]string{{vmcp.Name, "remote"}}, authorizer.started)

	// A grant the callback stored, for the vMCP's component, whichever agent uses it.
	key := kclient.ObjectKeyFromObject(vmcp)
	_, err = grants.Authorize(t.Context(), storage, key, "remote", func(*v1.VMCP, types.VMCPComponent) error { return nil },
		func(ctx context.Context, owner, mcpID, grantID string) error {
			return gatewayClient.ReplaceMCPOAuthGrant(ctx, owner, mcpID, "https://remote.example.com/mcp", grantID, "1", &oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "secret-access-token", RefreshToken: "secret-refresh-token"})
		})
	require.NoError(t, err)
	setup := agentSetup(t, handler, storage, gatewayClient, vmcp.Name)
	require.Len(t, setup.Components, 1)
	require.True(t, setup.Components[0].Ready)
	require.Equal(t, types.VMCPAgentOAuthAuthorized, setup.Components[0].OAuth.State)
	require.Equal(t, "1", setup.Components[0].OAuth.AuthorizedBy)

	body, err = callAgentOAuth(t, handler, storage, gatewayClient, http.MethodDelete, vmcp.Name, "remote")
	require.NoError(t, err)
	require.NotContains(t, body, "secret-")
	setup = types.VMCPAgentSetup{}
	require.NoError(t, json.Unmarshal([]byte(body), &setup))
	require.Equal(t, vmcp.Name, setup.VMCPID)
	require.Equal(t, []types.VMCPAgentSetupComponent{{
		ComponentID: "remote",
		Reasons:     []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired}},
		OAuth:       &types.VMCPAgentOAuthStatus{Required: true, State: types.VMCPAgentOAuthMissing},
	}}, setup.Components)
	_, err = gatewayClient.GetMCPOAuthToken(t.Context(), principal.AgentOAuthGrantOwner(vmcp.Name), vmcpconfig.AgentOAuthGrantMCPID(vmcp.Name, "remote"), "https://remote.example.com/mcp")
	require.Error(t, err)
	var stored v1.VMCP
	require.NoError(t, storage.Get(t.Context(), key, &stored))
	require.Empty(t, stored.Status.AgentOAuthGrants)

	// Revoking again deletes nothing.
	_, err = callAgentOAuth(t, handler, storage, gatewayClient, http.MethodDelete, vmcp.Name, "remote")
	require.NoError(t, err)

	// Only the revocation that deleted a grant is recorded, as the administrator's, against the vMCP, and nothing of the
	// grant is in it.
	events := storedAdminAuditEvents(t, gatewayClient)
	require.Len(t, events, 1)
	require.Equal(t, "admin", events[0].ActorUserID)
	require.Equal(t, "vmcp.agent_oauth.revoke", events[0].Action)
	require.Equal(t, "vmcp", events[0].ResourceKind)
	require.Equal(t, vmcp.Name, events[0].ResourceID)
	require.Empty(t, events[0].ParentID)
	require.Equal(t, map[string]any{"componentID": "remote"}, adminAuditEventDetails(t, events[0]))
	require.NotContains(t, string(events[0].Details), "secret-")
}

// TestVMCPAgentSetupCoversOnlyAgentGrantedComponents checks the agent setup route of a vMCP whose profiles grant agents
// some of its components: the setup lists each component that some profile naming an agent grants, once, however many
// do, with the OAuth requirement the vMCP controller's probe found and the configuration agents need and cannot be
// given, and no component that only a profile for users grants. A vMCP whose profiles name no agent has no setup.
func TestVMCPAgentSetupCoversOnlyAgentGrantedComponents(t *testing.T) {
	vmcp := agentOAuthTestVMCP()
	apiComponent := agentProfilesTestVMCP().Spec.Manifest.Components[0]
	usersOnly := vmcp.Spec.Manifest.Components[0]
	usersOnly.ID, usersOnly.Name = "users-only", "users-only"
	vmcp.Spec.Manifest.Components = append(vmcp.Spec.Manifest.Components, apiComponent, usersOnly)
	vmcp.Spec.Manifest.Profiles[0].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"remote": {}}}
	vmcp.Spec.Manifest.Profiles[1].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"remote": {}, apiComponent.ID: {}}}
	vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, types.VMCPProfile{
		ID:          "vp1users",
		Name:        "users",
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	})
	vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{"remote": {
		TargetHash: vmcpconfig.ComponentOAuthTargetHash(*vmcp, vmcp.Spec.Manifest.Components[0]),
		Required:   v1.VMCPComponentOAuthRequired,
	}}
	unnamed := agentOAuthTestVMCP()
	unnamed.Name = "vmcp1unnamed"
	unnamed.Spec.Manifest.Profiles = vmcp.Spec.Manifest.Profiles[2:]
	storage := newVMCPStatusTestStorage(vmcp, unnamed)
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPAgentOAuthHandler(&recordingAuthorizer{}, vmcpconfig.NewAgentOAuthGrantStore(gatewayClient))

	require.Equal(t, types.VMCPAgentSetup{
		VMCPID: vmcp.Name,
		Components: []types.VMCPAgentSetupComponent{
			{
				ComponentID: "remote",
				Reasons:     []types.AgentReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired}},
				OAuth:       &types.VMCPAgentOAuthStatus{Required: true, State: types.VMCPAgentOAuthMissing},
			},
			{
				ComponentID: apiComponent.ID,
				Reasons:     []types.AgentReadinessReason{{ComponentID: apiComponent.ID, Code: types.AgentReadinessConfigurationRequired, Field: "REGION"}},
			},
		},
	}, agentSetup(t, handler, storage, gatewayClient, vmcp.Name))
	require.Equal(t, types.VMCPAgentSetup{VMCPID: unnamed.Name, Components: []types.VMCPAgentSetupComponent{}}, agentSetup(t, handler, storage, gatewayClient, unnamed.Name))

	// An unknown vMCP has no setup.
	request := httptest.NewRequest(http.MethodGet, "/api/vmcps/vmcp1missing/agent-setup", nil)
	request.SetPathValue("vmcp_id", "vmcp1missing")
	err := handler.Setup(api.Context{ResponseWriter: httptest.NewRecorder(), Request: request, Storage: storage, GatewayClient: gatewayClient, User: vmcpAdminForTest()})
	var notFound *types.ErrHTTP
	require.ErrorAs(t, err, &notFound)
	require.Equal(t, http.StatusNotFound, notFound.Code)
}
