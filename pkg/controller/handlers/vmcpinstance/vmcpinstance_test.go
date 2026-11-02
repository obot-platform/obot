package vmcpinstance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func withUserChangeWatches(t *testing.T, builder *fake.ClientBuilder, userID string) *fake.ClientBuilder {
	t.Helper()
	return builder.
		WithIndex(&v1.UserGroupChange{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.UserGroupChange).Get("spec.userID")}
		}).
		WithIndex(&v1.UserRoleChange{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.UserRoleChange).Get("spec.userID")}
		}).
		WithInterceptorFuncs(interceptor.Funcs{List: func(ctx context.Context, client kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			switch list.(type) {
			case *v1.UserGroupChangeList, *v1.UserRoleChangeList:
				options := (&kclient.ListOptions{}).ApplyOptions(opts)
				require.Equal(t, "default", options.Namespace)
				require.NotNil(t, options.FieldSelector)
				require.Equal(t, "spec.userID="+userID, options.FieldSelector.String())
			}
			return client.List(ctx, list, opts...)
		}})
}

func TestDeleteUnauthorized(t *testing.T) {
	for _, tc := range []struct {
		name      string
		subjects  []types.Subject
		extra     map[string][]string
		personal  bool
		missing   bool
		userError bool
		invalidID bool
		deleted   bool
	}{
		{
			name:    "no profiles",
			deleted: true,
		},
		{
			name: "user matches second profile",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeUser,
					ID:   "1",
				},
			},
		},
		{
			name: "wildcard matches",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeSelector,
					ID:   "*",
				},
			},
		},
		{
			name: "obot group matches",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeObotGroup,
					ID:   "team",
				},
			},
			extra: map[string][]string{"obot_groups": {"team"}},
		},
		{
			name: "provider group matches",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeGroup,
					ID:   "team",
				},
			},
			extra: map[string][]string{"auth_provider_groups": {"team"}},
		},
		{
			name: "group membership lost",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeGroup,
					ID:   "team",
				},
			},
			deleted: true,
		},
		{
			name:     "personal VMCP needs no profiles",
			personal: true,
		},
		{
			name:    "missing VMCP left to reference cleanup",
			missing: true,
		},
		{
			name:      "lookup error preserves instance",
			userError: true,
		},
		{
			name:      "invalid user ID preserves instance",
			invalidID: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v1.AddToScheme(scheme))
			vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default"}
			if tc.personal {
				vmcp.Spec.UserID = "1"
			}
			if tc.subjects != nil {
				vmcp.Spec.Manifest.Profiles = []types.VMCPProfile{
					{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "2"}}},
					{Subjects: tc.subjects},
				}
			}
			// Access must be checked even without an explicit tool selection.
			instance := &v1.VMCPInstance{
				Name:      "vmcpi1test",
				Namespace: "default",
				Spec: v1.VMCPInstanceSpec{
					UserID:   "1",
					Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
				},
			}
			if tc.invalidID {
				instance.Spec.UserID = "invalid"
			}
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(instance)
			withUserChangeWatches(t, builder, instance.Spec.UserID)
			if !tc.missing {
				builder.WithObjects(vmcp)
			}
			client := builder.Build()
			handler := &Handler{userInfo: func(_ context.Context, id uint) (kuser.Info, error) {
				require.False(t, tc.personal || tc.missing || tc.invalidID)
				require.Equal(t, uint(1), id)
				if tc.userError {
					return nil, errors.New("lookup failed")
				}
				return &kuser.DefaultInfo{UID: "1", Extra: tc.extra}, nil
			}}
			for range 2 {
				err := handler.DeleteUnauthorized(router.Request{Ctx: t.Context(), Client: client, Object: instance}, nil)
				if tc.userError || tc.invalidID {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				err = client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &v1.VMCPInstance{})
				if tc.deleted {
					require.True(t, apierrors.IsNotFound(err), "expected deletion, got %v", err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestReconcileToolSelection(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "everything", Name: "everything"}},
		Profiles:   []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeObotGroup, ID: "team"}}, Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo"}}}}}},
	}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		UserID:   "1",
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name, ComponentSet: map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo", "revoked"}}}},
	}}
	client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp, instance), instance.Spec.UserID).Build()
	u := &kuser.DefaultInfo{UID: "1", Extra: map[string][]string{"obot_groups": {"team"}}}
	handler := &Handler{userInfo: func(context.Context, uint) (kuser.Info, error) { return u, nil }}
	req := router.Request{Ctx: t.Context(), Client: client, Object: instance}
	for _, want := range []map[string]types.VMCPComponentSet{{"everything": {AllowedTools: []string{"echo"}}}, {}, {}} {
		if err := handler.ReconcileToolSelection(req, nil); err != nil {
			t.Fatal(err)
		}
		if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(instance.Spec.Manifest.ComponentSet, want) {
			t.Fatalf("selection = %#v, want %#v", instance.Spec.Manifest.ComponentSet, want)
		}
		// Losing the group removes the last tool. Regaining it must not restore selection.
		if len(u.Extra["obot_groups"]) > 0 {
			u.Extra = nil
		} else {
			u.Extra = map[string][]string{"obot_groups": {"team"}}
		}
		data, err := json.Marshal(instance)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, instance); err != nil {
			t.Fatal(err)
		}
	}
	instance.Spec.Manifest.ComponentSet = nil
	if err := handler.ReconcileToolSelection(req, nil); err != nil || instance.Spec.Manifest.ComponentSet != nil {
		t.Fatalf("implicit selection changed: %v", err)
	}
}

func TestReconcileToolSelectionKeepsOwnerSelection(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{
		UserID: "1",
		Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{{ID: "everything", Name: "everything"}},
		},
	}}
	selection := map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo"}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		UserID: "1", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name, ComponentSet: selection},
	}}
	client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp, instance), instance.Spec.UserID).Build()
	handler := &Handler{userInfo: func(context.Context, uint) (kuser.Info, error) {
		return &kuser.DefaultInfo{UID: "1"}, nil
	}}
	if err := handler.ReconcileToolSelection(router.Request{Ctx: t.Context(), Client: client, Object: instance}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(instance.Spec.Manifest.ComponentSet, selection) {
		t.Fatalf("selection = %#v, want %#v", instance.Spec.Manifest.ComponentSet, selection)
	}
}

func TestReconcileToolSelectionDropsInvalidSelectionWithAllowAllComponents(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{
			ID: "everything", Name: "everything", ToolOverrides: []types.ToolOverride{{Name: "echo", Enabled: true}},
		}, {
			ID: "other", Name: "other", ToolOverrides: []types.ToolOverride{{Name: "echo", Enabled: true}},
		}},
		Profiles: []types.VMCPProfile{{
			Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		}},
	}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		UserID: "1",
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name, ComponentSet: map[string]types.VMCPComponentSet{
			"everything": {AllowedTools: []string{"echo"}},
			"":           {AllowedTools: []string{"echo"}}, // Legacy name is ambiguous and must remain denied.
		}},
	}}
	client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp, instance), instance.Spec.UserID).Build()
	handler := &Handler{userInfo: func(context.Context, uint) (kuser.Info, error) {
		return &kuser.DefaultInfo{UID: "1"}, nil
	}}
	if err := handler.ReconcileToolSelection(router.Request{Ctx: t.Context(), Client: client, Object: instance}, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance); err != nil {
		t.Fatal(err)
	}
	want := map[string]types.VMCPComponentSet{"everything": {AllowedTools: []string{"echo"}}}
	if !reflect.DeepEqual(instance.Spec.Manifest.ComponentSet, want) {
		t.Fatalf("selection = %#v, want %#v", instance.Spec.Manifest.ComponentSet, want)
	}
}

func allowAllProfiles() []types.VMCPProfile {
	return []types.VMCPProfile{{
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	}}
}

func staticUserInfo(u kuser.Info) func(context.Context, uint) (kuser.Info, error) {
	return func(context.Context, uint) (kuser.Info, error) { return u, nil }
}

func TestSyncUserConfigurationHash(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	vmcp := &v1.VMCP{
		Name:      "vmcp1test",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{
					{
						ID: "component-one",
						Configuration: []types.VMCPConfigurationPolicy{
							{
								Key:    "HEADER",
								Policy: types.VMCPConfigurationPolicyUserAllowed,
							},
							{
								Key:    "STATIC",
								Policy: types.VMCPConfigurationPolicyFixed,
							},
						},
					},
				},
				Profiles: allowAllProfiles(),
			},
		},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1test",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcp.Name,
			},
			UserID: "1",
		},
	}
	client := withUserChangeWatches(t, fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.VMCPInstance{}).
		WithObjects(vmcp, instance), instance.Spec.UserID).
		Build()
	userKey := vmcpconfig.ConfigurationKey("component-one", "HEADER")
	reveals := 0
	handler := &Handler{
		userInfo: staticUserInfo(&kuser.DefaultInfo{UID: "1"}),
		revealCredential: func(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
			reveals++
			if len(contexts) != 1 || contexts[0] != vmcpconfig.InstanceConfigurationCredentialContext(instance.Name) {
				t.Fatalf("credential contexts = %#v", contexts)
			}
			if name != vmcpconfig.ConfigurationCredentialName() {
				t.Fatalf("credential name = %q", name)
			}
			return gatewaytypes.Credential{
				Secrets: map[string]string{
					userKey: "user-value",
					vmcpconfig.ConfigurationKey("component-one", "STATIC"): "fixed-value",
					vmcpconfig.ConfigurationKey("unknown", "UNKNOWN"):      "unknown-value",
				},
			}, nil
		},
	}

	if err := handler.SyncUserConfigurationHash(router.Request{
		Ctx:    t.Context(),
		Client: client,
		Object: instance,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var updated v1.VMCPInstance
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &updated); err != nil {
		t.Fatal(err)
	}
	want := utils.Digest(map[string]string{userKey: "user-value"})
	if updated.Status.UserConfigurationHash != want {
		t.Fatalf("user configuration hash = %q, want %q", updated.Status.UserConfigurationHash, want)
	}
	req := router.Request{Ctx: t.Context(), Client: client, Object: &updated}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if reveals != 1 {
		t.Fatal("unchanged configuration was revealed again")
	}
	updated.Annotations = map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: "updated"}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if reveals != 2 {
		t.Fatal("configuration update did not recheck credential")
	}
	vmcp.Spec.Manifest.Components[0].Configuration[0].Policy = types.VMCPConfigurationPolicyFixed
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if reveals != 3 || updated.Status.UserConfigurationHash != utils.Digest(map[string]string{}) {
		t.Fatal("policy update did not recompute the filtered configuration")
	}
}

func TestSyncUserConfigurationHashUsesEmptyConfigurationWhenCredentialIsMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1test",
		Namespace: "default",
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1test",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcp.Name,
			},
			UserID: "1",
		},
	}
	client := withUserChangeWatches(t, fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.VMCPInstance{}).
		WithObjects(vmcp, instance), instance.Spec.UserID).
		Build()
	handler := &Handler{
		userInfo: staticUserInfo(&kuser.DefaultInfo{UID: "1"}),
		revealCredential: func(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
			return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{
				Contexts: contexts,
				Name:     name,
			}
		},
	}

	if err := handler.SyncUserConfigurationHash(router.Request{
		Ctx:    t.Context(),
		Client: client,
		Object: instance,
	}, nil); err != nil {
		t.Fatal(err)
	}

	var updated v1.VMCPInstance
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &updated); err != nil {
		t.Fatal(err)
	}
	want := utils.Digest(map[string]string{})
	if updated.Status.UserConfigurationHash != want {
		t.Fatalf("user configuration hash = %q, want %q", updated.Status.UserConfigurationHash, want)
	}
	// Cache missing credentials too, but retry failed checks of a new version.
	handler.revealCredential = func(context.Context, []string, string) (gatewaytypes.Credential, error) {
		return gatewaytypes.Credential{}, errors.New("unavailable")
	}
	req := router.Request{Ctx: t.Context(), Client: client, Object: &updated}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatalf("cached missing credential was queried again: %v", err)
	}
	previousHash := updated.Status.ConfigurationCheckHash
	updated.Annotations = map[string]string{v1.VMCPInstanceConfigurationSyncAnnotation: "retry"}
	for range 2 {
		if err := handler.SyncUserConfigurationHash(req, nil); err == nil {
			t.Fatal("expected credential error")
		}
		if updated.Status.ConfigurationCheckHash != previousHash {
			t.Fatal("failed reveal was acknowledged")
		}
	}
}

func TestEnsureMCPServersCreatesServersFromCachedComponents(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	vmcp := &v1.VMCP{
		Name:      "vmcp1test",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			UserID: "user-1",
			Manifest: types.VMCPManifest{
				DisplayName: "Test VMCP",
				Components: []types.VMCPComponent{
					{
						ID:                      "component-one",
						Name:                    "one",
						MCPServerCatalogEntryID: "entry-one",
						CatalogEntry: types.MCPServerCatalogEntrySnapshot{
							Manifest: types.MCPServerCatalogEntryManifest{
								Name:      "cached-npx",
								Runtime:   types.RuntimeNPX,
								NPXConfig: &types.NPXRuntimeConfig{Package: "cached-package"},
								Config: []types.MCPConfig{{
									Key:      "TOKEN",
									Required: true,
									Usage:    types.Env,
								}},
								Description: "cached description",
							},
							UnsupportedTools: []string{"broken-tool"},
						},
					},
					{
						ID:                      "component-two",
						Name:                    "two",
						MCPServerCatalogEntryID: "entry-two",
						CatalogEntry: types.MCPServerCatalogEntrySnapshot{
							Manifest: types.MCPServerCatalogEntryManifest{
								Name:    "cached-container",
								Runtime: types.RuntimeContainerized,
								ContainerizedConfig: &types.ContainerizedRuntimeConfig{
									Image: "example.test/component:latest",
									Port:  8080,
								},
							},
						},
					},
				},
			},
		},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1test",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcp.Name,
			},
			UserID: "user-1",
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }).Build()
	req := router.Request{
		Ctx:    t.Context(),
		Client: client,
		Object: instance,
	}

	for i := range vmcp.Spec.Manifest.Components {
		vmcp.Spec.Manifest.Components[i].ForceSingleUser = true
	}
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	handler := New(nil)
	if err := handler.EnsureMCPServers(req, nil); err != nil {
		t.Fatal(err)
	}
	// A repeated reconciliation must reuse the same deterministic servers.
	if err := handler.EnsureMCPServers(req, nil); err != nil {
		t.Fatal(err)
	}

	var servers v1.MCPServerList
	if err := client.List(t.Context(), &servers, kclient.InNamespace("default")); err != nil {
		t.Fatal(err)
	}
	if len(servers.Items) != 2 {
		t.Fatalf("expected two MCPServers, got %d", len(servers.Items))
	}

	serversByComponent := make(map[string]v1.MCPServer, len(servers.Items))
	for _, server := range servers.Items {
		serversByComponent[server.Spec.VMCPComponentID] = server
		if server.Spec.VMCPInstanceID != instance.Name {
			t.Errorf("server %q VMCP instance = %q, want %q", server.Name, server.Spec.VMCPInstanceID, instance.Name)
		}
		if server.Spec.UserID != instance.Spec.UserID {
			t.Errorf("server %q user = %q, want %q", server.Name, server.Spec.UserID, instance.Spec.UserID)
		}
		wantEntry := "entry-one"
		if server.Spec.VMCPComponentID == "component-two" {
			wantEntry = "entry-two"
		}
		if server.Spec.MCPServerCatalogEntryName != wantEntry {
			t.Errorf("server %q catalog entry = %q, want %q", server.Name, server.Spec.MCPServerCatalogEntryName, wantEntry)
		}
	}

	npxServer := serversByComponent["component-one"]
	if npxServer.Spec.Manifest.Name != "cached-npx" || npxServer.Spec.Manifest.NPXConfig == nil || npxServer.Spec.Manifest.NPXConfig.Package != "cached-package" {
		t.Fatalf("NPX server was not created from cached catalog configuration: %#v", npxServer.Spec.Manifest)
	}
	if len(npxServer.Spec.Manifest.Config) != 1 || npxServer.Spec.Manifest.Config[0].Key != "TOKEN" {
		t.Fatalf("NPX server did not retain cached environment schema: %#v", npxServer.Spec.Manifest.Config)
	}
	if len(npxServer.Spec.UnsupportedTools) != 1 || npxServer.Spec.UnsupportedTools[0] != "broken-tool" {
		t.Fatalf("NPX server did not retain cached unsupported tools: %#v", npxServer.Spec.UnsupportedTools)
	}
	npxServer.Spec.MCPServerCatalogEntryName = "stale-entry"
	if err := client.Update(t.Context(), &npxServer); err != nil {
		t.Fatal(err)
	}
	if err := handler.EnsureMCPServers(req, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(&npxServer), &npxServer); err != nil {
		t.Fatal(err)
	}
	if npxServer.Spec.MCPServerCatalogEntryName != "entry-one" {
		t.Fatalf("NPX server catalog entry = %q, want entry-one", npxServer.Spec.MCPServerCatalogEntryName)
	}

	containerServer := serversByComponent["component-two"]
	if containerServer.Spec.Manifest.ContainerizedConfig == nil || containerServer.Spec.Manifest.ContainerizedConfig.Image != "example.test/component:latest" {
		t.Fatalf("container server was not created from cached catalog configuration: %#v", containerServer.Spec.Manifest)
	}

	vmcp.Spec.Manifest.Components = vmcp.Spec.Manifest.Components[1:]
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	if err := handler.EnsureMCPServers(req, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.List(t.Context(), &servers, kclient.InNamespace("default")); err != nil {
		t.Fatal(err)
	}
	if len(servers.Items) != 1 || servers.Items[0].Spec.VMCPComponentID != "component-two" {
		t.Fatalf("servers after component removal = %#v", servers.Items)
	}
}

func TestEnsureMCPServersIgnoresMissingVMCP(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1missing",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: "vmcp1missing",
			},
		},
	}

	err := New(nil).EnsureMCPServers(router.Request{
		Ctx:    t.Context(),
		Client: client,
		Object: instance,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDedicatedComponentHeadersUseCredentials(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy types.VMCPConfigurationPolicyType
		value  string
	}{
		{
			name:   "user allowed",
			policy: types.VMCPConfigurationPolicyUserAllowed,
			value:  "user-token",
		},
		{
			name:   "fixed",
			policy: types.VMCPConfigurationPolicyFixed,
			value:  "fixed-token",
		},
		{
			name:   "prohibited",
			policy: types.VMCPConfigurationPolicyProhibited,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			component := types.VMCPComponent{
				ID:              "one",
				ForceSingleUser: true,
				Configuration: []types.VMCPConfigurationPolicy{{
					Key:    "Authorization",
					Policy: tc.policy,
				}},
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
					Config: []types.MCPConfig{{
						Key:         "Authorization",
						Usage:       types.Header,
						UserAllowed: true,
					}},
				}},
			}
			instance := &v1.VMCPInstance{
				Name:      "vmcpi1test",
				Namespace: "default",
				Spec: v1.VMCPInstanceSpec{
					UserID:   "1",
					Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1test"},
				},
			}
			server, err := mcpServerForComponent(instance, component)
			require.NoError(t, err)
			require.True(t, component.CatalogEntry.Manifest.Config[0].UserAllowed, "must not mutate the snapshot")
			check := func(server v1.MCPServer) {
				t.Helper()
				config, missing, err := mcp.ServerToServerConfig(server, nil, "1", "", "", map[string]string{"Authorization": tc.value}, false)
				require.NoError(t, err)
				require.Empty(t, missing)
				require.Empty(t, config.PassthroughHeaderNames)
				if tc.value == "" {
					require.Empty(t, config.Headers)
				} else {
					require.Equal(t, []string{"Authorization=" + tc.value}, config.Headers)
				}
			}
			check(server)

			// Repair a server created before normalization even when its snapshot
			// digest is unchanged (as it is for policy-only updates).
			server.Spec.Manifest.Config[0].UserAllowed = true
			parent := &v1.VMCP{
				Name:      instance.Spec.Manifest.VMCPID,
				Namespace: instance.Namespace,
				Spec: v1.VMCPSpec{
					UserID: instance.Spec.UserID,
					Manifest: types.VMCPManifest{
						Components: []types.VMCPComponent{component},
					},
				},
			}
			scheme := runtime.NewScheme()
			require.NoError(t, v1.AddToScheme(scheme))
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(parent, &server).
				WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string {
					return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID}
				}).Build()
			req := router.Request{Ctx: t.Context(), Client: client, Object: instance}
			require.NoError(t, New(nil).EnsureMCPServers(req, nil))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&server), &server))
			check(server)
			version := server.ResourceVersion
			require.NoError(t, New(nil).EnsureMCPServers(req, nil))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&server), &server))
			require.Equal(t, version, server.ResourceVersion, "unchanged configuration must not trigger another update")
		})
	}
}

func dedicatedTestComponent(id string) types.VMCPComponent {
	return types.VMCPComponent{
		ID:              id,
		ForceSingleUser: true,
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:    id,
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://example.com/mcp",
				},
			},
		},
	}
}

func TestEnsureMCPServersProfileAccessLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name        string
		subjectType types.SubjectType
		subjectID   string
		extraKey    string
	}{
		{
			name:        "provider group membership",
			subjectType: types.SubjectTypeGroup,
			subjectID:   "team",
			extraKey:    "auth_provider_groups",
		},
		{
			name:        "effective role membership",
			subjectType: types.SubjectTypeObotGroup,
			subjectID:   types.RoleAdmin.Groups()[0],
			extraKey:    "obot_groups",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v1.AddToScheme(scheme))

			retained := dedicatedTestComponent("retained")
			// User environment configuration also requires a dedicated server without ForceSingleUser.
			retained.ForceSingleUser = false
			retained.Configuration = []types.VMCPConfigurationPolicy{{
				Key:    "TOKEN",
				Policy: types.VMCPConfigurationPolicyUserAllowed,
			}}
			retained.CatalogEntry.Manifest.Config = []types.MCPConfig{{
				Key:   "TOKEN",
				Usage: types.Env,
			}}
			vmcp := &v1.VMCP{
				Name:      "vmcp1shared",
				Namespace: "default",
				Spec: v1.VMCPSpec{
					Manifest: types.VMCPManifest{
						Components: []types.VMCPComponent{dedicatedTestComponent("revoked"), retained},
						Profiles: []types.VMCPProfile{
							{
								Subjects: []types.Subject{{
									Type: types.SubjectTypeUser,
									ID:   "1",
								}},
								Permissions: types.VMCPProfilePermissions{
									AllowedComponents: map[string]types.VMCPComponentSet{"retained": {}},
								},
							},
							{
								Subjects: []types.Subject{{
									Type: tc.subjectType,
									ID:   tc.subjectID,
								}},
								Permissions: types.VMCPProfilePermissions{
									AllowedComponents: map[string]types.VMCPComponentSet{"revoked": {AllowedTools: []string{"*"}}},
								},
							},
							{
								Subjects: []types.Subject{{
									Type: types.SubjectTypeUser,
									ID:   "2",
								}},
								Permissions: types.VMCPProfilePermissions{
									AllowAllComponents: true,
								},
							},
						},
					},
				},
			}
			instance := &v1.VMCPInstance{
				Name:      "vmcpi1first",
				Namespace: "default",
				Spec: v1.VMCPInstanceSpec{
					UserID: "1",
					Manifest: types.VMCPInstanceManifest{
						VMCPID: vmcp.Name,
						// Selecting the revoked component must not preserve it, nor exclude the retained one.
						ComponentSet: map[string]types.VMCPComponentSet{"revoked": {AllowedTools: []string{"*"}}},
					},
				},
			}
			legacy := *retained.DeepCopy()
			legacy.SourceDigest = utils.Digest([]any{retained, ""})
			legacy.CatalogEntry.Manifest.Name = "legacy-retained"
			instance.Spec.LegacyComponents = []types.VMCPComponent{legacy}
			other := instance.DeepCopy()
			other.Name = "vmcpi1other"
			other.Spec.UserID = "2"
			otherServer, err := mcpServerForComponent(other, vmcp.Spec.Manifest.Components[0])
			require.NoError(t, err)

			client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(vmcp, instance, other, &otherServer).
				WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.VMCPInstanceID}
				}), instance.Spec.UserID).Build()
			u := &kuser.DefaultInfo{
				UID:   "1",
				Extra: map[string][]string{tc.extraKey: {tc.subjectID}},
			}
			lookups := 0
			handler := &Handler{userInfo: func(_ context.Context, id uint) (kuser.Info, error) {
				require.Equal(t, uint(1), id)
				lookups++
				return u, nil
			}}
			req := router.Request{
				Ctx:    t.Context(),
				Client: client,
				Object: instance,
			}
			reconciliations := 0
			// Once deletion starts, reconciliation must leave the server to its finalizer.
			req.Client = interceptor.NewClient(client, interceptor.Funcs{
				Delete: func(ctx context.Context, client kclient.WithWatch, obj kclient.Object, opts ...kclient.DeleteOption) error {
					require.True(t, obj.GetDeletionTimestamp().IsZero(), "must not delete a terminating server again")
					return client.Delete(ctx, obj, opts...)
				},
			})

			reconcile := func() {
				t.Helper()
				require.NoError(t, handler.EnsureMCPServers(req, nil))
				reconciliations++
			}
			reconcile()
			var servers v1.MCPServerList
			require.NoError(t, client.List(t.Context(), &servers, kclient.MatchingFields{"spec.vmcpInstanceID": instance.Name}))
			require.Len(t, servers.Items, 2)
			var revokedServer, retainedServer v1.MCPServer
			for _, server := range servers.Items {
				if server.Spec.VMCPComponentID == "revoked" {
					revokedServer = server
				} else {
					retainedServer = server
				}
			}
			require.Equal(t, "legacy-retained", retainedServer.Spec.Manifest.Name)
			retainedServer.UID = "retained-server"
			require.NoError(t, client.Update(t.Context(), &retainedServer))
			revokedServer.Finalizers = []string{v1.MCPServerFinalizer}
			require.NoError(t, client.Update(t.Context(), &revokedServer))
			parentBefore := vmcp.DeepCopy()
			instanceBefore := instance.DeepCopy()
			otherBefore := other.DeepCopy()
			otherServerBefore := otherServer.DeepCopy()

			u.Extra = nil
			for range 2 {
				reconcile()
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&revokedServer), &revokedServer))
				require.False(t, revokedServer.DeletionTimestamp.IsZero(), "revoked dedicated server must be finalized")
			}

			// Simulate the existing server finalizer; the instance configuration stays intact.
			revokedServer.Finalizers = nil
			require.NoError(t, client.Update(t.Context(), &revokedServer))
			reconcile()
			require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(&revokedServer), &v1.MCPServer{})))

			u.Extra = map[string][]string{tc.extraKey: {tc.subjectID}}
			reconcile()
			var restored v1.MCPServer
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&revokedServer), &restored))
			require.True(t, restored.DeletionTimestamp.IsZero())
			require.Equal(t, revokedServer.Name, restored.Name)
			require.Equal(t, revokedServer.Spec, restored.Spec)
			reconcile()

			var retainedAfter v1.MCPServer
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&retainedServer), &retainedAfter))
			require.Equal(t, retainedServer, retainedAfter)
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), vmcp))
			require.Equal(t, parentBefore, vmcp)
			require.Equal(t, instanceBefore, instance, "must not mutate instance or legacy snapshots")
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance))
			require.Equal(t, instanceBefore, instance)
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(other), other))
			require.Equal(t, otherBefore, other)
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&otherServer), &otherServer))
			require.Equal(t, otherServerBefore, &otherServer)
			require.Equal(t, reconciliations, lookups, "each reconciliation needs fresh user info")
		})
	}
}

func TestEnsureMCPServersComponentGrants(t *testing.T) {
	for _, tc := range []struct {
		name       string
		grants     []types.VMCPProfilePermissions
		selection  map[string]types.VMCPComponentSet
		personal   bool
		wantServer bool
	}{
		{
			name: "remaining matching profile",
			grants: []types.VMCPProfilePermissions{
				{
					AllowedComponents: map[string]types.VMCPComponentSet{"other": {}},
				},
				{
					AllowedComponents: map[string]types.VMCPComponentSet{"one": {}},
				},
			},
			wantServer: true,
		},
		{
			name: "allow all components",
			grants: []types.VMCPProfilePermissions{{
				AllowAllComponents: true,
			}},
			wantServer: true,
		},
		{
			name: "narrowed tool permissions",
			grants: []types.VMCPProfilePermissions{{
				AllowedComponents: map[string]types.VMCPComponentSet{"one": {AllowedTools: []string{"echo"}}},
			}},
			wantServer: true,
		},
		{
			name: "empty tool grant and selection",
			grants: []types.VMCPProfilePermissions{{
				AllowedComponents: map[string]types.VMCPComponentSet{"one": {AllowedTools: []string{}}},
			}},
			selection:  map[string]types.VMCPComponentSet{},
			wantServer: true,
		},
		{
			name:      "selection cannot enable a revoked component",
			grants:    []types.VMCPProfilePermissions{{}},
			selection: map[string]types.VMCPComponentSet{"one": {AllowedTools: []string{"*"}}},
		},
		{
			name:       "personal parent without profiles or numeric owner",
			personal:   true,
			wantServer: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v1.AddToScheme(scheme))
			vmcp := &v1.VMCP{
				Name:      "vmcp1test",
				Namespace: "default",
				Spec: v1.VMCPSpec{
					Manifest: types.VMCPManifest{
						Components: []types.VMCPComponent{dedicatedTestComponent("one")},
						Profiles:   allowAllProfiles(),
					},
				},
			}
			instance := &v1.VMCPInstance{
				Name:      "vmcpi1test",
				Namespace: "default",
				Spec: v1.VMCPInstanceSpec{
					UserID: "1",
					Manifest: types.VMCPInstanceManifest{
						VMCPID:       vmcp.Name,
						ComponentSet: tc.selection,
					},
				},
			}
			if tc.personal {
				vmcp.Spec.UserID = "personal-owner"
				instance.Spec.UserID = vmcp.Spec.UserID
			}

			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp).
				WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.VMCPInstanceID}
				})
			if !tc.personal {
				withUserChangeWatches(t, builder, instance.Spec.UserID)
			}
			client := builder.Build()
			handler := &Handler{userInfo: func(_ context.Context, id uint) (kuser.Info, error) {
				require.False(t, tc.personal, "personal parents must not look up user info")
				require.Equal(t, uint(1), id)
				return &kuser.DefaultInfo{UID: "1"}, nil
			}}
			req := router.Request{
				Ctx:    t.Context(),
				Client: client,
				Object: instance,
			}
			require.NoError(t, handler.EnsureMCPServers(req, nil))
			var before v1.MCPServerList
			require.NoError(t, client.List(t.Context(), &before))
			require.Len(t, before.Items, 1)

			// A nonmatching AllowAll profile must not affect the matching profiles' union.
			vmcp.Spec.Manifest.Profiles = []types.VMCPProfile{{
				Subjects: []types.Subject{{
					Type: types.SubjectTypeUser,
					ID:   "2",
				}},
				Permissions: types.VMCPProfilePermissions{
					AllowAllComponents: true,
				},
			}}
			for _, grant := range tc.grants {
				vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, types.VMCPProfile{
					Subjects: []types.Subject{{
						Type: types.SubjectTypeUser,
						ID:   "1",
					}},
					Permissions: grant,
				})
			}
			if tc.personal {
				vmcp.Spec.Manifest.Profiles = nil
			}
			require.NoError(t, client.Update(t.Context(), vmcp))
			for range 2 {
				require.NoError(t, handler.EnsureMCPServers(req, nil))
			}

			var after v1.MCPServerList
			require.NoError(t, client.List(t.Context(), &after))
			if tc.wantServer {
				require.Equal(t, before.Items, after.Items)
			} else {
				require.Empty(t, after.Items)
			}
		})
	}
}

func TestEnsureMCPServersIdentityFailuresPreserveServers(t *testing.T) {
	for _, tc := range []struct {
		name      string
		invalidID bool
		watch     string
	}{
		{
			name: "user lookup failure",
		},
		{
			name:      "invalid user ID",
			invalidID: true,
		},
		{
			name:  "group watch failure",
			watch: "group",
		},
		{
			name:  "role watch failure",
			watch: "role",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v1.AddToScheme(scheme))
			vmcp := &v1.VMCP{
				Name:      "vmcp1test",
				Namespace: "default",
				Spec: v1.VMCPSpec{
					Manifest: types.VMCPManifest{
						Components: []types.VMCPComponent{dedicatedTestComponent("update"), dedicatedTestComponent("create")},
						Profiles:   allowAllProfiles(),
					},
				},
			}
			instance := &v1.VMCPInstance{
				Name:      "vmcpi1test",
				Namespace: "default",
				Spec: v1.VMCPInstanceSpec{
					UserID: "1",
					Manifest: types.VMCPInstanceManifest{
						VMCPID: vmcp.Name,
					},
				},
			}
			if tc.invalidID {
				instance.Spec.UserID = "invalid"
			}
			updateServer, err := mcpServerForComponent(instance, dedicatedTestComponent("update"))
			require.NoError(t, err)
			updateServer.Annotations = nil
			deleteServer, err := mcpServerForComponent(instance, dedicatedTestComponent("delete"))
			require.NoError(t, err)
			failure := errors.New("identity unavailable")
			builder := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(vmcp, &updateServer, &deleteServer).
				WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.VMCPInstanceID}
				}), instance.Spec.UserID)
			client := builder.Build()
			var watched []string
			if tc.watch != "" {
				// Run the indexed list through withUserChangeWatches before injecting its failure.
				client = interceptor.NewClient(client, interceptor.Funcs{
					List: func(ctx context.Context, client kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
						if err := client.List(ctx, list, opts...); err != nil {
							return err
						}

						var watch string
						switch list.(type) {
						case *v1.UserGroupChangeList:
							watch = "group"
						case *v1.UserRoleChangeList:
							watch = "role"
						}
						if watch != "" {
							watched = append(watched, watch)
							if watch == tc.watch {
								return failure
							}
						}
						return nil
					},
				})
			}
			handler := &Handler{userInfo: func(_ context.Context, id uint) (kuser.Info, error) {
				require.False(t, tc.invalidID || tc.watch != "")
				require.Equal(t, uint(1), id)
				return nil, failure
			}}
			var before v1.MCPServerList
			require.NoError(t, client.List(t.Context(), &before))

			err = handler.EnsureMCPServers(router.Request{
				Ctx:    t.Context(),
				Client: client,
				Object: instance,
			}, nil)
			if tc.invalidID {
				require.ErrorContains(t, err, "invalid VMCP instance user ID")
			} else {
				require.ErrorIs(t, err, failure)
			}

			switch tc.watch {
			case "group":
				require.Equal(t, []string{"group"}, watched)
			case "role":
				require.Equal(t, []string{"group", "role"}, watched)
			}

			var after v1.MCPServerList
			require.NoError(t, client.List(t.Context(), &after))
			require.Equal(t, before.Items, after.Items, "identity failure must precede server deletion, update, and creation")
		})
	}
}

func TestEnsureMCPServersValidationAndOwnership(t *testing.T) {
	malformed := dedicatedTestComponent("one")
	malformed.CatalogEntry.Manifest.RemoteConfig = nil

	for _, tc := range []struct {
		name               string
		component          types.VMCPComponent
		foreignInstanceID  string
		foreignComponentID string
		wantError          string
	}{
		{
			name:      "enabled component requires an ID",
			component: dedicatedTestComponent(""),
			wantError: "component ID is required",
		},
		{
			name:      "enabled component requires valid runtime configuration",
			component: malformed,
			wantError: "remote configuration is required",
		},
		{
			name:              "different instance ownership",
			component:         dedicatedTestComponent("one"),
			foreignInstanceID: "vmcpi1other",
			wantError:         "different VMCP ownership",
		},
		{
			name:               "different component ownership",
			component:          dedicatedTestComponent("one"),
			foreignComponentID: "other",
			wantError:          "different VMCP ownership",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v1.AddToScheme(scheme))
			vmcp := &v1.VMCP{
				Name:      "vmcp1test",
				Namespace: "default",
				Spec: v1.VMCPSpec{
					Manifest: types.VMCPManifest{
						Components: []types.VMCPComponent{tc.component, dedicatedTestComponent("new")},
						Profiles:   allowAllProfiles(),
					},
				},
			}
			instance := &v1.VMCPInstance{
				Name:      "vmcpi1test",
				Namespace: "default",
				Spec: v1.VMCPInstanceSpec{
					UserID: "1",
					Manifest: types.VMCPInstanceManifest{
						VMCPID: vmcp.Name,
					},
				},
			}
			server, err := mcpServerForComponent(instance, dedicatedTestComponent("one"))
			require.NoError(t, err)
			if tc.foreignInstanceID != "" {
				server.Spec.VMCPInstanceID = tc.foreignInstanceID
			}
			if tc.foreignComponentID != "" {
				// Keep the other component desired so deletion does not remove the name collision first.
				vmcp.Spec.Manifest.Components = append(vmcp.Spec.Manifest.Components, dedicatedTestComponent(tc.foreignComponentID))
				server.Spec.VMCPComponentID = tc.foreignComponentID
			}
			client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp, &server).
				WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.VMCPInstanceID}
				}), instance.Spec.UserID).Build()
			handler := &Handler{userInfo: staticUserInfo(&kuser.DefaultInfo{UID: "1"})}
			before := server.DeepCopy()

			err = handler.EnsureMCPServers(router.Request{
				Ctx:    t.Context(),
				Client: client,
				Object: instance,
			}, nil)
			require.ErrorContains(t, err, tc.wantError)
			var after v1.MCPServerList
			require.NoError(t, client.List(t.Context(), &after))
			require.Equal(t, []v1.MCPServer{*before}, after.Items)
		})
	}
}
