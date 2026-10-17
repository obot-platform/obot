package mcpwebhookvalidation

import (
	"context"
	"reflect"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// newCleanupTestClient builds a client that, like the controller's, reads untriggered
// uncached gets from storage. Cached gets of the types in stale miss, as if the cache hadn't
// seen those objects yet.
func newCleanupTestClient(builder *fake.ClientBuilder, stale map[string]bool) kclient.WithWatch {
	return builder.WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
		if holder, ok := obj.(*untriggered.Holder); ok {
			return c.Get(ctx, key, holder.Object, opts...)
		}
		if stale[key.Name] {
			return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
		}
		return c.Get(ctx, key, obj, opts...)
	}}).Build()
}

func TestCleanupResourcesPreservesVMCP(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1existing", Namespace: system.DefaultNamespace}
	server := &v1.MCPServer{Name: "ms1existing", Namespace: system.DefaultNamespace}
	want := []types.Resource{
		{Type: types.ResourceTypeMCPServer, ID: vmcp.Name},
		{Type: types.ResourceTypeMCPServer, ID: server.Name},
	}
	validation := &v1.MCPWebhookValidation{}
	validation.Name = "mwv1test"
	validation.Namespace = system.DefaultNamespace
	validation.Spec.Manifest.Resources = append(want, types.Resource{
		Type: types.ResourceTypeMCPServer,
		ID:   "vmcp1deleted",
	})
	client := newCleanupTestClient(fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, server, validation), nil)
	if err := (&Handler{}).CleanupResources(router.Request{
		Ctx:       t.Context(),
		Client:    client,
		Object:    validation,
		Namespace: system.DefaultNamespace,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(validation.Spec.Manifest.Resources, want) {
		t.Fatalf("resources = %#v, want %#v", validation.Spec.Manifest.Resources, want)
	}
}

// TestCleanupResourcesKeepsUncachedResources checks that a target the cache hasn't seen yet,
// such as a catalog entry created just before the filter, stays on the filter.
func TestCleanupResourcesKeepsUncachedResources(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{Name: "new-entry", Namespace: system.DefaultNamespace}
	want := []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}}
	validation := &v1.MCPWebhookValidation{}
	validation.Name = "mwv1new"
	validation.Namespace = system.DefaultNamespace
	validation.Spec.Manifest.Resources = append(want, types.Resource{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "deleted-entry"})
	client := newCleanupTestClient(fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(entry, validation), map[string]bool{entry.Name: true})
	if err := (&Handler{}).CleanupResources(router.Request{
		Ctx:       t.Context(),
		Client:    client,
		Object:    validation,
		Namespace: system.DefaultNamespace,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(validation.Spec.Manifest.Resources, want) {
		t.Fatalf("resources = %#v, want %#v", validation.Spec.Manifest.Resources, want)
	}
}
