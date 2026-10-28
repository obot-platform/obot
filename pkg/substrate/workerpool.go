package substrate

import (
	"context"
	"fmt"
	"reflect"

	"github.com/obot-platform/obot/pkg/clauderunner"
	"github.com/obot-platform/obot/pkg/gateway/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	environmentLabel = "obot.ai/claude-environment"
)

// Worker pools belong to environments; each environment has an independent
// capacity budget, shared by all its matching profiles and sessions.
func (c *Client) ReconcileWorkerPools(ctx context.Context, environments []types.ClaudeEnvironment, inUse map[string]bool, mutate func(context.Context, string, *types.ClaudeEnvironment, func(context.Context) error) error) error {
	if c.Kube == nil {
		return fmt.Errorf("kubernetes client is required for Claude worker pools")
	}
	desired := map[string]bool{}
	for _, env := range environments {
		desired[env.ID] = true
		w := env.Runtime.Worker
		workerImage := clauderunner.WithDefaultImages(env.Runtime, c.Config.WorkerImage, c.Config.RunnerImage).WorkerImage
		nodeSelector := map[string]any{}
		for key, value := range c.Config.WorkerNodeSelector {
			nodeSelector[key] = value
		}

		tolerations := []any{}
		for _, toleration := range c.Config.WorkerTolerations {
			value, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&toleration)
			if err != nil {
				return err
			}
			tolerations = append(tolerations, value)
		}

		pool := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "ate.dev/v1alpha1", "kind": "WorkerPool",
			"metadata": map[string]any{"name": env.ID, "namespace": c.Config.Namespace, "labels": map[string]any{environmentLabel: env.ID}},
			"spec": map[string]any{
				"replicas": int64(w.WorkerReplicas), "sandboxClass": "gvisor", "workerImage": workerImage,
				"template": map[string]any{"nodeSelector": nodeSelector, "tolerations": tolerations, "resources": map[string]any{
					"requests": map[string]any{"cpu": w.CPULimit, "memory": w.MemoryLimit},
					"limits":   map[string]any{"cpu": w.CPULimit, "memory": w.MemoryLimit},
				}},
			},
		}}
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(pool.GroupVersionKind())
		err := c.Kube.Get(ctx, ktypes.NamespacedName{Namespace: c.Config.Namespace, Name: env.ID}, current)
		if apierrors.IsNotFound(err) {
			if err := mutate(ctx, env.ID, &env, func(ctx context.Context) error { return c.Kube.Create(ctx, pool) }); err != nil && !apierrors.IsAlreadyExists(err) {
				return err
			}
		} else if err != nil {
			return err
		} else if current.GetLabels()[environmentLabel] != env.ID {
			return fmt.Errorf("worker pool %s is not managed by this environment", env.ID)
		} else {
			// Patch only the fields we own, preserving server defaults and metadata.
			before := current.DeepCopy()
			for _, path := range [][]string{{"spec", "replicas"}, {"spec", "sandboxClass"}, {"spec", "workerImage"}, {"spec", "template", "resources"}, {"spec", "template", "nodeSelector"}, {"spec", "template", "tolerations"}} {
				value, _, err := unstructured.NestedFieldCopy(pool.Object, path...)
				if err != nil {
					return err
				}
				if err := unstructured.SetNestedField(current.Object, value, path...); err != nil {
					return err
				}
			}
			if !reflect.DeepEqual(before.Object, current.Object) {
				if err := mutate(ctx, env.ID, &env, func(ctx context.Context) error {
					return c.Kube.Patch(ctx, current, kclient.MergeFromWithOptions(before, kclient.MergeFromWithOptimisticLock{}))
				}); err != nil {
					return err
				}
			}
		}
	}
	pools := &unstructured.UnstructuredList{}
	pools.SetAPIVersion("ate.dev/v1alpha1")
	pools.SetKind("WorkerPoolList")
	if err := c.Kube.List(ctx, pools, kclient.InNamespace(c.Config.Namespace), kclient.HasLabels{environmentLabel}); err != nil {
		return err
	}
	for i := range pools.Items {
		pool := &pools.Items[i]
		id := pool.GetLabels()[environmentLabel]
		if !desired[id] && !inUse[id] {
			if err := mutate(ctx, id, nil, func(ctx context.Context) error {
				return c.Kube.Delete(ctx, pool, kclient.Preconditions{UID: new(pool.GetUID()), ResourceVersion: new(pool.GetResourceVersion())})
			}); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}
