package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDeletingAbsentComponentOAuthOverridePreservesAuthorization(t *testing.T) {
	for _, tt := range []struct {
		name        string
		removeFirst bool
	}{
		{
			name: "catalog credentials only",
		},
		{
			name:        "repeated deletion after reauthorization",
			removeFirst: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			component := types.VMCPComponent{
				ID:                      "one",
				MCPServerCatalogEntryID: "entry",
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{StaticOAuthRequired: true},
				}},
			}
			vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}}}}
			server := &v1.MCPServer{Name: "shared", Namespace: vmcp.Namespace, Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: component.ID}}
			storage := newVMCPTestStorage(vmcp, server)
			gw := newHandlerTestGateway(t)
			require.NoError(t, gw.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: system.MCPOAuthCredentialName("entry"),
				Name:    system.StaticOAuthCredentialName,
				Secrets: map[string]string{"CLIENT_ID": "catalog-client"},
			}))
			remove := func() {
				t.Helper()
				req := httptest.NewRequest(http.MethodDelete, "/", nil).WithContext(t.Context())
				req.SetPathValue("vmcp_id", vmcp.Name)
				req.SetPathValue("component_id", component.ID)
				require.NoError(t, NewVMCPHandler(nil).DeleteOAuthCredentials(api.Context{Request: req, ResponseWriter: httptest.NewRecorder(), Storage: storage, GatewayClient: gw, User: &user.DefaultInfo{UID: "admin", Groups: []string{types.GroupAdmin}}}))
			}
			if tt.removeFirst {
				require.NoError(t, gw.UpsertCredential(t.Context(), gatewaytypes.Credential{
					Context: vmcpconfig.ComponentOAuthCredentialContext(vmcp.Name, component.ID),
					Name:    system.StaticOAuthCredentialName,
					Secrets: map[string]string{"CLIENT_ID": "override"},
				}))
				remove()
			}
			conf := &oauth2.Config{ClientID: "catalog-client"}
			require.NoError(t, gw.ReplaceMCPOAuthToken(t.Context(), "user", server.Name, "url", "", conf, &oauth2.Token{AccessToken: "authorized"}))
			require.NoError(t, gw.CreateMCPOAuthPendingState(t.Context(), "user", server.Name, "url", "", "pending", "verifier", "", conf))
			var before v1.VMCP
			require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &before))
			remove()
			token, err := gw.GetMCPOAuthToken(t.Context(), "user", server.Name, "url")
			require.NoError(t, err)
			require.Equal(t, "authorized", token.AccessToken)
			_, err = gw.GetMCPOAuthPendingState(t.Context(), "pending")
			require.NoError(t, err)
			var after v1.VMCP
			require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &after))
			require.Equal(t, before.Annotations, after.Annotations, "no-op deletion must not publish invalidation markers")
		})
	}
}

func TestVMCPComponentOAuthCredentialsIsolationAndFallback(t *testing.T) {
	entry := vmcpCatalogEntryForTest("source")
	entry.Spec.Manifest.RemoteConfig = &types.RemoteCatalogConfig{StaticOAuthRequired: true}
	component := types.VMCPComponent{
		ID:                      "one",
		MCPServerCatalogEntryID: entry.Name,
		CatalogEntry:            types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest},
	}
	first := &v1.VMCP{Name: "vmcp1first", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}}}}
	second := first.DeepCopy()
	second.Name = "vmcp1second"
	instance := &v1.VMCPInstance{Name: "vmcpi1first", Namespace: first.Namespace, Spec: v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: first.Name}}}
	shared := &v1.MCPServer{Name: "shared", Namespace: first.Namespace, Spec: v1.MCPServerSpec{VMCPID: first.Name, VMCPComponentID: component.ID}}
	dedicated := &v1.MCPServer{Name: "dedicated", Namespace: first.Namespace, Spec: v1.MCPServerSpec{VMCPInstanceID: instance.Name, VMCPComponentID: component.ID}}
	unrelated := &v1.MCPServer{Name: "unrelated", Namespace: first.Namespace, Spec: v1.MCPServerSpec{VMCPID: second.Name, VMCPComponentID: component.ID}}
	connection := &v1.MCPServerInstance{Name: "connection", Namespace: first.Namespace, Spec: v1.MCPServerInstanceSpec{MCPServerName: shared.Name}}
	storage := newVMCPTestStorage(entry, first, second, instance, shared, dedicated, unrelated, connection)
	gw := newHandlerTestGateway(t)
	require.NoError(t, gw.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name),
		Name:    system.StaticOAuthCredentialName,
		Secrets: map[string]string{"CLIENT_ID": "catalog-client", "CLIENT_SECRET": "catalog-secret"},
	}))
	handler := NewVMCPHandler(nil)
	call := func(vmcpID, method, body string, action func(api.Context) error) (types.MCPServerOAuthCredentialStatus, error) {
		t.Helper()
		req := httptest.NewRequest(method, "/", strings.NewReader(body)).WithContext(t.Context())
		req.SetPathValue("vmcp_id", vmcpID)
		req.SetPathValue("component_id", component.ID)
		response := httptest.NewRecorder()
		err := action(api.Context{Request: req, ResponseWriter: response, Storage: storage, GatewayClient: gw, User: &user.DefaultInfo{UID: "admin", Groups: []string{types.GroupAdmin}}})
		var status types.MCPServerOAuthCredentialStatus
		if err == nil {
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
		}
		require.NotContains(t, response.Body.String(), "secret")
		return status, err
	}
	status, err := call(first.Name, http.MethodGet, "", handler.GetOAuthCredentials)
	require.NoError(t, err)
	require.Equal(t, "catalog", status.Source)
	for _, id := range []string{shared.Name, dedicated.Name, unrelated.Name, connection.Name} {
		for _, uid := range []string{"alice", "bob"} {
			require.NoError(t, gw.ReplaceMCPOAuthToken(t.Context(), uid, id, "url", "", &oauth2.Config{ClientID: "old"}, &oauth2.Token{AccessToken: "old-token"}))
		}
		require.NoError(t, gw.CreateMCPOAuthPendingState(t.Context(), "alice", id, "url", "", id, "verifier", "", &oauth2.Config{ClientID: "old"}))
	}
	status, err = call(first.Name, http.MethodPost, `{"clientID":"first-client","clientSecret":"first-secret"}`, handler.SetOAuthCredentials)
	require.NoError(t, err)
	require.Equal(t, "component", status.Source)
	require.Equal(t, "first-client", status.ClientID)
	for _, id := range []string{shared.Name, dedicated.Name, unrelated.Name, connection.Name} {
		for _, uid := range []string{"alice", "bob"} {
			_, err := gw.GetMCPOAuthToken(t.Context(), uid, id, "url")
			if id == unrelated.Name {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			}
		}
		_, err := gw.GetMCPOAuthPendingState(t.Context(), id)
		if id == unrelated.Name {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		}
	}
	_, err = call(first.Name, http.MethodPost, `{"clientID":"replacement"}`, handler.SetOAuthCredentials)
	require.ErrorContains(t, err, "already exist")
	status, err = call(second.Name, http.MethodPost, `{"clientID":"second-client"}`, handler.SetOAuthCredentials)
	require.NoError(t, err)
	require.Equal(t, "second-client", status.ClientID)
	for _, server := range []*v1.MCPServer{shared, dedicated, unrelated} {
		contexts, err := vmcpconfig.ServerOAuthCredentialContexts(t.Context(), storage, *server)
		require.NoError(t, err)
		credential, err := gw.RevealCredential(t.Context(), contexts, system.StaticOAuthCredentialName)
		require.NoError(t, err)
		want := "first-client"
		if server == unrelated {
			want = "second-client"
		}
		require.Equal(t, want, credential.Secrets["CLIENT_ID"])
	}
	status, err = call(first.Name, http.MethodDelete, "", handler.DeleteOAuthCredentials)
	require.NoError(t, err)
	require.Equal(t, "catalog", status.Source)
	require.Equal(t, "catalog-client", status.ClientID)
	for _, server := range []*v1.MCPServer{shared, dedicated, unrelated} {
		server.Spec.Manifest.Runtime = types.RuntimeRemote
		require.NoError(t, storage.Update(t.Context(), server))
		require.NoError(t, gw.ReplaceMCPOAuthToken(t.Context(), "user", server.Name, "url", "", &oauth2.Config{ClientID: "current"}, &oauth2.Token{AccessToken: "current-token"}))
	}
	require.NoError(t, vmcpconfig.InvalidateCatalogOAuthTokens(t.Context(), storage, gw, first.Namespace, system.MCPOAuthCredentialName(entry.Name)))
	for _, server := range []*v1.MCPServer{shared, dedicated, unrelated} {
		_, err := gw.GetMCPOAuthToken(t.Context(), "user", server.Name, "url")
		if server == unrelated {
			require.NoError(t, err, "catalog rotation must preserve component overrides")
		} else {
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		}
	}
	require.NoError(t, storage.Delete(t.Context(), entry))
	status, err = call(second.Name, http.MethodGet, "", handler.GetOAuthCredentials)
	require.NoError(t, err)
	require.Equal(t, "second-client", status.ClientID, "override must survive source deletion")
}

func TestVMCPComponentOAuthCredentialValidation(t *testing.T) {
	for _, tt := range []struct {
		name        string
		groups      []string
		sourceURL   string
		runtime     types.Runtime
		componentID string
		body        string
		wantError   string
	}{
		{
			name:        "basic",
			componentID: "one",
			wantError:   "only administrators",
		},
		{
			name:        "power user owner",
			groups:      []string{types.GroupPowerUser},
			componentID: "one",
			wantError:   "only administrators",
		},
		{
			name:        "git managed",
			groups:      []string{types.GroupAdmin},
			sourceURL:   "https://git.example/repo",
			componentID: "one",
			wantError:   "Git-managed",
		},
		{
			name:        "missing component",
			groups:      []string{types.GroupAdmin},
			componentID: "missing",
			wantError:   "not found",
		},
		{
			name:        "non remote",
			groups:      []string{types.GroupAdmin},
			runtime:     types.RuntimeNPX,
			componentID: "one",
			wantError:   "only supported for remote",
		},
		{
			name:        "blank ID",
			groups:      []string{types.GroupAdmin},
			runtime:     types.RuntimeRemote,
			componentID: "one",
			body:        `{"clientID":" "}`,
			wantError:   "clientID is required",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{UserID: "owner", SourceURL: tt.sourceURL, Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{ID: "one", CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Runtime: tt.runtime}}}}}}}
			storage := newVMCPTestStorage(vmcp)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req.SetPathValue("vmcp_id", vmcp.Name)
			req.SetPathValue("component_id", tt.componentID)
			err := NewVMCPHandler(nil).SetOAuthCredentials(api.Context{Request: req, Storage: storage, User: &user.DefaultInfo{UID: "owner", Groups: tt.groups}})
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}
