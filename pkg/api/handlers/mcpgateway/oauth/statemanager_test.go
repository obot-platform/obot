package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/nanobot/pkg/safehttp"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStateManagerBlocksStaticCatalogTokenExchangeToPrivateAddress(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	client := newStateManagerTestClient(t, entryName, mcpID)
	require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entryName), Name: "oauth",
		Secrets: map[string]string{
			"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-1",
			"MCP_URL": mcpURL, "GENERATION": "generation-1",
		},
	}))
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("restricted client reached the private token endpoint")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(provider.Close)
	manager := newStateManager(client, safehttp.NewClient(true, true, true))
	config := &oauth2.Config{
		ClientID: "client-1", ClientSecret: "secret-1",
		Endpoint: oauth2.Endpoint{AuthURL: "https://provider.example/authorize", TokenURL: provider.URL},
	}
	require.NoError(t, manager.store(t.Context(), "user-1", mcpID, mcpURL, "request-1", entryName, "state-private-token", "verifier-1", config))

	_, _, err := manager.createToken(t.Context(), "state-private-token", "code-1", "", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to exchange code")
}

func TestStateManagerFencesCallbackWriteAfterProviderExchange(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	for _, tc := range []struct {
		name       string
		changeApp  func(*testing.T, *gateway.Client)
		wantErr    bool
		wantAccess string
	}{
		{name: "ordinary callback succeeds", wantAccess: "provider-access"},
		{
			name:    "rotation during exchange rejects old app grant",
			wantErr: true,
			changeApp: func(t *testing.T, client *gateway.Client) {
				t.Helper()
				require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
					Context: system.MCPOAuthCredentialName(entryName), Name: "oauth",
					Secrets: map[string]string{
						"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-2",
						"MCP_URL": mcpURL, "GENERATION": "generation-2",
					},
				}))
				require.NoError(t, client.DeleteMCPOAuthTokenForAllUsers(t.Context(), mcpID))
			},
		},
		{
			name:    "clear during exchange rejects old app grant",
			wantErr: true,
			changeApp: func(t *testing.T, client *gateway.Client) {
				t.Helper()
				deleted, err := client.DeleteCredential(t.Context(), system.MCPOAuthCredentialName(entryName), "oauth")
				require.NoError(t, err)
				require.True(t, deleted)
				require.NoError(t, client.DeleteMCPOAuthTokenForAllUsers(t.Context(), mcpID))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newStateManagerTestClient(t, entryName, mcpID)
			require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: system.MCPOAuthCredentialName(entryName), Name: "oauth",
				Secrets: map[string]string{
					"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-1",
					"MCP_URL": mcpURL, "GENERATION": "generation-1",
				},
			}))

			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.changeApp != nil {
					release, err := client.AcquireCredentialLock(t.Context(), system.MCPOAuthCredentialName(entryName))
					require.NoError(t, err)
					tc.changeApp(t, client)
					release()
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"access_token":"provider-access","refresh_token":"provider-refresh","token_type":"Bearer"}`)
			}))
			t.Cleanup(provider.Close)

			manager := newStateManager(client)
			config := &oauth2.Config{
				ClientID:     "client-1",
				ClientSecret: "secret-1",
				Endpoint: oauth2.Endpoint{
					AuthURL:  provider.URL + "/authorize",
					TokenURL: provider.URL,
				},
				RedirectURL: "https://obot.example/oauth/mcp/callback",
			}
			require.NoError(t, manager.store(t.Context(), "user-1", mcpID, mcpURL, "request-1", entryName, "state-1", "verifier-1", config))

			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			_, _, err := manager.createToken(ctx, "state-1", "code-1", "", "")
			if tc.wantErr {
				require.ErrorIs(t, err, gateway.ErrMCPOAuthCatalogCredentialChanged)
				_, err = client.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL)
				require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
				_, err = client.GetMCPOAuthPendingState(t.Context(), "state-1")
				require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
				return
			}
			require.NoError(t, err)
			stored, err := client.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL)
			require.NoError(t, err)
			require.Equal(t, tc.wantAccess, stored.AccessToken)
			require.Equal(t, entryName, stored.CatalogEntryName)
		})
	}
}

func TestStateManagerRejectsLegacyStaticPendingState(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	client := newStateManagerTestClient(t, entryName, mcpID)
	require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entryName), Name: "oauth",
		Secrets: map[string]string{
			"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-1",
			"MCP_URL": mcpURL, "GENERATION": "generation-1",
		},
	}))
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"legacy-access","refresh_token":"legacy-refresh","token_type":"Bearer"}`)
	}))
	t.Cleanup(provider.Close)
	config := &oauth2.Config{
		ClientID: "client-1", ClientSecret: "secret-1",
		Endpoint: oauth2.Endpoint{AuthURL: provider.URL + "/authorize", TokenURL: provider.URL},
	}
	manager := newStateManager(client)
	// An empty catalog entry models a pending row created before the fence columns existed.
	require.NoError(t, manager.store(t.Context(), "user-1", mcpID, mcpURL, "request-1", "", "legacy-state", "verifier-1", config))

	_, _, err := manager.createToken(t.Context(), "legacy-state", "code-1", "", "")
	require.ErrorIs(t, err, gateway.ErrMCPOAuthCatalogCredentialChanged)
	_, err = client.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = client.GetMCPOAuthPendingState(t.Context(), "legacy-state")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestStateManagerPreservesLegacyDynamicPendingState(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	client := newStateManagerTestClientWithStaticRequirement(t, entryName, mcpID, false)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"dynamic-access","token_type":"Bearer"}`)
	}))
	t.Cleanup(provider.Close)
	config := &oauth2.Config{
		ClientID: "dynamic-client", ClientSecret: "dynamic-secret",
		Endpoint: oauth2.Endpoint{AuthURL: provider.URL + "/authorize", TokenURL: provider.URL},
	}
	manager := newStateManager(client)
	require.NoError(t, manager.store(t.Context(), "user-1", mcpID, mcpURL, "request-1", "", "legacy-dynamic", "verifier-1", config))
	_, _, err := manager.createToken(t.Context(), "legacy-dynamic", "code-1", "", "")
	require.NoError(t, err)
	stored, err := client.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL)
	require.NoError(t, err)
	require.Equal(t, "dynamic-access", stored.AccessToken)
	require.Empty(t, stored.CatalogEntryName)
}

func TestStateManagerPersistsDirectDynamicAndCIMDCallbacks(t *testing.T) {
	const (
		mcpID  = "direct-mcp-server"
		mcpURL = "https://direct-mcp.example/api"
	)
	for _, tc := range []struct {
		name   string
		legacy bool
		config *oauth2.Config
	}{
		{name: "new dynamic registration", config: &oauth2.Config{ClientID: "dynamic-client", ClientSecret: "dynamic-secret"}},
		{name: "new CIMD", config: &oauth2.Config{ClientID: "https://obot.example/oauth/client-metadata"}},
		{name: "legacy dynamic registration", legacy: true, config: &oauth2.Config{ClientID: "dynamic-client", ClientSecret: "dynamic-secret"}},
		{name: "legacy CIMD", legacy: true, config: &oauth2.Config{ClientID: "https://obot.example/oauth/client-metadata"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gatewayClient, storageClient := newDirectStateManagerTestClient(t, mcpID)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"access_token":"direct-access","refresh_token":"direct-refresh","token_type":"Bearer"}`)
			}))
			t.Cleanup(provider.Close)
			config := *tc.config
			config.Endpoint = oauth2.Endpoint{AuthURL: provider.URL + "/authorize", TokenURL: provider.URL}
			manager := newStateManager(gatewayClient)
			var state string
			if tc.legacy {
				state = "legacy-direct-state"
				require.NoError(t, manager.store(t.Context(), "user-1", mcpID, mcpURL, "request-1", "", state, "verifier-1", &config))
			} else {
				handler := &mcpOAuthHandler{
					client: storageClient, gatewayClient: gatewayClient, stateMgr: manager,
					userID: "user-1", mcpID: mcpID, mcpURL: mcpURL, urlChan: make(chan string, 1),
				}
				var err error
				state, _, err = handler.NewState(t.Context(), &config, "verifier-1")
				require.NoError(t, err)
			}

			_, _, err := manager.createToken(t.Context(), state, "code-1", "", "")
			require.NoError(t, err)
			stored, err := gatewayClient.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL)
			require.NoError(t, err)
			require.Equal(t, "direct-access", stored.AccessToken)
			require.Empty(t, stored.CatalogEntryName)
		})
	}
}

func TestMCPOAuthHandlerCapturesCatalogEntryOnlyForSelectedStaticApp(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	client := newStateManagerTestClient(t, entryName, mcpID)
	handlerStorage := clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(
			&v1.MCPServerInstance{
				ObjectMeta: metav1.ObjectMeta{Namespace: system.DefaultNamespace, Name: mcpID},
				Spec:       v1.MCPServerInstanceSpec{MCPServerCatalogEntryName: entryName},
			},
			&v1.MCPServerCatalogEntry{
				ObjectMeta: metav1.ObjectMeta{Namespace: system.DefaultNamespace, Name: entryName},
				Spec: v1.MCPServerCatalogEntrySpec{Manifest: apitypes.MCPServerCatalogEntryManifest{
					RemoteConfig: &apitypes.RemoteCatalogConfig{FixedURL: mcpURL, StaticOAuthRequired: true},
				}},
			},
		).
		Build()
	handler := &mcpOAuthHandler{
		client:        handlerStorage,
		gatewayClient: client,
		stateMgr:      newStateManager(client),
		userID:        "user-1",
		mcpID:         mcpID,
		mcpURL:        mcpURL,
		urlChan:       make(chan string, 1),
	}
	dynamicConfig := &oauth2.Config{ClientID: "dynamic-client", ClientSecret: "dynamic-secret"}
	_, _, err := handler.Lookup(t.Context(), "")
	require.Error(t, err)
	state, _, err := handler.NewState(t.Context(), dynamicConfig, "dynamic-verifier")
	require.NoError(t, err)
	pending, err := client.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Empty(t, pending.CatalogEntryName)

	require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entryName), Name: "oauth",
		Secrets: map[string]string{
			"CLIENT_ID": "static-client", "CLIENT_SECRET": "static-secret",
			"MCP_URL": mcpURL, "GENERATION": "generation-1",
		},
	}))
	clientID, clientSecret, err := handler.Lookup(t.Context(), "")
	require.NoError(t, err)
	require.Equal(t, "static-client", clientID)
	require.Equal(t, "static-secret", clientSecret)
	staticConfig := &oauth2.Config{ClientID: clientID, ClientSecret: clientSecret}
	state, _, err = handler.NewState(t.Context(), staticConfig, "static-verifier")
	require.NoError(t, err)
	pending, err = client.GetMCPOAuthPendingState(t.Context(), state)
	require.NoError(t, err)
	require.Equal(t, entryName, pending.CatalogEntryName)
	require.Equal(t, "generation-1", pending.CatalogCredentialGeneration)
}

func newStateManagerTestClient(t *testing.T, entryName, mcpID string) *gateway.Client {
	t.Helper()
	return newStateManagerTestClientWithStaticRequirement(t, entryName, mcpID, true)
}

func newStateManagerTestClientWithStaticRequirement(t *testing.T, entryName, mcpID string, staticOAuthRequired bool) *gateway.Client {
	t.Helper()
	storage := clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(
			&v1.MCPServerInstance{
				ObjectMeta: metav1.ObjectMeta{Namespace: system.DefaultNamespace, Name: mcpID},
				Spec:       v1.MCPServerInstanceSpec{MCPServerCatalogEntryName: entryName},
			},
			&v1.MCPServerCatalogEntry{
				ObjectMeta: metav1.ObjectMeta{Namespace: system.DefaultNamespace, Name: entryName},
				Spec: v1.MCPServerCatalogEntrySpec{Manifest: apitypes.MCPServerCatalogEntryManifest{
					RemoteConfig: &apitypes.RemoteCatalogConfig{FixedURL: "https://mcp.example/api", StaticOAuthRequired: staticOAuthRequired},
				}},
			},
		).
		Build()
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	client := gateway.New(t.Context(), db, storage, staticOAuthTestEncryptionConfig(), nil, nil, nil, time.Hour, 10, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func newDirectStateManagerTestClient(t *testing.T, mcpID string) (*gateway.Client, kclient.Client) {
	t.Helper()
	storageClient := clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(&v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Namespace: system.DefaultNamespace, Name: mcpID},
			Spec:       v1.MCPServerSpec{},
		}).
		Build()
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	gatewayClient := gateway.New(t.Context(), db, storageClient, nil, nil, nil, nil, time.Hour, 10, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gatewayClient.Close()) })
	return gatewayClient, storageClient
}
