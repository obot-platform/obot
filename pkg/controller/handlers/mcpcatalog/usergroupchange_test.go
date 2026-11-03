package mcpcatalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestHandleUserGroupChangeLookupErrors(t *testing.T) {
	storage, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(storage.DB.DB, storage.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })

	user := &gatewaytypes.User{}
	require.NoError(t, storage.DB.DB.Create(user).Error)
	require.NoError(t, storage.DB.DB.Create(&gatewaytypes.GroupMemberships{
		UserID:  user.ID,
		GroupID: "team",
	}).Error)
	failure := errors.New("database temporarily unavailable")

	for _, tc := range []struct {
		name      string
		failTable string
		missing   bool
	}{
		{
			name:    "missing user discards event",
			missing: true,
		},
		{
			name:      "user lookup failure preserves event",
			failTable: "users",
		},
		{
			name:      "group lookup failure preserves event",
			failTable: "group_memberships",
		},
		{
			name:      "role lookup failure preserves event",
			failTable: "group_role_assignments",
		},
		{
			name: "successful cleanup removes event",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			injected := false
			if tc.failTable != "" {
				require.NoError(t, storage.DB.DB.Callback().Query().Before("gorm:query").Register("test:lookup-failure", func(tx *gorm.DB) {
					if tx.Statement.Table == tc.failTable {
						injected = true
						_ = tx.AddError(failure)
					}
				}))
				t.Cleanup(func() { require.NoError(t, storage.DB.DB.Callback().Query().Remove("test:lookup-failure")) })
			}

			event := &v1.UserGroupChange{
				Name:      "group-change",
				Namespace: system.DefaultNamespace,
				Spec: v1.UserGroupChangeSpec{
					UserID: user.ID,
				},
			}
			if tc.missing {
				event.Spec.UserID = user.ID + 1
			}
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(event).
				WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.UserID}
				}).
				WithIndex(&v1.MCPServerInstance{}, "spec.userID", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServerInstance).Spec.UserID}
				}).Build()
			handler := &Handler{gatewayClient: gateway}

			err := handler.HandleUserGroupChange(router.Request{
				Ctx:       t.Context(),
				Client:    client,
				Namespace: event.Namespace,
				Object:    event,
			}, nil)

			if tc.failTable != "" {
				require.True(t, injected)
				require.ErrorIs(t, err, failure)
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(event), &v1.UserGroupChange{}))
			} else {
				require.NoError(t, err)
				require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(event), &v1.UserGroupChange{})))
			}
		})
	}
}

func TestDeleteUnauthorizedServersForUserPreservesVMCPComponents(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: system.DefaultCatalog,
		},
	}
	standalone := &v1.MCPServer{
		Name:      "standalone",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			UserID:                    "1",
			MCPServerCatalogEntryName: entry.Name,
		},
	}
	component := standalone.DeepCopy()
	component.Name = "component"
	component.Spec.VMCPInstanceID = "vmcpi1dedicated"
	component.Spec.VMCPComponentID = "one"
	catalogServer := standalone.DeepCopy()
	catalogServer.Name = "catalog-server"
	catalogServer.Spec.MCPCatalogID = system.DefaultCatalog

	workspaceServer := standalone.DeepCopy()
	workspaceServer.Name = "workspace-server"
	workspaceServer.Spec.PowerUserWorkspaceID = "workspace"

	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.UserID}
		}).WithObjects(entry, standalone, component, catalogServer, workspaceServer).Build()
	// No remaining ACR grants access to the live catalog entry.
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		"selectors":           cache.MetaNamespaceIndexFunc,
		"catalog-entry-names": cache.MetaNamespaceIndexFunc,
	})
	h := &Handler{accessControlRuleHelper: accesscontrolrule.NewAccessControlRuleHelper(indexer, client)}
	user := &userInfo{Info: &kuser.DefaultInfo{UID: "1"}}

	require.NoError(t, h.deleteUnauthorizedServersForUser(t.Context(), client, system.DefaultNamespace, "1", user))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(component), &v1.MCPServer{}))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalogServer), &v1.MCPServer{}))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(workspaceServer), &v1.MCPServer{}))
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(standalone), &v1.MCPServer{})))
}

func TestDeleteUnauthorizedInstancesForUserPreservesVMCPComponents(t *testing.T) {
	catalogServer := &v1.MCPServer{
		Name:      "catalog-server",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			MCPCatalogID: system.DefaultCatalog,
		},
	}
	server := &v1.MCPServer{
		Name:      "generated-component",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			MCPCatalogID:    system.DefaultCatalog,
			VMCPID:          "vmcp1shared",
			VMCPComponentID: "one",
		},
	}
	legacy := &v1.MCPServerInstance{
		Name:      "legacy",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerInstanceSpec{
			UserID:        "1",
			MCPServerName: catalogServer.Name,
		},
	}

	component := legacy.DeepCopy()
	component.Name = "component"
	component.Spec.MCPServerName = server.Name
	component.Spec.VMCPInstanceID = "vmcpi1shared"
	component.Spec.VMCPComponentID = "one"

	composite := legacy.DeepCopy()
	composite.Name = "composite"
	composite.Spec.CompositeName = "composite-server"

	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithIndex(&v1.MCPServerInstance{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerInstance).Spec.UserID}
		}).WithObjects(catalogServer, server, legacy, component, composite).Build()

	// No remaining ACR grants access to the backing catalog server.
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		"selectors":    cache.MetaNamespaceIndexFunc,
		"server-names": cache.MetaNamespaceIndexFunc,
	})
	h := &Handler{accessControlRuleHelper: accesscontrolrule.NewAccessControlRuleHelper(indexer, client)}
	user := &userInfo{Info: &kuser.DefaultInfo{UID: "1"}}

	require.NoError(t, h.deleteUnauthorizedInstancesForUser(t.Context(), client, system.DefaultNamespace, "1", user))

	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(component), &v1.MCPServerInstance{}))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(composite), &v1.MCPServerInstance{}))
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(legacy), &v1.MCPServerInstance{})))
}

func TestGroupChangeCleanupStorageErrors(t *testing.T) {
	transientErr := errors.New("storage temporarily unavailable")
	notFoundErr := apierrors.NewNotFound(schema.GroupResource{Resource: "test-resources"}, "missing")

	for _, resource := range []string{"server", "instance"} {
		for _, tc := range []struct {
			name        string
			getErr      error
			deleteErr   error
			wantErr     error
			wantDeletes int
		}{
			{
				name:   "missing source is skipped",
				getErr: notFoundErr,
			},
			{
				name:    "transient source lookup error is retried",
				getErr:  transientErr,
				wantErr: transientErr,
			},
			{
				name:        "concurrent deletion succeeds",
				deleteErr:   notFoundErr,
				wantDeletes: 1,
			},
			{
				name:        "transient deletion error is retried",
				deleteErr:   transientErr,
				wantErr:     transientErr,
				wantDeletes: 1,
			},
		} {
			t.Run(resource+"/"+tc.name, func(t *testing.T) {
				entry := &v1.MCPServerCatalogEntry{
					Name:      "entry",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerCatalogEntrySpec{
						MCPCatalogName: system.DefaultCatalog,
					},
				}
				server := &v1.MCPServer{
					Name:      "server",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerSpec{
						UserID:                    "1",
						MCPServerCatalogEntryName: entry.Name,
					},
				}
				instance := &v1.MCPServerInstance{
					Name:      "instance",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerInstanceSpec{
						UserID:        "1",
						MCPServerName: server.Name,
					},
				}
				if resource == "instance" {
					server.Spec.MCPCatalogID = system.DefaultCatalog
				}

				getCalls, deleteCalls := 0, 0
				client := fake.NewClientBuilder().WithScheme(scheme.Scheme).
					WithObjects(entry, server, instance).
					WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
						return []string{obj.(*v1.MCPServer).Spec.UserID}
					}).
					WithIndex(&v1.MCPServerInstance{}, "spec.userID", func(obj kclient.Object) []string {
						return []string{obj.(*v1.MCPServerInstance).Spec.UserID}
					}).
					WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, client kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
							getCalls++
							if tc.getErr != nil {
								return tc.getErr
							}

							return client.Get(ctx, key, obj, opts...)
						},
						Delete: func(_ context.Context, _ kclient.WithWatch, _ kclient.Object, _ ...kclient.DeleteOption) error {
							deleteCalls++
							return tc.deleteErr
						},
					}).Build()

				// With no grants, a successful source lookup must reach deletion.
				indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
					"selectors":           cache.MetaNamespaceIndexFunc,
					"catalog-entry-names": cache.MetaNamespaceIndexFunc,
					"server-names":        cache.MetaNamespaceIndexFunc,
				})
				h := &Handler{accessControlRuleHelper: accesscontrolrule.NewAccessControlRuleHelper(indexer, client)}
				user := &userInfo{Info: &kuser.DefaultInfo{UID: "1"}}
				cleanup := h.deleteUnauthorizedServersForUser
				if resource == "instance" {
					cleanup = h.deleteUnauthorizedInstancesForUser
				}

				err := cleanup(t.Context(), client, system.DefaultNamespace, "1", user)

				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, 1, getCalls)
				require.Equal(t, tc.wantDeletes, deleteCalls)
			})
		}
	}
}
