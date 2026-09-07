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
