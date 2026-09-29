package mcpserverinstance

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Connections synced before the status field existed carry the hash as an
// annotation; the next sync must move it to status so callers see it as synced.
func TestSyncVMCPConfigurationRecordsConnectionStatus(t *testing.T) {
	component := types.VMCPComponent{
		ID:            "github",
		Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyUserAllowed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "TOKEN", Usage: types.Header, Required: true}},
		}},
	}
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}},
	}}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1shared",
		Namespace: vmcp.Namespace,
		Spec:      v1.VMCPInstanceSpec{UserID: "1", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
		Status:    v1.VMCPInstanceStatus{UserConfigurationHash: "saved"},
	}
	server := &v1.MCPServer{Name: "ms1shared", Namespace: vmcp.Namespace, Spec: v1.MCPServerSpec{
		VMCPID:          vmcp.Name,
		VMCPComponentID: component.ID,
	}}
	connection := &v1.MCPServerInstance{
		Name:        "msi1shared",
		Namespace:   vmcp.Namespace,
		Annotations: map[string]string{v1.LegacyVMCPConnectionConfigurationHashAnnotation: "legacy"},
		Spec: v1.MCPServerInstanceSpec{
			UserID:          instance.Spec.UserID,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: component.ID,
			MCPServerName:   server.Name,
		},
	}
	storage := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.MCPServerInstance{}).
		WithObjects(vmcp, instance, server, connection).
		Build()
	gateway := newTestGatewayClient(t)
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey(component.ID, "TOKEN"): "user-token"},
	}))
	handler := New(gateway)

	sync := func() v1.MCPServerInstance {
		t.Helper()
		var current v1.MCPServerInstance
		require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(connection), &current))
		require.NoError(t, handler.SyncVMCPConfiguration(router.Request{Ctx: t.Context(), Client: storage, Object: &current, Namespace: current.Namespace}, nil))
		require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(connection), &current))
		return current
	}

	synced := sync()
	require.NotContains(t, synced.Annotations, v1.LegacyVMCPConnectionConfigurationHashAnnotation)
	require.Equal(t, vmcpconfig.ConnectionConfiguration(component), synced.Spec.Config)
	require.Equal(t, vmcpconfig.ConnectionConfigurationHash(synced.Spec.Config, *instance), synced.Status.VMCPConfigurationHash)
	require.True(t, vmcpconfig.ConnectionConfigurationSynced(synced, component, *instance))

	credential, err := gateway.RevealCredential(t.Context(), []string{instance.Spec.UserID + "-" + connection.Name}, connection.Name)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "user-token"}, credential.Secrets)

	// A synced connection is left alone.
	require.Equal(t, synced.ResourceVersion, sync().ResourceVersion)
}

func newTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gatewayClient := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	return gatewayClient
}

// A component without user configuration stores an empty Config, which storage
// returns as nil. The controller must still treat the connection as synced.
func TestSyncVMCPConfigurationSettlesWithoutUserConfiguration(t *testing.T) {
	component := types.VMCPComponent{
		ID:            "github",
		Configuration: []types.VMCPConfigurationPolicy{{Key: "FIXED", Policy: types.VMCPConfigurationPolicyFixed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "FIXED", Required: true, Usage: types.Header}},
		}},
	}
	vmcp := &v1.VMCP{Name: "vmcp1fixed", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}},
	}}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1fixed",
		Namespace: vmcp.Namespace,
		Spec:      v1.VMCPInstanceSpec{UserID: "1", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
		Status:    v1.VMCPInstanceStatus{UserConfigurationHash: "saved"},
	}
	server := &v1.MCPServer{Name: "ms1fixed", Namespace: vmcp.Namespace, Spec: v1.MCPServerSpec{
		VMCPID:          vmcp.Name,
		VMCPComponentID: component.ID,
	}}
	connection := &v1.MCPServerInstance{
		Name:      "msi1fixed",
		Namespace: vmcp.Namespace,
		Spec: v1.MCPServerInstanceSpec{
			UserID:          instance.Spec.UserID,
			VMCPInstanceID:  instance.Name,
			VMCPComponentID: component.ID,
			MCPServerName:   server.Name,
		},
	}
	storage := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.MCPServerInstance{}).
		WithObjects(vmcp, instance, server, connection).
		Build()
	handler := New(newTestGatewayClient(t))

	sync := func() v1.MCPServerInstance {
		t.Helper()
		var current v1.MCPServerInstance
		require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(connection), &current))
		require.NoError(t, handler.SyncVMCPConfiguration(router.Request{Ctx: t.Context(), Client: storage, Object: &current, Namespace: current.Namespace}, nil))
		require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(connection), &current))
		return current
	}

	synced := sync()
	require.NotEmpty(t, synced.Status.VMCPConfigurationHash)
	require.True(t, vmcpconfig.ConnectionConfigurationSynced(synced, component, *instance))
	require.Equal(t, synced.ResourceVersion, sync().ResourceVersion, "synced connection was rewritten")
}
