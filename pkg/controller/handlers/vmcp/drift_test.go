package vmcp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestDetectDrift(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name: "entry", Namespace: "default",
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{Name: "original", Runtime: types.RuntimeRemote},
		},
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1test",
		Namespace: "default",
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{
			{
				ID:                      "one",
				Name:                    "one",
				MCPServerCatalogEntryID: entry.Name,
				CatalogEntry:            types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest},
				SourceDigest:            vmcpconfig.SourceDigest(types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}),
			},
			{
				ID:                      "two",
				Name:                    "two",
				MCPServerCatalogEntryID: "missing",
			},
		}}},
		Status: v1.VMCPStatus{
			Ready: true,
			Components: []v1.VMCPComponentStatus{
				{Name: "one", Ready: true, Error: "keep this error"},
				{Name: "removed", NeedsUpdate: true},
			},
		},
	}
	originalSpec := vmcp.DeepCopy().Spec
	failLookup := false
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, entry).
		WithStatusSubresource(vmcp).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", vmcpInstanceVMCPID).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
				if failLookup && key.Name == "missing" {
					return errors.New("lookup failed")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()
	reconcile := func() {
		t.Helper()
		if err := DetectDrift(router.Request{Ctx: t.Context(), Client: client, Object: vmcp}, nil); err != nil {
			t.Fatal(err)
		}
		if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), vmcp); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(vmcp.Spec, originalSpec) {
			t.Fatal("drift detection changed the snapshot")
		}
		if !vmcp.Status.Ready || !vmcp.Status.Components[0].Ready || vmcp.Status.Components[0].Error != "keep this error" {
			t.Fatal("drift detection changed readiness/error status")
		}
	}
	reconcile()
	if len(vmcp.Status.Components) != 2 || vmcp.Status.Components[0].NeedsUpdate || !vmcp.Status.Components[1].SourceMissing {
		t.Fatalf("unexpected initial status: %#v", vmcp.Status)
	}
	version := vmcp.ResourceVersion
	reconcile()
	if vmcp.ResourceVersion != version {
		t.Fatal("unchanged status was written again")
	}

	entry.Spec.Manifest.ToolPreview = []types.MCPServerTool{{Name: "echo"}}
	if err := client.Update(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	reconcile()
	if vmcp.Status.Components[0].NeedsUpdate {
		t.Fatal("tool previews should not cause snapshot drift")
	}

	entry.Spec.Manifest.Metadata = map[string]string{"categories": "Developer Tools"}
	entry.Spec.Manifest.ShortDescription = "new short description"
	entry.Spec.Manifest.Description = "new description"
	entry.Spec.Manifest.Icon = "https://example.com/icon.png"
	entry.Spec.Manifest.UpgradeNote = "Read before upgrading."
	if err := client.Update(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	reconcile()
	if vmcp.Status.Components[0].NeedsUpdate {
		t.Fatal("informational catalog fields should not cause snapshot drift")
	}

	entry.Spec.Manifest.Name = "changed"
	entry.Spec.UnsupportedTools = []string{"newly-unsupported"}
	if err := client.Update(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	reconcile()
	if !vmcp.Status.Components[0].NeedsUpdate {
		t.Fatal("source changes did not mark the component as needing an update")
	}

	// An error on a later source must not persist partially computed statuses.
	before := vmcp.DeepCopy().Status
	failLookup = true
	if err := DetectDrift(router.Request{Ctx: t.Context(), Client: client, Object: vmcp}, nil); err == nil {
		t.Fatal("expected lookup error")
	}
	if !reflect.DeepEqual(before, vmcp.Status) {
		t.Fatal("lookup error changed status")
	}
	failLookup = false

	if err := client.Delete(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	reconcile()
	if !vmcp.Status.Components[0].SourceMissing || vmcp.Status.Components[0].NeedsUpdate {
		t.Fatal("missing source did not clear the update flag")
	}

	entry.ResourceVersion = ""
	entry.Spec.Manifest = originalSpec.Manifest.Components[0].CatalogEntry.Manifest
	entry.Spec.UnsupportedTools = nil
	if err := client.Create(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	reconcile()
	if vmcp.Status.Components[0].SourceMissing || vmcp.Status.Components[0].NeedsUpdate {
		t.Fatal("restored matching source did not clear drift")
	}
}

func vmcpInstanceVMCPID(object kclient.Object) []string {
	return []string{object.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
}

func TestDetectDriftReportsOutdatedConnections(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry",
		Namespace: "default",
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:      "Server",
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "current"},
				Config: []types.MCPConfig{
					{
						Key:      "TOKEN",
						Required: true,
					},
				},
			},
		},
	}
	snapshot := types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}
	component := types.VMCPComponent{
		ID:                      "server",
		Name:                    "server",
		MCPServerCatalogEntryID: entry.Name,
		CatalogEntry:            snapshot,
		SourceDigest:            vmcpconfig.SourceDigest(snapshot),
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1migrated",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			LegacySlug: entry.Name,
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{component},
			},
		},
	}
	retained := func(name, pkg string) *v1.VMCPInstance {
		legacy := *component.DeepCopy()
		legacy.SourceDigest = utils.Digest([]any{component, ""})
		legacy.CatalogEntry.Manifest.NPXConfig = &types.NPXRuntimeConfig{Package: pkg}
		legacy.CatalogEntry.Manifest.Config[0].Value = "secret"
		return &v1.VMCPInstance{
			Name:       name,
			Namespace:  vmcp.Namespace,
			Finalizers: []string{v1.VMCPInstanceFinalizer},
			Spec: v1.VMCPInstanceSpec{
				LegacySlug:       name,
				Manifest:         types.VMCPInstanceManifest{VMCPID: vmcp.Name},
				LegacyComponents: []types.VMCPComponent{legacy},
			},
		}
	}
	current, outdated := retained("current", "current"), retained("outdated", "older")
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, entry, current, outdated).
		WithStatusSubresource(vmcp).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", vmcpInstanceVMCPID).
		Build()
	reconcile := func() v1.VMCPComponentStatus {
		t.Helper()
		require.NoError(t, DetectDrift(router.Request{Ctx: t.Context(), Client: client, Object: vmcp}, nil))
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), vmcp))
		require.Len(t, vmcp.Status.Components, 1)
		return vmcp.Status.Components[0]
	}

	status := reconcile()
	require.True(t, status.NeedsUpdate, "a connection retaining an older snapshot can be updated")
	require.NotNil(t, status.ConnectionSnapshot)
	require.Equal(t, "older", status.ConnectionSnapshot.Manifest.NPXConfig.Package)
	require.Empty(t, status.ConnectionSnapshot.Manifest.Config[0].Value, "connection configuration is not exposed")

	version := vmcp.ResourceVersion
	reconcile()
	require.Equal(t, version, vmcp.ResourceVersion, "unchanged status was written again")

	// A deleting connection no longer needs an update.
	require.NoError(t, client.Delete(t.Context(), outdated))
	status = reconcile()
	require.False(t, status.NeedsUpdate)
	require.Nil(t, status.ConnectionSnapshot)

	// Changing the component releases retained snapshots, so only the component's drift remains.
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(outdated), outdated))
	outdated.Finalizers = nil
	require.NoError(t, client.Update(t.Context(), outdated))
	outdated = retained("outdated", "older")
	require.NoError(t, client.Create(t.Context(), outdated))
	require.True(t, reconcile().NeedsUpdate)
	vmcp.Spec.Manifest.Components[0].ToolPrefix = "changed_"
	require.NoError(t, client.Update(t.Context(), vmcp))
	status = reconcile()
	require.False(t, status.NeedsUpdate)
	require.Nil(t, status.ConnectionSnapshot)

	// An outdated component reports its own drift rather than a connection's snapshot.
	entry.Spec.Manifest.NPXConfig = &types.NPXRuntimeConfig{Package: "newest"}
	require.NoError(t, client.Update(t.Context(), entry))
	status = reconcile()
	require.True(t, status.NeedsUpdate)
	require.Nil(t, status.ConnectionSnapshot)
}
