package vmcpinstance

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInstanceConfigurationStatus(t *testing.T) {
	component := types.VMCPComponent{
		ID:            "one",
		Configuration: []types.VMCPConfigurationPolicy{{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			RemoteConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{{Key: "HEADER", Required: true}}},
		}},
	}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, instance).WithStatusSubresource(instance).Build()
	values := map[string]string{}
	handler := &Handler{revealCredential: func(context.Context, []string, string) (gatewaytypes.Credential, error) {
		if len(values) == 0 {
			return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{}
		}
		return gatewaytypes.Credential{Secrets: values}, nil
	}}
	req := router.Request{Ctx: t.Context(), Client: client, Object: instance}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	key := vmcpconfig.ConfigurationKey("one", "HEADER")
	if instance.Status.Configured || len(instance.Status.MissingRequiredConfiguration) != 1 || instance.Status.MissingRequiredConfiguration[0] != key {
		t.Fatalf("missing shared header not reported: %+v", instance.Status)
	}
	// Schema changes must update status even when the credential hash is unchanged.
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.Headers[0].Required = false
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if !instance.Status.Configured || len(instance.Status.MissingRequiredConfiguration) != 0 {
		t.Fatalf("stale missing configuration: %+v", instance.Status)
	}
	vmcp.Spec.Manifest.ForceSingleUser = true
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.Headers[0].Required = true
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	values[key] = "secret header"
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if !instance.Status.Configured || len(instance.Status.MissingRequiredConfiguration) != 0 {
		t.Fatalf("supplied dedicated input reported missing: %+v", instance.Status)
	}
	version := instance.ResourceVersion
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if instance.ResourceVersion != version {
		t.Fatal("unchanged status was rewritten")
	}
}
