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
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apimachinery/pkg/runtime"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileToolSelection(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "everything", Name: "everything"}},
		Profiles:   []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "team"}}, AllowedTools: types.VMCPToolSet{"everything": []string{"echo"}}}},
	}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		UserID:   "1",
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name, EnabledTools: types.VMCPToolSet{"everything": []string{"echo", "revoked"}}},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp, instance).Build()
	u := &kuser.DefaultInfo{UID: "1", Extra: map[string][]string{"obot_groups": {"team"}}}
	handler := &Handler{userInfo: func(context.Context, uint) (kuser.Info, error) { return u, nil }}
	req := router.Request{Ctx: t.Context(), Client: client, Object: instance}
	for _, want := range []types.VMCPToolSet{{"everything": []string{"echo"}}, {}, {}} {
		if err := handler.ReconcileToolSelection(req, nil); err != nil {
			t.Fatal(err)
		}
		if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(instance.Spec.Manifest.EnabledTools, want) {
			t.Fatalf("selection = %#v, want %#v", instance.Spec.Manifest.EnabledTools, want)
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
	instance.Spec.Manifest.EnabledTools = nil
	if err := handler.ReconcileToolSelection(req, nil); err != nil || instance.Spec.Manifest.EnabledTools != nil {
		t.Fatalf("implicit selection changed: %v", err)
	}
}

func TestReconcileToolSelectionDropsInvalidSelectionWithAllowAllTools(t *testing.T) {
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
			Subjects:      []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			AllowAllTools: true,
		}},
	}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		UserID: "1",
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name, EnabledTools: types.VMCPToolSet{
			"everything": []string{"echo"},
			"":           []string{"echo"}, // Legacy name is ambiguous and must remain denied.
		}},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp, instance).Build()
	handler := &Handler{userInfo: func(context.Context, uint) (kuser.Info, error) {
		return &kuser.DefaultInfo{UID: "1"}, nil
	}}
	if err := handler.ReconcileToolSelection(router.Request{Ctx: t.Context(), Client: client, Object: instance}, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance); err != nil {
		t.Fatal(err)
	}
	want := types.VMCPToolSet{"everything": []string{"echo"}}
	if !reflect.DeepEqual(instance.Spec.Manifest.EnabledTools, want) {
		t.Fatalf("selection = %#v, want %#v", instance.Spec.Manifest.EnabledTools, want)
	}
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
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.VMCPInstance{}).
		WithObjects(vmcp, instance).
		Build()
	userKey := vmcpconfig.ConfigurationKey("component-one", "HEADER")
	reveals := 0
	handler := &Handler{
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
		},
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.VMCPInstance{}).
		WithObjects(vmcp, instance).
		Build()
	handler := &Handler{
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

	vmcp.Spec.Manifest.ForceSingleUser = true
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
