package catalogupgrade

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type memoryCredentials struct {
	credential gatewaytypes.Credential
}

func (m *memoryCredentials) RevealCredential(context.Context, []string, string) (gatewaytypes.Credential, error) {
	return m.credential, nil
}

func (m *memoryCredentials) UpsertCredential(_ context.Context, credential gatewaytypes.Credential) error {
	m.credential = credential
	return nil
}

func (m *memoryCredentials) DeleteCredential(context.Context, string, string) (bool, error) {
	m.credential = gatewaytypes.Credential{}
	return true, nil
}

func TestPlanCatalogUpgradeReportsReusableAndMissingConfiguration(t *testing.T) {
	entry, versions, server := upgradeFixtures()
	versions[1].Spec.Manifest = types.MCPServerCatalogEntryManifest{
		Name: "Target", Runtime: types.RuntimeRemote, ServerUserType: types.ServerUserTypeSingleUser,
		RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://target.example.com/mcp", Headers: []types.MCPHeader{
			{Key: "AUTH", Required: true, Sensitive: false},
		}},
		Env: []types.MCPEnv{
			{MCPHeader: types.MCPHeader{Key: "API_KEY", Required: true, Sensitive: true}},
			{MCPHeader: types.MCPHeader{Key: "NEW_KEY", Required: true, Sensitive: true}},
		},
	}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, versions[0], versions[1], server).Build()
	credentials := &memoryCredentials{credential: gatewaytypes.Credential{Secrets: map[string]string{"API_KEY": "secret", "AUTH": "old"}}}
	planner := New(client, credentials, nil, nil, mcp.ValidationOptions{AllowMissingURL: true})

	plan, err := planner.Plan(t.Context(), server.Name, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"API_KEY"}, plan.ReusableConfiguration)
	assert.Equal(t, []string{"NEW_KEY"}, plan.MissingRequiredEnvVars)
	assert.Equal(t, []string{"AUTH"}, plan.MissingRequiredHeaders)
	assert.True(t, plan.RuntimeChanged)
	assert.True(t, plan.DestructiveStorageCleanup)
	assert.False(t, plan.CanApply)
}

func TestApplyCatalogUpgradeRejectsMutableTargetChangeBeforeShutdown(t *testing.T) {
	entry, versions, server := upgradeFixtures()
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, versions[0], versions[1], server).Build()
	credentials := &memoryCredentials{credential: gatewaytypes.Credential{Secrets: map[string]string{}}}
	shutdowns := 0
	planner := New(client, credentials, func(context.Context, string) error {
		shutdowns++
		return nil
	}, nil, mcp.ValidationOptions{AllowMissingURL: true})

	plan, err := planner.Plan(t.Context(), server.Name, nil)
	require.NoError(t, err)
	var target v1.MCPServerCatalogEntryVersion
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(versions[1]), &target))
	target.Spec.Manifest.Description = "mutated"
	require.NoError(t, client.Update(t.Context(), &target))

	_, err = planner.Apply(t.Context(), server.Name, types.CatalogUpgradeApplyRequest{PlanID: plan.ID})
	assert.ErrorContains(t, err, "upgrade plan is stale")
	assert.Zero(t, shutdowns)
}

func TestApplyCatalogUpgradeUpdatesSameServerInPlace(t *testing.T) {
	entry, versions, server := upgradeFixtures()
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, versions[0], versions[1], server).Build()
	credentials := &memoryCredentials{credential: gatewaytypes.Credential{Secrets: map[string]string{"API_KEY": "secret"}}}
	shutdowns := 0
	planner := New(client, credentials, func(context.Context, string) error {
		shutdowns++
		return nil
	}, nil, mcp.ValidationOptions{})

	plan, err := planner.Plan(t.Context(), server.Name, nil)
	require.NoError(t, err)
	assert.True(t, plan.CanApply)
	result, err := planner.Apply(t.Context(), server.Name, types.CatalogUpgradeApplyRequest{PlanID: plan.ID})
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Equal(t, 1, shutdowns)

	var updated v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &updated))
	assert.Equal(t, server.Name, updated.Name)
	assert.Equal(t, 2, updated.Spec.MCPServerCatalogEntryVersion)
	assert.Equal(t, "target:v2", updated.Spec.Manifest.ContainerizedConfig.Image)
	assert.Equal(t, "secret", credentials.credential.Secrets["API_KEY"])
}

func TestApplyCatalogUpgradeAcceptsTargetURLAndOAuthConfirmation(t *testing.T) {
	entry, versions, server := upgradeFixtures()
	versions[1].Spec.Manifest.Runtime = types.RuntimeRemote
	versions[1].Spec.Manifest.ContainerizedConfig = nil
	versions[1].Spec.Manifest.RemoteConfig = &types.RemoteCatalogConfig{Hostname: "example.com"}
	entry.Spec.Manifest = versions[1].Spec.Manifest
	server.Status.UserHasAuthenticated = true
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, versions[0], versions[1], server).Build()
	credentials := &memoryCredentials{credential: gatewaytypes.Credential{Secrets: map[string]string{"API_KEY": "secret"}}}
	planner := New(client, credentials, func(context.Context, string) error { return nil }, nil, mcp.ValidationOptions{})

	plan, err := planner.Plan(t.Context(), server.Name, nil)
	require.NoError(t, err)
	assert.True(t, plan.MissingURL)
	assert.True(t, plan.OAuthReauthorizationRequired)

	result, err := planner.Apply(t.Context(), server.Name, types.CatalogUpgradeApplyRequest{
		PlanID: plan.ID, URL: "example.com/mcp", ConfirmOAuthReauthorization: true,
	})
	require.NoError(t, err)
	assert.True(t, result.Applied)
	var updated v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &updated))
	require.NotNil(t, updated.Spec.Manifest.RemoteConfig)
	assert.Equal(t, "https://example.com/mcp", updated.Spec.Manifest.RemoteConfig.URL)
}

func TestPlanCatalogUpgradeReportsMultiUserConfigurationImpact(t *testing.T) {
	entry, versions, server := upgradeFixtures()
	server.Spec.MCPCatalogID = "default"
	for _, version := range versions {
		version.Spec.Manifest.ServerUserType = types.ServerUserTypeMultiUser
	}
	versions[1].Spec.Manifest.MultiUserConfig = &types.MultiUserConfig{UserDefinedHeaders: []types.MCPHeader{{Key: "USER_TOKEN", Required: true, Sensitive: true}}}
	entry.Spec.Manifest = versions[1].Spec.Manifest
	instance := &v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "msi-user", Namespace: server.Namespace},
		Spec:       v1.MCPServerInstanceSpec{MCPServerName: server.Name, UserID: "user-1", MultiUserConfig: &types.MultiUserConfig{}},
	}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, versions[0], versions[1], server, instance).Build()
	credentials := &memoryCredentials{credential: gatewaytypes.Credential{Secrets: map[string]string{"API_KEY": "secret"}}}
	planner := New(client, credentials, nil, nil, mcp.ValidationOptions{})

	plan, err := planner.Plan(t.Context(), server.Name, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, plan.AffectedUsers)
	assert.Equal(t, map[string][]string{"msi-user": {"USER_TOKEN"}}, plan.MissingInstanceConfiguration)
	assert.Contains(t, plan.Warnings, types.CatalogUpgradeWarning{Code: "user-reconfiguration", Message: "1 user instances require additional configuration"})
}

func TestApplyCatalogUpgradeToleratesStatusOnlyUpdateAfterReservation(t *testing.T) {
	entry, versions, server := upgradeFixtures()
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, versions[0], versions[1], server).Build()
	credentials := &memoryCredentials{credential: gatewaytypes.Credential{Secrets: map[string]string{"API_KEY": "secret"}}}
	planner := New(client, credentials, func(ctx context.Context, serverID string) error {
		var current v1.MCPServer
		require.NoError(t, client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: serverID}, &current))
		current.Status.NeedsUpdate = true
		return client.Update(ctx, &current)
	}, nil, mcp.ValidationOptions{})

	plan, err := planner.Plan(t.Context(), server.Name, nil)
	require.NoError(t, err)
	result, err := planner.Apply(t.Context(), server.Name, types.CatalogUpgradeApplyRequest{PlanID: plan.ID})
	require.NoError(t, err)
	assert.True(t, result.Applied)

	var updated v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &updated))
	assert.Equal(t, 2, updated.Spec.MCPServerCatalogEntryVersion)
	assert.Equal(t, "target:v2", updated.Spec.Manifest.ContainerizedConfig.Image)
	assert.Empty(t, updated.Annotations[reservationAnnotation])
}

func TestApplyCatalogUpgradeRollsBackAfterReservedSpecChange(t *testing.T) {
	entry, versions, server := upgradeFixtures()
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, versions[0], versions[1], server).Build()
	credentials := &memoryCredentials{credential: gatewaytypes.Credential{
		Context: "user-ms-server", Name: server.Name, Secrets: map[string]string{"API_KEY": "old-secret"},
	}}
	planner := New(client, credentials, func(ctx context.Context, serverID string) error {
		var current v1.MCPServer
		require.NoError(t, client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: serverID}, &current))
		current.Spec.Alias = "concurrent-change"
		return client.Update(ctx, &current)
	}, nil, mcp.ValidationOptions{})

	plan, err := planner.Plan(t.Context(), server.Name, nil)
	require.NoError(t, err)
	_, err = planner.Apply(t.Context(), server.Name, types.CatalogUpgradeApplyRequest{
		PlanID: plan.ID, Configuration: map[string]string{"API_KEY": "new-secret"},
	})
	require.ErrorContains(t, err, "upgrade plan is stale")

	var updated v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &updated))
	assert.Equal(t, "concurrent-change", updated.Spec.Alias)
	assert.Equal(t, 1, updated.Spec.MCPServerCatalogEntryVersion)
	assert.Empty(t, updated.Annotations[reservationAnnotation])
	assert.Equal(t, "old-secret", credentials.credential.Secrets["API_KEY"])
}

func TestApplyCatalogUpgradeRollsBackWhenTargetChangesAfterReservation(t *testing.T) {
	entry, versions, server := upgradeFixtures()
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, versions[0], versions[1], server).Build()
	credentials := &memoryCredentials{credential: gatewaytypes.Credential{
		Context: "user-ms-server", Name: server.Name, Secrets: map[string]string{"API_KEY": "old-secret"},
	}}
	planner := New(client, credentials, func(ctx context.Context, _ string) error {
		var target v1.MCPServerCatalogEntryVersion
		require.NoError(t, client.Get(ctx, kclient.ObjectKeyFromObject(versions[1]), &target))
		target.Spec.Manifest.Description = "concurrent-target-change"
		return client.Update(ctx, &target)
	}, nil, mcp.ValidationOptions{})

	plan, err := planner.Plan(t.Context(), server.Name, nil)
	require.NoError(t, err)
	_, err = planner.Apply(t.Context(), server.Name, types.CatalogUpgradeApplyRequest{
		PlanID: plan.ID, Configuration: map[string]string{"API_KEY": "new-secret"},
	})
	require.ErrorContains(t, err, "target catalog version changed")

	var updated v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &updated))
	assert.Equal(t, 1, updated.Spec.MCPServerCatalogEntryVersion)
	assert.Empty(t, updated.Annotations[reservationAnnotation])
	assert.Equal(t, "old-secret", credentials.credential.Secrets["API_KEY"])
}

func upgradeFixtures() (*v1.MCPServerCatalogEntry, []*v1.MCPServerCatalogEntryVersion, *v1.MCPServer) {
	v1Manifest := types.MCPServerCatalogEntryManifest{
		Name: "Source", Runtime: types.RuntimeContainerized, ServerUserType: types.ServerUserTypeSingleUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{Image: "source:v1", Port: 8080, Path: "/mcp"},
		Env:                 []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "API_KEY", Required: true, Sensitive: true}}},
	}
	v2Manifest := v1Manifest
	v2Manifest.Name = "Target"
	v2Manifest.ContainerizedConfig = &types.ContainerizedRuntimeConfig{Image: "target:v2", Port: 8080, Path: "/mcp"}
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "entry", Namespace: system.DefaultNamespace},
		Spec:       v1.MCPServerCatalogEntrySpec{Manifest: v2Manifest, DefaultVersion: 2},
	}
	versions := []*v1.MCPServerCatalogEntryVersion{
		{ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(entry.Name, 1), Namespace: entry.Namespace}, Spec: v1.MCPServerCatalogEntryVersionSpec{MCPServerCatalogEntryName: entry.Name, Version: 1, Manifest: v1Manifest, Active: true}},
		{ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(entry.Name, 2), Namespace: entry.Namespace}, Spec: v1.MCPServerCatalogEntryVersionSpec{MCPServerCatalogEntryName: entry.Name, Version: 2, Manifest: v2Manifest, Active: true}},
	}
	serverManifest, _ := types.MapCatalogEntryToServer(v1Manifest, "", false)
	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "ms-server", Namespace: entry.Namespace},
		Spec:       v1.MCPServerSpec{Manifest: serverManifest, MCPServerCatalogEntryName: entry.Name, MCPServerCatalogEntryVersion: 1, UserID: "user"},
	}
	return entry, versions, server
}
