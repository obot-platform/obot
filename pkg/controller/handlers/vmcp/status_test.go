package vmcp

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeCredentials struct {
	reveal func(context.Context, []string, string) (gatewaytypes.Credential, error)
	list   func(context.Context, gateway.ListCredentialsOptions) ([]gatewaytypes.Credential, error)
	delete func(context.Context, string, string) (bool, error)
}

func (f fakeCredentials) RevealCredential(ctx context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
	return f.reveal(ctx, contexts, name)
}

func (f fakeCredentials) ListCredentials(ctx context.Context, opts gateway.ListCredentialsOptions) ([]gatewaytypes.Credential, error) {
	return f.list(ctx, opts)
}

func (f fakeCredentials) DeleteCredential(ctx context.Context, credentialContext, name string) (bool, error) {
	return f.delete(ctx, credentialContext, name)
}

func TestSyncReadiness(t *testing.T) {
	component := types.VMCPComponent{
		ID:            "one",
		Name:          "component",
		Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
			Config:       []types.MCPConfig{{Key: "TOKEN", Required: true, Usage: types.Header}},
		}},
	}
	singleComponent := component
	singleComponent.ID = "two"
	singleComponent.Name = "single-component"
	singleComponent.Configuration = nil
	singleComponent.CatalogEntry.Manifest.Config = nil
	singleComponent.ForceSingleUser = true
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component, singleComponent}}, StaticConfigurationHash: "hash"}}
	vmcp.Status.Components = []v1.VMCPComponentStatus{{Name: component.Name, SourceMissing: true, NeedsUpdate: true}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(vmcp).
		WithObjects(vmcp).WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.VMCPID} }).Build()
	values := map[string]string{}
	oauthConfigured := false
	reveals := 0
	handler := &Handler{credentials: fakeCredentials{
		reveal: func(_ context.Context, _ []string, name string) (gatewaytypes.Credential, error) {
			reveals++
			if name == system.StaticOAuthCredentialName && !oauthConfigured {
				return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{}
			}
			return gatewaytypes.Credential{Secrets: values}, nil
		},
		list: func(context.Context, gateway.ListCredentialsOptions) ([]gatewaytypes.Credential, error) {
			return nil, nil
		},
	}}
	req := router.Request{Ctx: t.Context(), Client: client, Object: vmcp}
	check := func(ready bool, message string) {
		t.Helper()
		if err := handler.SyncStatus(req, nil); err != nil {
			t.Fatal(err)
		}
		if vmcp.Status.Ready != ready || !strings.Contains(vmcp.Status.Components[0].Error, message) {
			t.Fatalf("unexpected status: %+v", vmcp.Status)
		}
		if !vmcp.Status.Components[0].SourceMissing || !vmcp.Status.Components[0].NeedsUpdate {
			t.Fatal("readiness overwrote drift status")
		}
	}
	check(false, "missing required administrator configuration")
	check(false, "missing required administrator configuration")
	if reveals != 1 {
		t.Fatalf("unchanged missing configuration caused %d reveals", reveals)
	}
	values[vmcpconfig.ConfigurationKey(component.ID, "TOKEN")] = "never expose this"
	vmcp.Spec.StaticConfigurationHash = "configured-hash"
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	check(false, "waiting for component server")
	server := &v1.MCPServer{Name: "ms1test", Namespace: "default", Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: component.ID}}
	if err := client.Create(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	check(false, "waiting for component configuration")
	server.Annotations = map[string]string{v1.VMCPSnapshotDigestAnnotation: utils.Digest(component.CatalogEntry)}
	server.Status.VMCPStaticConfigurationHash = vmcp.Spec.StaticConfigurationHash
	server.Status.DeploymentStatus = "Unavailable"
	if err := client.Update(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	check(false, "Unavailable")
	server.Status.DeploymentStatus = "Available"
	if err := client.Update(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	check(true, "")
	version := vmcp.ResourceVersion
	check(true, "")
	if vmcp.ResourceVersion != version {
		t.Fatal("unchanged status was rewritten")
	}
	if reveals != 2 {
		t.Fatalf("deployment updates caused extra reveals: %d", reveals)
	}
	vmcp.Spec.Manifest.Components[0].ForceSingleUser = true
	vmcp.Spec.Manifest.Components[0].Configuration[0].Policy = types.VMCPConfigurationPolicyUserAllowed
	delete(values, vmcpconfig.ConfigurationKey(component.ID, "TOKEN"))
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	check(true, "") // Each user's missing input is instance-scoped.
	vmcp.Spec.Manifest.Components[0].OAuthCredentialID = "oauth-ref"
	vmcp.Spec.Manifest.Components[0].MCPServerCatalogEntryID = "deleted-entry"
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	check(false, "static OAuth credentials")
	previousReveals := reveals
	check(false, "static OAuth credentials")
	if reveals != previousReveals {
		t.Fatal("unchanged missing OAuth credential was revealed again")
	}
	oauthConfigured = true
	entry := &v1.MCPServerCatalogEntry{Name: "deleted-entry", Namespace: vmcp.Namespace, Annotations: map[string]string{v1.OAuthCredentialRevisionAnnotation: "created"}}
	if err := client.Create(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	check(true, "")
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.Runtime = types.RuntimeNPX
	if err := client.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	check(false, "invalid component definition")
}

func TestSyncStatusDeletesSupersededStaticConfiguration(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1rotate", Namespace: "default", Spec: v1.VMCPSpec{
		Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
			ID:           "one",
			Name:         "component",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Runtime: types.RuntimeNPX, NPXConfig: &types.NPXRuntimeConfig{Package: "pkg"}}},
		}}},
		StaticConfigurationHash:           "current",
		StaticConfigurationCredentialName: "configuration-current",
	}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(vmcp).WithObjects(vmcp).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.VMCPID} }).Build()

	now := time.Now()
	credentialContext := vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)
	credentials := []gatewaytypes.Credential{
		{Context: credentialContext, Name: "configuration", CreatedAt: now.Add(-2 * time.Hour)},
		{Context: credentialContext, Name: "configuration-old", CreatedAt: now.Add(-time.Hour)},
		{Context: credentialContext, Name: "configuration-current", CreatedAt: now},
		// Written by an update that has not published its revision yet.
		{Context: credentialContext, Name: "configuration-pending", CreatedAt: now.Add(time.Minute)},
	}
	var deleted []string
	lists := 0
	handler := &Handler{credentials: fakeCredentials{
		reveal: func(context.Context, []string, string) (gatewaytypes.Credential, error) {
			return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{}
		},
		list: func(_ context.Context, opts gateway.ListCredentialsOptions) ([]gatewaytypes.Credential, error) {
			lists++
			if !slices.Equal(opts.CredentialContexts, []string{credentialContext}) {
				t.Fatalf("listed contexts %v", opts.CredentialContexts)
			}
			return credentials, nil
		},
		delete: func(_ context.Context, credentialContext, name string) (bool, error) {
			if credentialContext != vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name) {
				t.Fatalf("deleted from context %q", credentialContext)
			}
			deleted = append(deleted, name)
			return true, nil
		},
	}}

	req := router.Request{Ctx: t.Context(), Client: client, Object: vmcp}
	if err := handler.SyncStatus(req, nil); err != nil {
		t.Fatal(err)
	}
	slices.Sort(deleted)
	if !slices.Equal(deleted, []string{"configuration", "configuration-old"}) {
		t.Fatalf("deleted %v", deleted)
	}

	// Unchanged configuration does not look for superseded revisions again.
	if err := handler.SyncStatus(req, nil); err != nil {
		t.Fatal(err)
	}
	if lists != 1 {
		t.Fatalf("unchanged configuration listed credentials %d times", lists)
	}
}
