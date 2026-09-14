package mcp

import (
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAddExtractedEnvVarsToCatalogEntryManifestPreservesRemoteConfig(t *testing.T) {
	manifest := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			URLTemplate: "https://${EXISTING}.example.com/${DETECTED}",
		},
		Config: []types.MCPConfig{{
			Name: "Existing", Key: "EXISTING", Required: true,
			Usage: types.Header,
		}},
	}

	addExtractedEnvVarsToCatalogEntryManifest(manifest)

	require.ElementsMatch(t, []types.MCPConfig{
		{Name: "Existing", Key: "EXISTING", Required: true, Usage: types.Header},
		{Name: "DETECTED", Key: "DETECTED", Description: "Automatically detected variable", Required: true, Usage: types.Header},
	}, manifest.Config)
}

func TestServerOrInstanceFromConnectURLCreatesRemoteServerThatNeedsUserURL(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "user-1"
	)

	entry := &v1.MCPServerCatalogEntry{
		Name:      entryID,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					Hostname: "api.example.com",
				},
			},
		},
	}

	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(entry).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.UserID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.Template)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.CompositeName}
		}).
		Build()

	manager := SessionManager{storageClient: storageClient}
	server, instance, err := manager.serverOrInstanceFromConnectURL(t.Context(), entryID, userID)
	require.NoError(t, err)
	require.Empty(t, instance.Name)
	require.NotEmpty(t, server.Name)
	require.Equal(t, entryID, server.Spec.MCPServerCatalogEntryName)
	require.Equal(t, userID, server.Spec.UserID)
	require.True(t, server.Spec.NeedsURL)
	require.NotNil(t, server.Spec.Manifest.RemoteConfig)
	require.Equal(t, "api.example.com", server.Spec.Manifest.RemoteConfig.Hostname)
	require.Empty(t, server.Spec.Manifest.RemoteConfig.URL)
}

func TestServerOrInstanceFromConnectURLIgnoresVMCPComponentServers(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "user-1"
	)

	entry := &v1.MCPServerCatalogEntry{
		Name:      entryID,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{
				Hostname: "api.example.com",
			},
		}},
	}

	for _, component := range []*v1.MCPServer{
		{
			Name:      "ms1-vmcp-component",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				MCPServerCatalogEntryName: entryID,
				UserID:                    userID,
				VMCPID:                    "vmcp1-test",
			},
		},
		{
			Name:      "ms1-vmcp-instance-component",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				MCPServerCatalogEntryName: entryID,
				UserID:                    userID,
				VMCPInstanceID:            "vmcpi1-test",
			},
		},
	} {
		t.Run(component.Name, func(t *testing.T) {
			storageClient := fake.NewClientBuilder().
				WithScheme(storagescheme.Scheme).
				WithObjects(entry.DeepCopy(), component).
				WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
				}).
				WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.UserID}
				}).
				WithIndex(&v1.MCPServer{}, "spec.template", func(obj kclient.Object) []string {
					return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.Template)}
				}).
				WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.CompositeName}
				}).
				Build()

			server, instance, err := (&SessionManager{storageClient: storageClient}).serverOrInstanceFromConnectURL(t.Context(), entryID, userID)
			require.NoError(t, err)
			require.Empty(t, instance.Name)
			require.NotEqual(t, component.Name, server.Name)
			require.Empty(t, server.Spec.VMCPID)
			require.Empty(t, server.Spec.VMCPInstanceID)
		})
	}
}

func TestServerOrInstanceFromConnectURLPrefersStandaloneServerOverVMCPComponent(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "user-1"
	)

	entry := &v1.MCPServerCatalogEntry{Name: entryID, Namespace: system.DefaultNamespace}
	component := &v1.MCPServer{
		Name:              "ms1-vmcp-component",
		Namespace:         system.DefaultNamespace,
		CreationTimestamp: metav1.NewTime(time.Unix(1, 0)),
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: entryID,
			UserID:                    userID,
			VMCPID:                    "vmcp1-test",
		},
	}
	standalone := &v1.MCPServer{
		Name:              "ms1-standalone",
		Namespace:         system.DefaultNamespace,
		CreationTimestamp: metav1.NewTime(time.Unix(2, 0)),
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: entryID,
			UserID:                    userID,
		},
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(entry, component, standalone).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.UserID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.Template)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.CompositeName}
		}).
		Build()

	server, instance, err := (&SessionManager{storageClient: storageClient}).serverOrInstanceFromConnectURL(t.Context(), entryID, userID)
	require.NoError(t, err)
	require.Empty(t, instance.Name)
	require.Equal(t, standalone.Name, server.Name)
}

func TestServerOrInstanceFromConnectURLRejectsResourcesAbovePersistedMaximum(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "user-1"
	)

	maximum := resource.MustParse("500m")
	entry := &v1.MCPServerCatalogEntry{
		Name: entryID, Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "example"},
				Resources: &types.MCPResourceRequirements{
					Requests: types.MCPResourceRequests{CPU: "1"},
				},
			},
		},
	}
	settings := &v1.K8sSettings{
		Name: system.K8sSettingsName, Namespace: system.DefaultNamespace,
		Spec: v1.K8sSettingsSpec{MaxCPURequest: &maximum},
	}

	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(entry, settings).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.UserID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.Template)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.CompositeName}
		}).
		Build()

	manager := SessionManager{
		runtimeBackend: RuntimeBackendKubernetes,
		storageClient:  storageClient,
	}
	server, instance, err := manager.serverOrInstanceFromConnectURL(t.Context(), entryID, userID)
	require.ErrorContains(t, err, "resources.requests.cpu 1 exceeds configured maximum 500m")
	require.Empty(t, server.Name)
	require.Empty(t, instance.Name)

	var servers v1.MCPServerList
	require.NoError(t, storageClient.List(t.Context(), &servers))
	require.Empty(t, servers.Items)
}

func TestCatalogNameForServerWhenSourceEntryIsDeleted(t *testing.T) {
	const entryID = "default-everything-c001b50cc6rtk"

	tests := []struct {
		name                string
		server              v1.MCPServer
		entryExists         bool
		failOnEntryMissing  bool
		expectedCatalogName string
		expectError         bool
	}{
		{
			name: "shared vMCP component keeps its catalog when the entry is gone",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPCatalogID:              system.DefaultCatalog,
					MCPServerCatalogEntryName: entryID,
					VMCPID:                    "vmcp1-test",
					VMCPComponentID:           "component-1",
				},
			},
			entryExists:         false,
			failOnEntryMissing:  false,
			expectedCatalogName: system.DefaultCatalog,
			expectError:         false,
		},
		{
			name: "shared vMCP component keeps its catalog when reached through an instance",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPCatalogID:              system.DefaultCatalog,
					MCPServerCatalogEntryName: entryID,
					VMCPID:                    "vmcp1-test",
					VMCPComponentID:           "component-1",
				},
			},
			entryExists:         false,
			failOnEntryMissing:  true,
			expectedCatalogName: system.DefaultCatalog,
			expectError:         false,
		},
		{
			name: "dedicated vMCP component falls back to the default catalog",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPServerCatalogEntryName: entryID,
					VMCPInstanceID:            "vmcpi1-test",
					VMCPComponentID:           "component-1",
				},
			},
			entryExists:         false,
			failOnEntryMissing:  false,
			expectedCatalogName: system.DefaultCatalog,
			expectError:         false,
		},
		{
			name: "dedicated vMCP component falls back when the entry must exist",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPServerCatalogEntryName: entryID,
					VMCPInstanceID:            "vmcpi1-test",
					VMCPComponentID:           "component-1",
				},
			},
			entryExists:         false,
			failOnEntryMissing:  true,
			expectedCatalogName: system.DefaultCatalog,
			expectError:         false,
		},
		{
			name: "dedicated vMCP component keeps the workspace scope it recorded",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPServerCatalogEntryName: entryID,
					VMCPInstanceID:            "vmcpi1-test",
					VMCPComponentID:           "component-1",
				},
				Status: v1.MCPServerStatus{MCPCatalogID: "puw1-test"},
			},
			entryExists:         false,
			failOnEntryMissing:  false,
			expectedCatalogName: "puw1-test",
			expectError:         false,
		},
		{
			name: "standalone server still fails when the entry is gone",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPServerCatalogEntryName: entryID,
					UserID:                    "user-1",
				},
			},
			entryExists:        false,
			failOnEntryMissing: false,
			expectError:        true,
		},
		{
			// A reconciled standalone server has its catalog recorded on status, but it is
			// garbage collected along with its entry, so it must not resolve without one.
			name: "standalone server with a recorded catalog still fails",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPServerCatalogEntryName: entryID,
					UserID:                    "user-1",
				},
				Status: v1.MCPServerStatus{MCPCatalogID: system.DefaultCatalog},
			},
			entryExists:        false,
			failOnEntryMissing: false,
			expectError:        true,
		},
		{
			name: "admin catalog server with a known catalog still fails",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPCatalogID:              system.DefaultCatalog,
					MCPServerCatalogEntryName: entryID,
				},
			},
			entryExists:        false,
			failOnEntryMissing: false,
			expectError:        true,
		},
		{
			name: "existing entry supplies the catalog name",
			server: v1.MCPServer{
				Spec: v1.MCPServerSpec{
					MCPServerCatalogEntryName: entryID,
					VMCPInstanceID:            "vmcpi1-test",
					VMCPComponentID:           "component-1",
				},
			},
			entryExists:         true,
			failOnEntryMissing:  false,
			expectedCatalogName: "other-catalog",
			expectError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(storagescheme.Scheme)
			if tt.entryExists {
				builder = builder.WithObjects(&v1.MCPServerCatalogEntry{
					Name:      entryID,
					Namespace: system.DefaultNamespace,
					Spec:      v1.MCPServerCatalogEntrySpec{MCPCatalogName: "other-catalog"},
				})
			}

			manager := SessionManager{storageClient: builder.Build()}
			catalogName, err := manager.catalogNameForServer(t.Context(), tt.server, tt.failOnEntryMissing)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedCatalogName, catalogName)
		})
	}
}
