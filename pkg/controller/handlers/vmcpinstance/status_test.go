package vmcpinstance

import (
	"context"
	"slices"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInstanceConfigurationStatus(t *testing.T) {
	component := types.VMCPComponent{
		ID:            "one",
		Configuration: []types.VMCPConfigurationPolicy{{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "HEADER", Required: true, Usage: types.Header}},
		}},
	}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component}, Profiles: allowAllProfiles()}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "1", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, instance).WithStatusSubresource(instance), instance.Spec.UserID).Build()
	values := map[string]string{}
	handler := &Handler{
		revealCredential: func(context.Context, []string, string) (gatewaytypes.Credential, error) {
			if len(values) == 0 {
				return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{}
			}
			return gatewaytypes.Credential{Secrets: values}, nil
		},
		userInfo: staticUserInfo(&kuser.DefaultInfo{UID: "1"}),
	}
	req := router.Request{Ctx: t.Context(), Client: client, Object: instance}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	key := vmcpconfig.ConfigurationKey("one", "HEADER")
	if instance.Status.Configured || len(instance.Status.MissingRequiredConfiguration) != 1 || instance.Status.MissingRequiredConfiguration[0] != key {
		t.Fatalf("missing shared header not reported: %+v", instance.Status)
	}
	// Schema changes must update status even when the credential hash is unchanged.
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config[0].Required = false
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if !instance.Status.Configured || len(instance.Status.MissingRequiredConfiguration) != 0 {
		t.Fatalf("stale missing configuration: %+v", instance.Status)
	}
	vmcp.Spec.Manifest.Components[0].ForceSingleUser = true
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config[0].Required = true
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

func TestInstanceConfigurationStatusIgnoresDisabledComponents(t *testing.T) {
	component := func(id string) types.VMCPComponent {
		return types.VMCPComponent{
			ID:            id,
			Configuration: []types.VMCPConfigurationPolicy{{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed}},
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Config: []types.MCPConfig{{Key: "HEADER", Required: true, Usage: types.Header}},
			}},
		}
	}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{component("enabled"), component("disabled")},
		Profiles: []types.VMCPProfile{{
			Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"enabled": {AllowedTools: []string{}}}},
		}},
	}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "1", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, instance).WithStatusSubresource(instance), instance.Spec.UserID).Build()
	u := &kuser.DefaultInfo{UID: "1"}
	handler := &Handler{
		revealCredential: func(context.Context, []string, string) (gatewaytypes.Credential, error) {
			return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{}
		},
		userInfo: staticUserInfo(u),
	}
	req := router.Request{Ctx: t.Context(), Client: client, Object: instance}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	enabledKey := vmcpconfig.ConfigurationKey("enabled", "HEADER")
	if instance.Status.Configured || !slices.Equal(instance.Status.MissingRequiredConfiguration, []string{enabledKey}) {
		t.Fatalf("missing configuration = %+v", instance.Status)
	}

	// Gaining access to the component through a group must require its configuration.
	vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, types.VMCPProfile{
		Subjects:    []types.Subject{{Type: types.SubjectTypeObotGroup, ID: "team"}},
		Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"disabled": {AllowedTools: []string{"echo"}}}},
	})
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(instance.Status.MissingRequiredConfiguration, []string{enabledKey}) {
		t.Fatalf("profile for another group changed missing configuration: %+v", instance.Status)
	}
	u.Extra = map[string][]string{"obot_groups": {"team"}}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if want := []string{vmcpconfig.ConfigurationKey("disabled", "HEADER"), enabledKey}; !slices.Equal(instance.Status.MissingRequiredConfiguration, want) {
		t.Fatalf("missing configuration = %v, want %v", instance.Status.MissingRequiredConfiguration, want)
	}

	// Deselecting a component's tools does not disable it; only the profiles do.
	instance.Spec.Manifest.ComponentSet = map[string]types.VMCPComponentSet{"enabled": {AllowedTools: []string{}}}
	if err := handler.SyncUserConfigurationHash(req, nil); err != nil {
		t.Fatal(err)
	}
	if want := []string{vmcpconfig.ConfigurationKey("disabled", "HEADER"), enabledKey}; !slices.Equal(instance.Status.MissingRequiredConfiguration, want) {
		t.Fatalf("missing configuration = %v, want %v", instance.Status.MissingRequiredConfiguration, want)
	}
}
