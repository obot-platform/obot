package mcp

import (
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCatalogEntryConnectionDoesNotSelectOrCreateServer(t *testing.T) {
	for _, existingServer := range []bool{false, true} {
		t.Run(map[bool]string{false: "unconnected", true: "previously connected"}[existingServer], func(t *testing.T) {
			entry := &v1.MCPServerCatalogEntry{Name: "default-entry", Namespace: system.DefaultNamespace}
			client := newVMCPTestStorage(entry)
			if existingServer {
				require.NoError(t, client.Create(t.Context(), &v1.MCPServer{
					Name:      "ms1existing",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerSpec{
						UserID:                    "7",
						MCPServerCatalogEntryName: entry.Name,
					},
				}))
			}
			manager := &SessionManager{storageClient: client}
			_, _, _, err := manager.ServerForActionWithConnectID(t.Context(), entry.Name, &kuser.DefaultInfo{UID: "7"})
			require.ErrorContains(t, err, "invalid MCP connection ID")
			var servers v1.MCPServerList
			require.NoError(t, client.List(t.Context(), &servers))
			require.Len(t, servers.Items, map[bool]int{false: 0, true: 1}[existingServer])
			var instances v1.MCPServerInstanceList
			require.NoError(t, client.List(t.Context(), &instances))
			require.Empty(t, instances.Items)
		})
	}
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
