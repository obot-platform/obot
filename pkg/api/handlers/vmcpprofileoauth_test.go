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
	started [][3]string
}

func (r *recordingAuthorizer) StartProfileOAuthGrant(_ api.Context, vmcpID, profileID, componentID string) (types.VMCPProfileOAuthAuthorization, error) {
	r.started = append(r.started, [3]string{vmcpID, profileID, componentID})
	return types.VMCPProfileOAuthAuthorization{AuthURL: "https://provider.example.com/authorize?state=s"}, nil
}

// profileOAuthTestVMCP returns a stored vMCP with a remote component, with a profile for agent 7 and one for agent 8.
func profileOAuthTestVMCP() *v1.VMCP {
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	return &v1.VMCP{Name: "vmcp1profileoauth", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
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

func callProfileOAuth(t *testing.T, handler *VMCPProfileOAuthHandler, storage *vmcpTestStorage, gatewayClient *gateway.Client, method, vmcpID, profileID, componentID string) (string, error) {
	t.Helper()
	request := httptest.NewRequest(method, "/api/vmcps/"+vmcpID+"/profiles/"+profileID+"/components/"+componentID+"/oauth", nil)
	request.SetPathValue("vmcp_id", vmcpID)
	request.SetPathValue("profile_id", profileID)
	request.SetPathValue("component_id", componentID)
	recorder := httptest.NewRecorder()
	call := handler.Authorize
	if method == http.MethodDelete {
		call = handler.Reset
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

// TestVMCPProfileOAuthAPI checks the routes that authorize and reset a profile's grant: authorizing starts the
// administrator's authorization and returns where to open it, and resetting deletes the grant and reports the
// profile's status, which names who authorized a grant and never its tokens.
func TestVMCPProfileOAuthAPI(t *testing.T) {
	vmcp := profileOAuthTestVMCP()
	storage := newProfileConfigurationTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	grants := vmcpconfig.NewProfileOAuthGrantStore(gatewayClient)
	authorizer := &recordingAuthorizer{}
	handler := NewVMCPProfileOAuthHandler(authorizer, grants)

	body, err := callProfileOAuth(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "vp1eight", "remote")
	require.NoError(t, err)
	var authorization types.VMCPProfileOAuthAuthorization
	require.NoError(t, json.Unmarshal([]byte(body), &authorization))
	require.Equal(t, "https://provider.example.com/authorize?state=s", authorization.AuthURL)
	require.Equal(t, [][3]string{{vmcp.Name, "vp1eight", "remote"}}, authorizer.started)

	// A grant the callback stored.
	key := kclient.ObjectKeyFromObject(vmcp)
	_, err = grants.Authorize(t.Context(), storage, key, "vp1eight", "remote", func(*v1.VMCP, types.VMCPProfile, types.VMCPComponent) error { return nil },
		func(ctx context.Context, owner, mcpID, grantID string) error {
			return gatewayClient.ReplaceMCPOAuthGrant(ctx, owner, mcpID, "https://remote.example.com/mcp", grantID, "1", &oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "secret-access-token", RefreshToken: "secret-refresh-token"})
		})
	require.NoError(t, err)

	body, err = callProfileOAuth(t, handler, storage, gatewayClient, http.MethodDelete, vmcp.Name, "vp1seven", "remote")
	require.NoError(t, err)
	var status types.VMCPProfileStatus
	require.NoError(t, json.Unmarshal([]byte(body), &status))
	require.Equal(t, types.VMCPProfileOAuthMissing, status.Components[0].OAuth.State)
	_, err = gatewayClient.GetMCPOAuthToken(t.Context(), principal.ProfileGrant(vmcp.Name, "vp1eight"), vmcpconfig.ProfileOAuthGrantMCPID(vmcp.Name, "remote"), "https://remote.example.com/mcp")
	require.NoError(t, err, "resetting one profile's grant deleted another's")

	body, err = callProfileOAuth(t, handler, storage, gatewayClient, http.MethodDelete, vmcp.Name, "vp1eight", "remote")
	require.NoError(t, err)
	require.NotContains(t, body, "secret-")
	status = types.VMCPProfileStatus{}
	require.NoError(t, json.Unmarshal([]byte(body), &status))
	require.Equal(t, "vp1eight", status.ProfileID)
	require.Equal(t, types.VMCPProfileOAuthMissing, status.Components[0].OAuth.State)
	_, err = gatewayClient.GetMCPOAuthToken(t.Context(), principal.ProfileGrant(vmcp.Name, "vp1eight"), vmcpconfig.ProfileOAuthGrantMCPID(vmcp.Name, "remote"), "https://remote.example.com/mcp")
	require.Error(t, err)
	var stored v1.VMCP
	require.NoError(t, storage.Get(t.Context(), key, &stored))
	require.Empty(t, stored.Status.ProfileOAuthGrants)

	// Only the reset that deleted a grant is recorded, as the administrator's, and nothing of the grant is in it.
	events := storedAdminAuditEvents(t, gatewayClient)
	require.Len(t, events, 1)
	require.Equal(t, "admin", events[0].ActorUserID)
	require.Equal(t, "vmcp.profile.oauth.reset", events[0].Action)
	require.Equal(t, "vmcpProfile", events[0].ResourceKind)
	require.Equal(t, "vp1eight", events[0].ResourceID)
	require.Equal(t, vmcp.Name, events[0].ParentID)
	require.Equal(t, map[string]any{"componentID": "remote"}, adminAuditEventDetails(t, events[0]))
	require.NotContains(t, string(events[0].Details), "secret-")
}

// TestVMCPProfileStatusCoversOnlyGrantedComponents checks the profile status route and the write routes of a profile
// that grants some of the vMCP's components: the status lists only the components the profile grants, with the OAuth
// requirement the vMCP controller's probe found, and configuration of a component it does not grant is refused with a
// 400 that names the component.
func TestVMCPProfileStatusCoversOnlyGrantedComponents(t *testing.T) {
	vmcp := profileOAuthTestVMCP()
	vmcp.Spec.Manifest.Components = append(vmcp.Spec.Manifest.Components, profileConfigurationTestVMCP().Spec.Manifest.Components[0])
	vmcp.Spec.Manifest.Profiles[0].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"remote": {}}}
	vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{"remote": {
		DefinitionHash: vmcpconfig.ProfileOAuthGrantDefinitionHash(vmcp.Spec.Manifest.Components[0]),
		Probes: []v1.VMCPComponentOAuthProbe{{
			URLDigest: vmcpconfig.ComponentOAuthURLDigest(vmcp.Name, "remote", "https://remote.example.com/mcp"),
			Required:  v1.VMCPComponentOAuthRequired,
		}},
	}}
	storage := newProfileConfigurationTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPProfileOAuthHandler(&recordingAuthorizer{}, vmcpconfig.NewProfileOAuthGrantStore(gatewayClient))
	status := func(profileID string) types.VMCPProfileStatus {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/vmcps/"+vmcp.Name+"/profiles/"+profileID+"/status", nil)
		request.SetPathValue("vmcp_id", vmcp.Name)
		request.SetPathValue("profile_id", profileID)
		recorder := httptest.NewRecorder()
		require.NoError(t, handler.Status(api.Context{ResponseWriter: recorder, Request: request, Storage: storage, GatewayClient: gatewayClient, User: vmcpAdminForTest()}))
		var result types.VMCPProfileStatus
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
		return result
	}
	componentIDs := func(status types.VMCPProfileStatus) []string {
		var ids []string
		for _, component := range status.Components {
			ids = append(ids, component.ComponentID)
		}
		return ids
	}

	seven := status("vp1seven")
	require.Equal(t, []string{"remote"}, componentIDs(seven))
	require.Equal(t, &types.VMCPProfileOAuthStatus{Required: true, State: types.VMCPProfileOAuthMissing}, seven.Components[0].OAuth)
	require.Equal(t, []string{"remote", "api"}, componentIDs(status("vp1eight")))

	configuration := NewVMCPProfileConfigurationHandler(vmcpconfig.NewProfileConfigurationStore(gatewayClient))
	_, err := callProfileConfiguration(t, configuration, storage, gatewayClient, http.MethodPut, vmcp.Name, "vp1seven", map[string]any{
		"components": map[string]any{"api": map[string]any{"REGION": "us-east"}},
	})
	requireHTTPStatus(t, err, http.StatusBadRequest)
	require.ErrorContains(t, err, `does not grant component "api"`)
	_, err = callProfileConfiguration(t, configuration, storage, gatewayClient, http.MethodPut, vmcp.Name, "vp1eight", map[string]any{
		"components": map[string]any{"api": map[string]any{"REGION": "us-east"}},
	})
	require.NoError(t, err)
}
