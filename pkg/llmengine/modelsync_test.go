package llmengine

import (
	"context"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestTriggerModelSyncRetriesConflicts checks that a conflicting write, as when the provider
// controller updates the ModelProvider at the same time, is retried instead of returned.
func TestTriggerModelSyncRetriesConflicts(t *testing.T) {
	mp := ModelProviderResource()
	conflicts := 2
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(mp).
		WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if conflicts > 0 {
				conflicts--
				return apierrors.NewConflict(schema.GroupResource{Resource: "modelproviders"}, obj.GetName(), nil)
			}
			return c.Update(ctx, obj, opts...)
		}}).
		Build()

	if err := TriggerModelSync(t.Context(), client); err != nil {
		t.Fatalf("TriggerModelSync: %v", err)
	}
	var got v1.ModelProvider
	if err := client.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.LLMGatewayModelProvider}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[v1.ModelProviderSyncAnnotation] != "true" {
		t.Fatalf("sync annotation not set: %v", got.Annotations)
	}
	if conflicts != 0 {
		t.Fatalf("expected both conflicts to be retried, %d left", conflicts)
	}
}
