package accesscontrolrule

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestPruneDeletedResourcesKeepsUncachedResources checks that a catalog entry the cache
// hasn't seen yet, such as one created just before the rule, stays in the rule, while an
// entry that really doesn't exist is pruned.
func TestPruneDeletedResourcesKeepsUncachedResources(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name:      "new-entry",
		Namespace: system.DefaultNamespace,
		Spec:      v1.MCPServerCatalogEntrySpec{MCPCatalogName: system.DefaultCatalog},
	}
	acr := &v1.AccessControlRule{
		Name:      "acr1",
		Namespace: system.DefaultNamespace,
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: system.DefaultCatalog,
			Manifest: types.AccessControlRuleManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				Resources: []types.Resource{
					{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name},
					{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "deleted-entry"},
				},
			},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(entry, acr).
		WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if holder, ok := obj.(*untriggered.Holder); ok {
				// An uncached read sees storage.
				return c.Get(ctx, key, holder.Object, opts...)
			}
			if _, ok := obj.(*v1.MCPServerCatalogEntry); ok {
				// The cache hasn't seen any catalog entry yet.
				return apierrors.NewNotFound(schema.GroupResource{Resource: "mcpservercatalogentries"}, key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		}}).
		Build()

	var stored v1.AccessControlRule
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(acr), &stored))
	req := router.Request{Ctx: t.Context(), Client: client, Object: &stored, Namespace: system.DefaultNamespace}
	require.NoError(t, New(nil).PruneDeletedResources(req, nil))

	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(acr), &stored))
	require.Equal(t, []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}}, stored.Spec.Manifest.Resources)
}
