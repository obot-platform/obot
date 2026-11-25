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

func (r *recordingAuthorizer) StartPreconfiguredOAuth(_ api.Context, vmcpID, componentID string) (types.VMCPPreconfiguredOAuthAuthorization, error) {
	r.started = append(r.started, [2]string{vmcpID, componentID})
	return types.VMCPPreconfiguredOAuthAuthorization{AuthURL: "https://provider.example.com/authorize?state=s"}, nil
}

// preconfiguredOAuthTestVMCP returns a stored vMCP with a remote component, with a profile for agent 7 and one for
// agent 8.
func preconfiguredOAuthTestVMCP() *v1.VMCP {
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	return &v1.VMCP{Name: "vmcp1preconfigured", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
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

// callPreconfiguredOAuth calls the route that reads (GET), authorizes (POST) or revokes (DELETE) the preconfigured
// OAuth of a vMCP's component, and returns its status and body.
func callPreconfiguredOAuth(t *testing.T, handler *VMCPPreconfiguredOAuthHandler, storage *vmcpTestStorage, gatewayClient *gateway.Client, method, vmcpID, componentID string) (int, string, error) {
	t.Helper()
	request := httptest.NewRequest(method, "/api/vmcps/"+vmcpID+"/components/"+componentID+"/preconfigured-oauth", nil)
	request.SetPathValue("vmcp_id", vmcpID)
	request.SetPathValue("component_id", componentID)
	recorder := httptest.NewRecorder()
	call := map[string]api.HandlerFunc{
		http.MethodGet:    handler.Get,
		http.MethodPost:   handler.Authorize,
		http.MethodDelete: handler.Revoke,
	}[method]
	err := call(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           vmcpAdminForTest(),
	})
	return recorder.Code, recorder.Body.String(), err
}

// preconfiguredOAuthState calls the route that reports a component's preconfigured OAuth.
func preconfiguredOAuthState(t *testing.T, handler *VMCPPreconfiguredOAuthHandler, storage *vmcpTestStorage, gatewayClient *gateway.Client, vmcpID, componentID string) types.VMCPPreconfiguredOAuth {
	t.Helper()
	_, body, err := callPreconfiguredOAuth(t, handler, storage, gatewayClient, http.MethodGet, vmcpID, componentID)
	require.NoError(t, err)
	require.NotContains(t, body, "secret-")
	var status types.VMCPPreconfiguredOAuth
	require.NoError(t, json.Unmarshal([]byte(body), &status))
	return status
}

// TestVMCPPreconfiguredOAuthAPI checks the routes of a component's preconfigured OAuth: authorizing starts the
// administrator's authorization of the vMCP's grant for the component and returns where to open it; reading it reports
// who authorized the grant and when, and never its tokens; and revoking deletes the one grant everyone shares, after
// which the component needs authorizing again, and is recorded as the administrator's.
func TestVMCPPreconfiguredOAuthAPI(t *testing.T) {
	vmcp := preconfiguredOAuthTestVMCP()
	// The vMCP controller's probe found that the component asks for OAuth.
	vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{"remote": {
		TargetHash: vmcpconfig.ComponentOAuthTargetHash(*vmcp, vmcp.Spec.Manifest.Components[0]),
		Required:   v1.VMCPComponentOAuthRequired,
	}}
	storage := newVMCPStatusTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	grants := vmcpconfig.NewPreconfiguredOAuthStore(gatewayClient)
	authorizer := &recordingAuthorizer{}
	handler := NewVMCPPreconfiguredOAuthHandler(authorizer, grants)
	needsAuthorization := types.VMCPPreconfiguredOAuth{VMCPID: vmcp.Name, ComponentID: "remote", State: types.VMCPPreconfiguredOAuthNeedsAuthorization}
	require.Equal(t, needsAuthorization, preconfiguredOAuthState(t, handler, storage, gatewayClient, vmcp.Name, "remote"))

	_, body, err := callPreconfiguredOAuth(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "remote")
	require.NoError(t, err)
	var authorization types.VMCPPreconfiguredOAuthAuthorization
	require.NoError(t, json.Unmarshal([]byte(body), &authorization))
	require.Equal(t, "https://provider.example.com/authorize?state=s", authorization.AuthURL)
	require.Equal(t, [][2]string{{vmcp.Name, "remote"}}, authorizer.started)

	// A grant the callback stored, for the vMCP's component, which everyone uses.
	key := kclient.ObjectKeyFromObject(vmcp)
	_, err = grants.Authorize(t.Context(), storage, key, "remote", func(*v1.VMCP, types.VMCPComponent) error { return nil },
		func(ctx context.Context, owner, mcpID, grantID string) error {
			return gatewayClient.ReplaceMCPOAuthGrant(ctx, owner, mcpID, "https://remote.example.com/mcp", grantID, "1", &oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "secret-access-token", RefreshToken: "secret-refresh-token"})
		})
	require.NoError(t, err)
	status := preconfiguredOAuthState(t, handler, storage, gatewayClient, vmcp.Name, "remote")
	require.Equal(t, types.VMCPPreconfiguredOAuthAuthorized, status.State)
	require.Equal(t, "1", status.AuthorizedBy)
	require.NotNil(t, status.AuthorizedAt)
	require.Equal(t, "https://remote.example.com/mcp", status.Target)

	code, body, err := callPreconfiguredOAuth(t, handler, storage, gatewayClient, http.MethodDelete, vmcp.Name, "remote")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, code)
	require.Empty(t, body)
	require.Equal(t, needsAuthorization, preconfiguredOAuthState(t, handler, storage, gatewayClient, vmcp.Name, "remote"))
	_, err = gatewayClient.GetMCPOAuthToken(t.Context(), principal.PreconfiguredOAuthOwner(vmcp.Name), vmcpconfig.PreconfiguredOAuthMCPID(vmcp.Name, "remote"), "https://remote.example.com/mcp")
	require.Error(t, err)
	var stored v1.VMCP
	require.NoError(t, storage.Get(t.Context(), key, &stored))
	require.Empty(t, stored.Status.PreconfiguredOAuth)

	// Revoking again deletes nothing.
	_, _, err = callPreconfiguredOAuth(t, handler, storage, gatewayClient, http.MethodDelete, vmcp.Name, "remote")
	require.NoError(t, err)

	// Only the revocation that deleted a grant is recorded, as the administrator's, against the vMCP, and nothing of the
	// grant is in it.
	events := storedAdminAuditEvents(t, gatewayClient)
	require.Len(t, events, 1)
	require.Equal(t, "admin", events[0].ActorUserID)
	require.Equal(t, "vmcp.preconfigured_oauth.revoke", events[0].Action)
	require.Equal(t, "vmcp", events[0].ResourceKind)
	require.Equal(t, vmcp.Name, events[0].ResourceID)
	require.Empty(t, events[0].ParentID)
	require.Equal(t, map[string]any{"componentID": "remote"}, adminAuditEventDetails(t, events[0]))
	require.NotContains(t, string(events[0].Details), "secret-")
}

// TestVMCPPreconfiguredOAuthForAnyRemoteComponent checks that every remote component of a shared vMCP has a
// preconfigured OAuth state, whichever profiles grant it, agents or only users, with the OAuth requirement the vMCP
// controller's probe found; and that a component that is not remote, an unknown component, a personal vMCP and an
// unknown vMCP have none.
func TestVMCPPreconfiguredOAuthForAnyRemoteComponent(t *testing.T) {
	vmcp := preconfiguredOAuthTestVMCP()
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
	personal := preconfiguredOAuthTestVMCP()
	personal.Name, personal.Spec.UserID = "vmcp1personal", "101"
	personal.Spec.Manifest.Profiles = nil
	storage := newVMCPStatusTestStorage(vmcp, personal)
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPPreconfiguredOAuthHandler(&recordingAuthorizer{}, vmcpconfig.NewPreconfiguredOAuthStore(gatewayClient))

	require.Equal(t, types.VMCPPreconfiguredOAuth{VMCPID: vmcp.Name, ComponentID: "remote", State: types.VMCPPreconfiguredOAuthNeedsAuthorization},
		preconfiguredOAuthState(t, handler, storage, gatewayClient, vmcp.Name, "remote"))
	// Granted only to users, and not yet probed: not known to need OAuth, and still one an administrator can authorize.
	require.Equal(t, types.VMCPPreconfiguredOAuth{VMCPID: vmcp.Name, ComponentID: "users-only", State: types.VMCPPreconfiguredOAuthNotRequired},
		preconfiguredOAuthState(t, handler, storage, gatewayClient, vmcp.Name, "users-only"))

	for _, tt := range []struct {
		vmcpID, componentID string
		code                int
	}{
		{
			vmcpID:      vmcp.Name,
			componentID: apiComponent.ID,
			code:        http.StatusBadRequest,
		},
		{
			vmcpID:      vmcp.Name,
			componentID: "missing",
			code:        http.StatusNotFound,
		},
		{
			vmcpID:      personal.Name,
			componentID: "remote",
			code:        http.StatusBadRequest,
		},
		{
			vmcpID:      "vmcp1missing",
			componentID: "remote",
			code:        http.StatusNotFound,
		},
	} {
		_, _, err := callPreconfiguredOAuth(t, handler, storage, gatewayClient, http.MethodGet, tt.vmcpID, tt.componentID)
		var httpErr *types.ErrHTTP
		require.ErrorAs(t, err, &httpErr, "%s/%s", tt.vmcpID, tt.componentID)
		require.Equal(t, tt.code, httpErr.Code, "%s/%s: %s", tt.vmcpID, tt.componentID, httpErr.Message)
	}
}
