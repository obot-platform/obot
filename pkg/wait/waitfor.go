package wait

import (
	"context"
	"fmt"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Option struct {
	Timeout       time.Duration
	Create        bool
	WaitForExists bool
}

func complete(opts ...Option) (result Option) {
	for _, opt := range opts {
		result.Timeout = types.FirstSet(result.Timeout, opt.Timeout)
		result.Create = types.FirstSet(result.Create, opt.Create)
		result.WaitForExists = types.FirstSet(result.WaitForExists, opt.WaitForExists)
	}
	if result.Timeout == 0 {
		result.Timeout = 2 * time.Minute
	}
	return
}

func load(ctx context.Context, c kclient.Client, obj kclient.Object, create bool) error {
	if obj.GetUID() != "" {
		return nil
	}

	if obj.GetName() != "" {
		err := c.Get(ctx, kclient.ObjectKeyFromObject(obj), obj)
		if err == nil {
			return nil
		} else if err := kclient.IgnoreNotFound(err); err != nil {
			return err
		}

		if !create {
			return err
		}
	}

	err := c.Create(ctx, obj)
	if !apierrors.IsAlreadyExists(err) {
		// If the object already exists, we can retrieve it
		return err
	}

	return c.Get(ctx, kclient.ObjectKeyFromObject(obj), obj)
}

func For[T kclient.Object](ctx context.Context, c kclient.WithWatch, obj T, condition func(T) (bool, error), opts ...Option) (def T, _ error) {
	opt := complete(opts...)

	obj = obj.DeepCopyObject().(T)

	gvk, err := c.GroupVersionKindFor(obj)
	if err != nil {
		return def, err
	}

	list, err := c.Scheme().New(schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind + "List",
	})
	if err != nil {
		return def, err
	}

	if err = load(ctx, c, obj, opt.Create); err != nil {
		if !apierrors.IsNotFound(err) || !opt.WaitForExists {
			return def, err
		}
	}

	if obj.GetName() != "" {
		if ok, err := condition(obj); ok || err != nil {
			return obj, err
		}
	}

	timeout := int64(opt.Timeout / time.Second)

	w, err := c.Watch(ctx, list.(kclient.ObjectList),
		kclient.InNamespace(obj.GetNamespace()),
		kclient.MatchingFields{"metadata.name": obj.GetName()},
		&kclient.ListOptions{
			Raw: &metav1.ListOptions{
				TimeoutSeconds:  &timeout,
				ResourceVersion: obj.GetResourceVersion(),
			},
		})
	if err != nil {
		return def, err
	}
	defer func() {
		w.Stop()
		//nolint:revive
		for range w.ResultChan() {
		}
	}()

	for event := range w.ResultChan() {
		if event.Type == watch.Deleted {
			gvk, _ := c.GroupVersionKindFor(obj)
			return def, apierrors.NewNotFound(schema.GroupResource{
				Group:    gvk.Group,
				Resource: gvk.Kind,
			}, obj.GetName())
		}
		switch event.Type {
		case watch.Added, watch.Modified:
			if ok, err := condition(event.Object.(T)); err != nil {
				return def, err
			} else if ok {
				return event.Object.(T), nil
			}
		case watch.Error:
			return def, apierrors.FromObject(event.Object)
		}
	}

	return def, fmt.Errorf("timeout waiting for %s %s to meet condition", gvk.Kind, obj.GetName())
}
