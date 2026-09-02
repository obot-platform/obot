package mcpcatalog

import (
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCredentializeCatalogObjectsIsolatesRegularAndSystemEntriesWithSameName(t *testing.T) {
	gatewayClient := newStaticConfigTestGatewayClient(t)
	handler := &Handler{gatewayClient: gatewayClient}
	const entryName = "default-shared"

	regular := &v1.MCPServerCatalogEntry{Name: entryName, Spec: v1.MCPServerCatalogEntrySpec{
		Manifest: types.MCPServerCatalogEntryManifest{Env: []types.MCPEnv{{
			Key: "TOKEN", Value: "regular-secret", Sensitive: true,
		}}},
	}}
	systemEntry := &v1.SystemMCPServerCatalogEntry{Name: entryName, Spec: v1.SystemMCPServerCatalogEntrySpec{
		Manifest: types.SystemMCPServerCatalogEntryManifest{Env: []types.MCPEnv{{
			Key: "TOKEN", Value: "system-secret", Sensitive: true,
		}}},
	}}
	require.NoError(t, handler.credentializeCatalogObjects(t.Context(), []kclient.Object{regular, systemEntry}))

	regularContext := mcp.CatalogEntryStaticCredentialContext(entryName)
	systemContext := mcp.SystemCatalogEntryStaticCredentialContext(entryName)
	regularSecrets, err := mcp.StaticCredentialSecrets(t.Context(), gatewayClient, regularContext, entryName)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "regular-secret"}, regularSecrets)
	systemSecrets, err := mcp.StaticCredentialSecrets(t.Context(), gatewayClient, systemContext, entryName)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "system-secret"}, systemSecrets)

	_, err = gatewayClient.DeleteCredential(t.Context(), regularContext, mcp.StaticConfigurationCredentialName(entryName))
	require.NoError(t, err)
	_, err = gatewayClient.RevealCredential(t.Context(), []string{regularContext}, mcp.StaticConfigurationCredentialName(entryName))
	require.Error(t, err)
	var notFound gatewayclient.CredentialNotFoundError
	require.True(t, errors.As(err, &notFound))
	systemSecrets, err = mcp.StaticCredentialSecrets(t.Context(), gatewayClient, systemContext, entryName)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "system-secret"}, systemSecrets)
}

func TestCredentializeCatalogObjectsRestoresCredentialsWhenCredentializationFails(t *testing.T) {
	gatewayClient := newStaticConfigTestGatewayClient(t)
	handler := &Handler{gatewayClient: gatewayClient}

	oldManifest := types.MCPServerCatalogEntryManifest{Env: []types.MCPEnv{{
		Key: "TOKEN", Value: "old", Sensitive: true,
	}}}
	oldSecrets := mcp.ExtractStaticCatalogConfiguration(&oldManifest, nil, false)
	require.NoError(t, mcp.StoreStaticCredentialSecrets(t.Context(), gatewayClient, mcp.CatalogEntryStaticCredentialContext("entry"), "entry", oldSecrets))

	entry := &v1.MCPServerCatalogEntry{Name: "entry", Spec: v1.MCPServerCatalogEntrySpec{
		Manifest: types.MCPServerCatalogEntryManifest{Env: []types.MCPEnv{{
			Key: "TOKEN", Value: "new", Sensitive: true,
		}}},
	}}
	invalidEntry := &v1.MCPServerCatalogEntry{Spec: v1.MCPServerCatalogEntrySpec{
		Manifest: types.MCPServerCatalogEntryManifest{Env: []types.MCPEnv{{
			Key: "TOKEN", Value: "invalid", Sensitive: true,
		}}},
	}}
	require.Error(t, handler.credentializeCatalogObjects(t.Context(), []kclient.Object{entry, invalidEntry}))

	secrets, err := mcp.StaticCredentialSecrets(t.Context(), gatewayClient, mcp.CatalogEntryStaticCredentialContext("entry"), "entry")
	require.NoError(t, err)
	require.Equal(t, oldSecrets, secrets)
}

func TestCredentializeCatalogObjectsPreservesResolvedComponentStaticConfiguration(t *testing.T) {
	gatewayClient := newStaticConfigTestGatewayClient(t)
	handler := &Handler{gatewayClient: gatewayClient}

	target := testCatalogEntry("target", "source", "tool", types.MCPServerCatalogEntryManifest{
		Runtime:        types.RuntimeNPX,
		NPXConfig:      &types.NPXRuntimeConfig{Package: "tool"},
		ServerUserType: types.ServerUserTypeSingleUser,
		Env: []types.MCPEnv{{
			Key: "TOKEN", Value: "secret", Sensitive: true,
		}},
	})
	composite := testCatalogEntry("composite", "source", "composite", types.MCPServerCatalogEntryManifest{
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{{
			CatalogEntryID: sourceRef("source", "tool"),
		}}},
	})

	objects, errsBySourceURL := handler.resolveCompositeSourceRefs(t.Context(), nil, "", "", []kclient.Object{target, composite})
	require.Empty(t, errsBySourceURL)
	err := handler.credentializeCatalogObjects(t.Context(), objects)
	require.NoError(t, err)

	targetSecrets, err := mcp.StaticCredentialSecrets(t.Context(), gatewayClient, mcp.CatalogEntryStaticCredentialContext(target.Name), target.Name)
	require.NoError(t, err)
	targetManifest := mcp.HydrateStaticCatalogConfiguration(target.Spec.Manifest, targetSecrets)
	require.Equal(t, "secret", targetManifest.Env[0].Value)

	compositeSecrets, err := mcp.StaticCredentialSecrets(t.Context(), gatewayClient, mcp.CatalogEntryStaticCredentialContext(composite.Name), composite.Name)
	require.NoError(t, err)
	compositeManifest := mcp.HydrateStaticCatalogConfiguration(composite.Spec.Manifest, compositeSecrets)
	require.Equal(t, "secret", compositeManifest.CompositeConfig.ComponentServers[0].Manifest.Env[0].Value)
}

func newStaticConfigTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	client := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = client.Close() })
	return client
}
