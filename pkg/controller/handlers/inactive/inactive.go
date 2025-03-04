package inactive

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/backend"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func RemoveFromCache(ctx context.Context, backend backend.Backend, obj kclient.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" {
		var err error
		gvk, err = backend.GVKForObject(obj, backend.Scheme())
		if err != nil {
			return fmt.Errorf("failed to get GVK for object: %w", err)
		}
	}

	informer, err := backend.GetInformerForKind(ctx, gvk)
	if err != nil {
		return fmt.Errorf("failed to get informer for kind %s: %w", gvk.String(), err)
	}

	return informer.GetStore().Delete(obj)
}
