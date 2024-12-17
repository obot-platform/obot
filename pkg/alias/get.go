package alias

import (
	"context"
	"errors"
	"fmt"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func Get(ctx context.Context, c kclient.Client, obj v1.Aliasable, namespace string, name string) error {
	var errLookup error
	if namespace == "" {
		gvk, err := c.GroupVersionKindFor(obj.(kclient.Object))
		if err != nil {
			return err
		}
		errLookup = apierrors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, name)
	} else {
		errLookup = c.Get(ctx, router.Key(namespace, name), obj.(kclient.Object))
		if kclient.IgnoreNotFound(errLookup) != nil {
			return errLookup
		} else if errLookup == nil {
			return nil
		}
	}

	gvk, err := c.GroupVersionKindFor(obj.(kclient.Object))
	if err != nil {
		return err
	}

	var alias v1.Alias
	if err := c.Get(ctx, router.Key("", KeyFromScopeID(GetScope(gvk, obj), name)), &alias); apierrors.IsNotFound(err) {
		return errLookup
	} else if err != nil {
		return errors.Join(errLookup, err)
	} else if alias.Spec.TargetKind != gvk.Kind {
		return errLookup
	}

	return c.Get(ctx, router.Key(alias.Spec.TargetNamespace, alias.Spec.TargetName), obj.(kclient.Object))
}

func GetFromScope(ctx context.Context, c kclient.Client, scope, namespace, name string) (kclient.Object, error) {
	gvk := schema.GroupVersionKind{
		Group:   v1.SchemeGroupVersion.Group,
		Version: v1.Version,
		Kind:    scope,
	}

	obj, err := c.Scheme().New(gvk)
	if err != nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, name)
	}

	cObj := obj.(kclient.Object)

	var alias v1.Alias
	if err := c.Get(ctx, router.Key("", KeyFromScopeID(scope, name)), &alias); apierrors.IsNotFound(err) {
		return cObj, c.Get(ctx, router.Key(namespace, name), cObj)
	} else if err != nil {
		return nil, err
	}

	gvk.Kind = alias.Spec.TargetKind
	obj, err = c.Scheme().New(gvk)
	if err != nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{
			Group:    gvk.Group,
			Resource: gvk.Kind,
		}, name)
	}

	cObj = obj.(kclient.Object)

	return cObj, c.Get(ctx, router.Key(alias.Spec.TargetNamespace, alias.Spec.TargetName), cObj)
}

func KeyFromScopeID(scope, id string) string {
	return system.AliasPrefix + hash.String(name.SafeHashConcatName(id, scope))[:8]
}

func GetScope(gvk schema.GroupVersionKind, obj v1.Aliasable) string {
	if scoped, ok := obj.(v1.AliasScoped); ok && scoped.GetAliasScope() != "" {
		return scoped.GetAliasScope()
	}

	return gvk.Kind
}

type GVKLookup interface {
	GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error)
}

type FromGVK schema.GroupVersionKind

func (f FromGVK) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind(f), nil
}

func Name(lookup GVKLookup, obj v1.Aliasable) (string, error) {
	id := obj.GetAliasName()
	if id == "" {
		return "", nil
	}
	runtimeObject, ok := obj.(runtime.Object)
	if !ok {
		return "", fmt.Errorf("object %T does not implement runtime.Object, can not lookup gvk", obj)
	}
	gvk, err := lookup.GroupVersionKindFor(runtimeObject)
	if err != nil {
		return "", err
	}
	return KeyFromScopeID(GetScope(gvk, obj), id), nil
}
