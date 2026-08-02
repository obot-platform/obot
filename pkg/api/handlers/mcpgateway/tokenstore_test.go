package mcpgateway

import (
	"testing"
	"time"

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

func TestTokenStoreRejectsCatalogGrantAfterProviderURLChanges(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		oldURL    = "https://mcp.example/api"
		newURL    = "https://new-mcp.example/api"
	)
	client, storage := newCatalogTokenStoreTestClientWithStorage(t, entryName, mcpID, true)
	config := &oauth2.Config{ClientID: "client-1", ClientSecret: "secret-1"}
	require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entryName),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     config.ClientID,
			"CLIENT_SECRET": config.ClientSecret,
			"MCP_URL":       oldURL,
			"GENERATION":    "generation-1",
		},
	}))
	require.NoError(t, client.ReplaceMCPOAuthTokenWithCatalogCredentialGenerationFence(
		t.Context(), "user-1", mcpID, oldURL, "", entryName, "generation-1", config,
		&oauth2.Token{AccessToken: "old-provider-access"},
	))

	var entry v1.MCPServerCatalogEntry
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: entryName}, &entry))
	entry.Spec.Manifest.RemoteConfig.FixedURL = newURL
	require.NoError(t, storage.Update(t.Context(), &entry))

	store := &tokenStore{gatewayClient: client, userID: "user-1", mcpID: mcpID}
	_, _, err := store.GetTokenConfig(t.Context(), oldURL)
	require.ErrorIs(t, err, gateway.ErrMCPOAuthCatalogCredentialChanged)
}

func TestTokenStoreRejectsLegacyGrantWithoutCatalogFence(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	client := newCatalogTokenStoreTestClient(t, entryName, mcpID, true)
	config := &oauth2.Config{ClientID: "client-1", ClientSecret: "secret-1"}
	require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entryName),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     config.ClientID,
			"CLIENT_SECRET": config.ClientSecret,
			"MCP_URL":       mcpURL,
			"GENERATION":    "generation-1",
		},
	}))
	require.NoError(t, client.ReplaceMCPOAuthToken(
		t.Context(), "user-1", mcpID, mcpURL, "", config,
		&oauth2.Token{AccessToken: "legacy-access"},
	))

	store := &tokenStore{gatewayClient: client, userID: "user-1", mcpID: mcpID}
	_, _, err := store.GetTokenConfig(t.Context(), mcpURL)
	require.ErrorIs(t, err, gateway.ErrMCPOAuthCatalogCredentialChanged)
}

func TestTokenStoreReadWaitsForCatalogMutationFence(t *testing.T) {
	const (
		entryName          = "catalog-entry-1"
		mcpID              = "mcp-instance-1"
		mcpURL             = "https://mcp.example/api"
		catalogMutationKey = "mcp-static-oauth-catalog-mutation"
	)
	client := newCatalogTokenStoreTestClient(t, entryName, mcpID, true)
	config := &oauth2.Config{ClientID: "client-1", ClientSecret: "secret-1"}
	require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entryName),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     config.ClientID,
			"CLIENT_SECRET": config.ClientSecret,
			"MCP_URL":       mcpURL,
			"GENERATION":    "generation-1",
		},
	}))
	require.NoError(t, client.ReplaceMCPOAuthTokenWithCatalogCredentialGenerationFence(
		t.Context(), "user-1", mcpID, mcpURL, "", entryName, "generation-1", config,
		&oauth2.Token{AccessToken: "active-access"},
	))

	release, err := client.AcquireCredentialLock(t.Context(), catalogMutationKey)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		_, _, err := (&tokenStore{gatewayClient: client, userID: "user-1", mcpID: mcpID}).GetTokenConfig(t.Context(), mcpURL)
		result <- err
	}()
	select {
	case err := <-result:
		release()
		t.Fatalf("token read bypassed catalog mutation fence: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	release()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for fenced token read")
	}
}

func TestTokenStoreRefreshCannotResurrectCatalogGrantAfterAppChange(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *gateway.Client)
	}{
		{
			name: "same client ID with new secret",
			change: func(t *testing.T, client *gateway.Client) {
				t.Helper()
				require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
					Context: system.MCPOAuthCredentialName(entryName),
					Name:    "oauth",
					Secrets: map[string]string{"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-2", "MCP_URL": mcpURL, "GENERATION": "generation-2"},
				}))
			},
		},
		{
			name: "credential cleared",
			change: func(t *testing.T, client *gateway.Client) {
				t.Helper()
				deleted, err := client.DeleteCredential(t.Context(), system.MCPOAuthCredentialName(entryName), "oauth")
				require.NoError(t, err)
				require.True(t, deleted)
			},
		},
		{
			name: "same values replaced with a new generation",
			change: func(t *testing.T, client *gateway.Client) {
				t.Helper()
				require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
					Context: system.MCPOAuthCredentialName(entryName),
					Name:    "oauth",
					Secrets: map[string]string{"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-1", "MCP_URL": mcpURL, "GENERATION": "generation-2"},
				}))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newCatalogTokenStoreTestClient(t, entryName, mcpID, true)
			require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: system.MCPOAuthCredentialName(entryName),
				Name:    "oauth",
				Secrets: map[string]string{"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-1", "MCP_URL": mcpURL, "GENERATION": "generation-1"},
			}))
			oldConfig := &oauth2.Config{ClientID: "client-1", ClientSecret: "secret-1"}
			require.NoError(t, client.ReplaceMCPOAuthTokenWithCatalogCredentialGenerationFence(t.Context(), "user-1", mcpID, mcpURL, "", entryName, "generation-1", oldConfig,
				&oauth2.Token{AccessToken: "old-access", RefreshToken: "refresh-1", Expiry: time.Now().Add(-time.Minute)}))

			store := &tokenStore{gatewayClient: client, userID: "user-1", mcpID: mcpID}
			config, _, err := store.GetTokenConfig(t.Context(), mcpURL)
			require.NoError(t, err)
			require.NotNil(t, config)

			credentialKey := system.MCPOAuthCredentialName(entryName)
			release, err := client.AcquireCredentialLock(t.Context(), credentialKey)
			require.NoError(t, err)
			tc.change(t, client)
			require.NoError(t, client.DeleteMCPOAuthTokenForAllUsers(t.Context(), mcpID))
			release()

			err = store.SetTokenConfig(t.Context(), mcpURL, config, &oauth2.Token{AccessToken: "refreshed-access", RefreshToken: "refresh-1"})
			require.ErrorIs(t, err, gateway.ErrMCPOAuthCatalogCredentialChanged)
			_, err = client.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		})
	}
}

func TestTokenStoreRejectsLegacyStaticGrantLeftByFailedClear(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	client := newCatalogTokenStoreTestClient(t, entryName, mcpID, true)
	config := &oauth2.Config{ClientID: "client-1", ClientSecret: "secret-1"}
	require.NoError(t, client.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entryName), Name: "oauth",
		Secrets: map[string]string{"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-1"},
	}))
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL, "", config,
		&oauth2.Token{AccessToken: "legacy-access", RefreshToken: "legacy-refresh"}))

	// Clear removed the shared app, then target discovery failed before it could delete this legacy token.
	deleted, err := client.DeleteCredential(t.Context(), system.MCPOAuthCredentialName(entryName), "oauth")
	require.NoError(t, err)
	require.True(t, deleted)
	store := &tokenStore{gatewayClient: client, userID: "user-1", mcpID: mcpID}
	_, _, err = store.GetTokenConfig(t.Context(), mcpURL)
	require.ErrorIs(t, err, gateway.ErrMCPOAuthCatalogCredentialChanged)

	// A successful Clear retry deletes the leftover row while a stale refresh result is ready to write.
	require.NoError(t, client.DeleteMCPOAuthTokenForAllUsers(t.Context(), mcpID))
	err = store.SetTokenConfig(t.Context(), mcpURL, config, &oauth2.Token{AccessToken: "resurrected-access"})
	require.ErrorIs(t, err, gateway.ErrMCPOAuthCatalogCredentialChanged)
	_, err = client.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestTokenStorePreservesLegacyDynamicGrantRefresh(t *testing.T) {
	const (
		entryName = "catalog-entry-1"
		mcpID     = "mcp-instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	client := newCatalogTokenStoreTestClient(t, entryName, mcpID, false)
	config := &oauth2.Config{ClientID: "dynamic-client", ClientSecret: "dynamic-secret"}
	require.NoError(t, client.ReplaceMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL, "", config,
		&oauth2.Token{AccessToken: "old-dynamic", RefreshToken: "dynamic-refresh"}))
	store := &tokenStore{gatewayClient: client, userID: "user-1", mcpID: mcpID}
	loadedConfig, _, err := store.GetTokenConfig(t.Context(), mcpURL)
	require.NoError(t, err)
	require.NoError(t, store.SetTokenConfig(t.Context(), mcpURL, loadedConfig,
		&oauth2.Token{AccessToken: "new-dynamic", RefreshToken: "dynamic-refresh"}))
	stored, err := client.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL)
	require.NoError(t, err)
	require.Equal(t, "new-dynamic", stored.AccessToken)
}

func newCatalogTokenStoreTestClient(t *testing.T, entryName, mcpID string, staticOAuthRequired bool) *gateway.Client {
	t.Helper()
	client, _ := newCatalogTokenStoreTestClientWithStorage(t, entryName, mcpID, staticOAuthRequired)
	return client
}

func newCatalogTokenStoreTestClientWithStorage(t *testing.T, entryName, mcpID string, staticOAuthRequired bool) (*gateway.Client, kclient.Client) {
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
	client := gateway.New(t.Context(), db, storage, nil, nil, nil, nil, time.Hour, 10, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client, storage
}
