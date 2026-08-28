package vmcp_test

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	vmcphandler "github.com/obot-platform/obot/pkg/controller/handlers/vmcp"
	"github.com/obot-platform/obot/pkg/controller/handlers/vmcpinstance"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSharingTransitions(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "one", CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Name: "cached", Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
		}}}},
	}}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }).Build()
	req := router.Request{Ctx: t.Context(), Client: client, Object: vmcp}
	instances := []*v1.VMCPInstance{
		{Name: "vmcpi1one", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "1", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}},
		{Name: "vmcpi1two", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "2", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}},
	}
	for _, tc := range []struct {
		name            string
		policy          types.VMCPConfigurationPolicyType
		usage           types.Usage
		forceSingleUser bool
		shared          bool
	}{
		{
			name:   "no user configuration",
			shared: true,
		},
		{
			name:   "user environment",
			policy: types.VMCPConfigurationPolicyUserAllowed,
			usage:  types.Env,
		},
		{
			name:   "fixed environment",
			policy: types.VMCPConfigurationPolicyFixed,
			usage:  types.Env,
			shared: true,
		},
		{
			name:   "user file",
			policy: types.VMCPConfigurationPolicyUserAllowed,
			usage:  types.File,
		},
		{
			name:   "user header",
			policy: types.VMCPConfigurationPolicyUserAllowed,
			usage:  types.Header,
			shared: true,
		},
		{
			name:            "forced single user",
			policy:          types.VMCPConfigurationPolicyUserAllowed,
			usage:           types.Header,
			forceSingleUser: true,
		},
		{
			name:   "shared again",
			policy: types.VMCPConfigurationPolicyUserAllowed,
			usage:  types.Header,
			shared: true,
		},
	} {
		t.Log(tc.name)
		shared := tc.shared
		vmcp.Spec.Manifest.ForceSingleUser = tc.forceSingleUser
		component := &vmcp.Spec.Manifest.Components[0]
		if tc.policy != "" {
			component.Configuration = []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: tc.policy}}
			component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "TOKEN", Usage: tc.usage}}
		}
		if err := client.Update(t.Context(), vmcp); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := vmcphandler.EnsureMCPServers(req, nil); err != nil {
				t.Fatal(err)
			}
			for _, instance := range instances {
				if err := vmcpinstance.New(nil).EnsureMCPServers(router.Request{Ctx: t.Context(), Client: client, Object: instance}, nil); err != nil {
					t.Fatal(err)
				}
			}
		}
		var servers v1.MCPServerList
		if err := client.List(t.Context(), &servers); err != nil {
			t.Fatal(err)
		}
		want := 2
		if shared {
			want = 1
		}
		if len(servers.Items) != want {
			t.Fatalf("shared=%v: got %d servers, want %d", shared, len(servers.Items), want)
		}
		for _, server := range servers.Items {
			if (server.Spec.VMCPID == vmcp.Name) != shared || server.Spec.IsSingleUser() == shared {
				t.Fatalf("wrong ownership: %#v", server.Spec)
			}
			if server.Spec.Manifest.RemoteConfig.URL != "https://example.com/mcp" {
				t.Fatal("snapshot not copied")
			}
		}
		vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Name += "-updated"
		if err := client.Update(t.Context(), vmcp); err != nil {
			t.Fatal(err)
		}
		if err := vmcphandler.EnsureMCPServers(req, nil); err != nil {
			t.Fatal(err)
		}
		for _, instance := range instances {
			if err := vmcpinstance.New(nil).EnsureMCPServers(router.Request{Ctx: t.Context(), Client: client, Object: instance}, nil); err != nil {
				t.Fatal(err)
			}
		}
		for _, old := range servers.Items {
			var updated v1.MCPServer
			if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(&old), &updated); err != nil {
				t.Fatal(err)
			}
			if updated.Spec.Manifest.Name != vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Name {
				t.Fatal("existing component server did not adopt updated snapshot")
			}
		}
	}
}

func TestEnsureMCPServersDeletesRemovedComponents(t *testing.T) {
	component := func(id string) types.VMCPComponent {
		return types.VMCPComponent{
			ID: id,
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Name: id, Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
			}},
		}
	}
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{component("removed"), component("retained")},
	}}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPID} }).Build()
	req := router.Request{Ctx: t.Context(), Client: client, Object: vmcp}
	if err := vmcphandler.EnsureMCPServers(req, nil); err != nil {
		t.Fatal(err)
	}
	vmcp.Spec.Manifest.Components = vmcp.Spec.Manifest.Components[1:]
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	if err := vmcphandler.EnsureMCPServers(req, nil); err != nil {
		t.Fatal(err)
	}
	var servers v1.MCPServerList
	if err := client.List(t.Context(), &servers); err != nil {
		t.Fatal(err)
	}
	if len(servers.Items) != 1 || servers.Items[0].Spec.VMCPComponentID != "retained" {
		t.Fatalf("servers after component removal = %#v", servers.Items)
	}
}
