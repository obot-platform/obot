package vmcpinstance

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
	handler := &Handler{
		revealCredential: func(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
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
						ID:   "component-one",
						Name: "one",
						CatalogEntry: types.MCPServerCatalogEntrySnapshot{
							Manifest: types.MCPServerCatalogEntryManifest{
								Name:        "cached-npx",
								Runtime:     types.RuntimeNPX,
								NPXConfig:   &types.NPXRuntimeConfig{Package: "cached-package"},
								Env:         []types.MCPEnv{{Key: "TOKEN", Required: true}},
								Description: "cached description",
							},
							UnsupportedTools: []string{"broken-tool"},
						},
					},
					{
						ID:   "component-two",
						Name: "two",
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
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmcp).Build()
	req := router.Request{
		Ctx:    t.Context(),
		Client: client,
		Object: instance,
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
	}

	npxServer := serversByComponent["component-one"]
	if npxServer.Spec.Manifest.Name != "cached-npx" || npxServer.Spec.Manifest.NPXConfig == nil || npxServer.Spec.Manifest.NPXConfig.Package != "cached-package" {
		t.Fatalf("NPX server was not created from cached catalog configuration: %#v", npxServer.Spec.Manifest)
	}
	if len(npxServer.Spec.Manifest.Env) != 1 || npxServer.Spec.Manifest.Env[0].Key != "TOKEN" {
		t.Fatalf("NPX server did not retain cached environment schema: %#v", npxServer.Spec.Manifest.Env)
	}
	if len(npxServer.Spec.UnsupportedTools) != 1 || npxServer.Spec.UnsupportedTools[0] != "broken-tool" {
		t.Fatalf("NPX server did not retain cached unsupported tools: %#v", npxServer.Spec.UnsupportedTools)
	}

	containerServer := serversByComponent["component-two"]
	if containerServer.Spec.Manifest.ContainerizedConfig == nil || containerServer.Spec.Manifest.ContainerizedConfig.Image != "example.test/component:latest" {
		t.Fatalf("container server was not created from cached catalog configuration: %#v", containerServer.Spec.Manifest)
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
