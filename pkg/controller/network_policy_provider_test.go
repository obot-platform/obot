package controller

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeNetworkPolicyProviderInstaller struct {
	installed       *networkPolicyProviderInstallSpec
	uninstallCalled bool
	uninstallNS     string
}

func (f *fakeNetworkPolicyProviderInstaller) InstallOrUpgrade(_ context.Context, spec networkPolicyProviderInstallSpec) error {
	copied := spec
	copied.Values = cloneMap(spec.Values)
	f.installed = &copied
	return nil
}

func (f *fakeNetworkPolicyProviderInstaller) Uninstall(releaseNamespace string) error {
	f.uninstallCalled = true
	f.uninstallNS = releaseNamespace
	return nil
}

func newNetworkPolicyProviderController(t *testing.T, installer networkPolicyProviderInstaller) *Controller {
	t.Helper()

	return &Controller{
		services: &services.Services{
			MCPRuntimeBackend:                    "kubernetes",
			MCPServerNamespace:                   "obot-mcp",
			MCPClusterDomain:                     "cluster.local",
			MCPNetworkPolicyEnabled:              true,
			MCPNetworkPolicyProviderChartRepo:    "https://charts.example.com",
			MCPNetworkPolicyProviderChartName:    "network-policy-provider",
			MCPNetworkPolicyProviderChartVersion: "1.2.3",
			ServiceName:                          "obot",
			ServiceNamespace:                     "obot-system",
			ServiceAccountName:                   "obot-obot",
			StorageListenPort:                    8443,
		},
		providerInstaller: installer,
	}
}

func TestEnsureNetworkPolicyProviderInstallsChartRelease(t *testing.T) {
	ctx := t.Context()
	installer := &fakeNetworkPolicyProviderInstaller{}
	controller := newNetworkPolicyProviderController(t, installer)

	require.NoError(t, controller.reconcileNetworkPolicyProvider(ctx))
	require.NotNil(t, installer.installed)
	assert.Equal(t, networkPolicyProviderReleaseName, installer.installed.ReleaseName)
	assert.Equal(t, "obot-system", installer.installed.ReleaseNamespace)
	assert.Equal(t, "https://charts.example.com", installer.installed.ChartRepoURL)
	assert.Equal(t, "network-policy-provider", installer.installed.ChartName)
	assert.Equal(t, "1.2.3", installer.installed.ChartVersion)
	assert.Equal(t, "obot-mcp", installer.installed.Values["mcpRuntimeNamespace"])
	assert.Equal(t, "https://obot.obot-system.svc.cluster.local:8443", installer.installed.Values["obotStorageURL"])
	assert.Equal(t, serviceaccounts.NetworkPolicySecretName, installer.installed.Values["secretName"])
	assert.Equal(t, "/var/run/secrets/obot-network-policy-provider/apiKey", installer.installed.Values["obotStorageTokenFile"])
	obotValues := installer.installed.Values["obot"].(map[string]any)
	serviceAccountValues := obotValues["serviceAccount"].(map[string]any)
	assert.Equal(t, "obot-obot", serviceAccountValues["name"])
	assert.Equal(t, "obot-system", serviceAccountValues["namespace"])
}

func TestEnsureNetworkPolicyProviderMergesValuesBlob(t *testing.T) {
	ctx := t.Context()
	installer := &fakeNetworkPolicyProviderInstaller{}
	controller := newNetworkPolicyProviderController(t, installer)
	controller.services.MCPNetworkPolicyProviderValues = `
mcpRuntimeNamespace: custom-runtime
extraFlag: true
`

	require.NoError(t, controller.reconcileNetworkPolicyProvider(ctx))
	require.NotNil(t, installer.installed)
	assert.Equal(t, "custom-runtime", installer.installed.Values["mcpRuntimeNamespace"])
	assert.Equal(t, true, installer.installed.Values["extraFlag"])
	assert.Equal(t, "https://obot.obot-system.svc.cluster.local:8443", installer.installed.Values["obotStorageURL"])
}

func TestEnsureNetworkPolicyProviderUninstallsWhenDisabled(t *testing.T) {
	ctx := t.Context()
	installer := &fakeNetworkPolicyProviderInstaller{}
	controller := newNetworkPolicyProviderController(t, installer)
	controller.services.MCPNetworkPolicyEnabled = false

	require.NoError(t, controller.reconcileNetworkPolicyProvider(ctx))
	assert.True(t, installer.uninstallCalled)
	assert.Equal(t, "obot-system", installer.uninstallNS)
	assert.Nil(t, installer.installed)
}

func TestEnsureNetworkPolicyProviderSkipsUninstallOutsideKubernetes(t *testing.T) {
	ctx := t.Context()
	installer := &fakeNetworkPolicyProviderInstaller{}
	controller := newNetworkPolicyProviderController(t, installer)
	controller.services.MCPRuntimeBackend = "docker"
	controller.services.MCPNetworkPolicyEnabled = false

	require.NoError(t, controller.reconcileNetworkPolicyProvider(ctx))
	assert.False(t, installer.uninstallCalled)
	assert.Nil(t, installer.installed)
}

func TestEnsureNetworkPolicyProviderRequiresStorageSettings(t *testing.T) {
	ctx := t.Context()
	installer := &fakeNetworkPolicyProviderInstaller{}
	controller := newNetworkPolicyProviderController(t, installer)
	controller.services.ServiceName = ""

	err := controller.reconcileNetworkPolicyProvider(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service name")
}

func TestEnsureNetworkPolicyProviderRequiresServiceAccountName(t *testing.T) {
	ctx := t.Context()
	installer := &fakeNetworkPolicyProviderInstaller{}
	controller := newNetworkPolicyProviderController(t, installer)
	controller.services.ServiceAccountName = ""

	err := controller.reconcileNetworkPolicyProvider(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service account name")
}

func TestEnsureNetworkPolicyProviderUsesConfiguredClusterDomain(t *testing.T) {
	ctx := t.Context()
	installer := &fakeNetworkPolicyProviderInstaller{}
	controller := newNetworkPolicyProviderController(t, installer)
	controller.services.MCPClusterDomain = "example.internal"

	require.NoError(t, controller.reconcileNetworkPolicyProvider(ctx))
	require.NotNil(t, installer.installed)
	assert.Equal(t, "https://obot.obot-system.svc.example.internal:8443", installer.installed.Values["obotStorageURL"])
}
