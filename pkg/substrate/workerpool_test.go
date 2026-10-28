package substrate

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/pkg/clauderunner"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnvironmentWorkerPoolLifecycle(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "ate.dev", Version: "v1alpha1", Kind: "WorkerPool"}, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "ate.dev", Version: "v1alpha1", Kind: "WorkerPoolList"}, &unstructured.UnstructuredList{})
	kube := fake.NewClientBuilder().WithScheme(scheme).Build()
	toleration := corev1.Toleration{Key: "dedicated", Value: "agents", Effect: corev1.TaintEffectNoSchedule}
	c := Client{Kube: kube, Config: Config{
		Namespace:          "workers",
		WorkerNodeSelector: map[string]string{"dedicated": "agents"},
		WorkerTolerations:  []corev1.Toleration{toleration},
		WorkerImage:        "worker:test",
	}}
	env := gt.ClaudeEnvironment{ID: "environment"}
	clauderunner.DefaultRuntime(&env.Runtime)
	ctx := t.Context()
	require.NoError(t, c.ReconcileWorkerPools(ctx, []gt.ClaudeEnvironment{env}, nil, mutateWorkerPool))
	pool := &unstructured.Unstructured{}
	pool.SetAPIVersion("ate.dev/v1alpha1")
	pool.SetKind("WorkerPool")
	key := kclient.ObjectKey{Namespace: "workers", Name: env.ID}
	require.NoError(t, kube.Get(ctx, key, pool))
	require.Equal(t, env.ID, pool.GetLabels()[environmentLabel])
	replicas, _, err := unstructured.NestedInt64(pool.Object, "spec", "replicas")
	require.NoError(t, err)
	require.EqualValues(t, 1, replicas)
	workerImage, _, err := unstructured.NestedString(pool.Object, "spec", "workerImage")
	require.NoError(t, err)
	require.Equal(t, "worker:test", workerImage, "an environment without a worker image uses Obot's default")
	resources, _, err := unstructured.NestedMap(pool.Object, "spec", "template", "resources")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"cpu": "2", "memory": "4Gi"}, resources["limits"])
	require.Equal(t, resources["limits"], resources["requests"])
	selector, _, err := unstructured.NestedStringMap(pool.Object, "spec", "template", "nodeSelector")
	require.NoError(t, err)
	require.Equal(t, c.Config.WorkerNodeSelector, selector)
	tolerations, _, err := unstructured.NestedSlice(pool.Object, "spec", "template", "tolerations")
	require.NoError(t, err)
	require.Equal(t, []any{map[string]any{"key": "dedicated", "value": "agents", "effect": "NoSchedule"}}, tolerations)

	// Repair a pool whose requests differ from its limits on the next reconciliation.
	require.NoError(t, unstructured.SetNestedMap(pool.Object, map[string]any{"cpu": "500m", "memory": "1Gi"}, "spec", "template", "resources", "requests"))
	require.NoError(t, kube.Update(ctx, pool))
	env.Runtime.Worker.WorkerReplicas = 2
	env.Runtime.Worker.CPULimit = "1"
	env.Runtime.Worker.MemoryLimit = "2Gi"
	require.NoError(t, c.ReconcileWorkerPools(ctx, []gt.ClaudeEnvironment{env}, nil, mutateWorkerPool))
	require.NoError(t, kube.Get(ctx, key, pool))
	request, _, err := unstructured.NestedString(pool.Object, "spec", "template", "resources", "requests", "cpu")
	require.NoError(t, err)
	require.Equal(t, "1", request)
	resources, _, err = unstructured.NestedMap(pool.Object, "spec", "template", "resources")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"cpu": "1", "memory": "2Gi"}, resources["limits"])
	require.Equal(t, resources["limits"], resources["requests"])

	// Global scheduling updates replace old selectors and revoke removed tolerations.
	c.Config.WorkerNodeSelector = map[string]string{"pool": "new"}
	c.Config.WorkerTolerations = nil
	require.NoError(t, c.ReconcileWorkerPools(ctx, []gt.ClaudeEnvironment{env}, nil, mutateWorkerPool))
	require.NoError(t, kube.Get(ctx, key, pool))
	selector, _, err = unstructured.NestedStringMap(pool.Object, "spec", "template", "nodeSelector")
	require.NoError(t, err)
	require.Equal(t, c.Config.WorkerNodeSelector, selector)
	tolerations, _, err = unstructured.NestedSlice(pool.Object, "spec", "template", "tolerations")
	require.NoError(t, err)
	require.Empty(t, tolerations)

	// Configuration additions apply to existing environments without an environment update.
	c.Config.WorkerTolerations = []corev1.Toleration{toleration}
	require.NoError(t, c.ReconcileWorkerPools(ctx, []gt.ClaudeEnvironment{env}, nil, mutateWorkerPool))
	require.NoError(t, kube.Get(ctx, key, pool))
	tolerations, _, err = unstructured.NestedSlice(pool.Object, "spec", "template", "tolerations")
	require.NoError(t, err)
	require.Equal(t, []any{map[string]any{"key": "dedicated", "value": "agents", "effect": "NoSchedule"}}, tolerations)

	// A new default image reaches environments that don't set one; a set image stays.
	c.Config.WorkerImage = "worker:new"
	require.NoError(t, c.ReconcileWorkerPools(ctx, []gt.ClaudeEnvironment{env}, nil, mutateWorkerPool))
	require.NoError(t, kube.Get(ctx, key, pool))
	workerImage, _, err = unstructured.NestedString(pool.Object, "spec", "workerImage")
	require.NoError(t, err)
	require.Equal(t, "worker:new", workerImage)
	env.Runtime.WorkerImage = "worker:pinned"
	c.Config.WorkerImage = "worker:newer"
	require.NoError(t, c.ReconcileWorkerPools(ctx, []gt.ClaudeEnvironment{env}, nil, mutateWorkerPool))
	require.NoError(t, kube.Get(ctx, key, pool))
	workerImage, _, err = unstructured.NestedString(pool.Object, "spec", "workerImage")
	require.NoError(t, err)
	require.Equal(t, "worker:pinned", workerImage)

	require.NoError(t, c.ReconcileWorkerPools(ctx, nil, map[string]bool{env.ID: true}, mutateWorkerPool))
	require.NoError(t, kube.Get(ctx, key, pool), "retain pool while its sessions are being finalized")
	require.NoError(t, c.ReconcileWorkerPools(ctx, nil, nil, mutateWorkerPool))
	require.True(t, apierrors.IsNotFound(kube.Get(ctx, key, pool)))
}

func mutateWorkerPool(ctx context.Context, _ string, _ *gt.ClaudeEnvironment, apply func(context.Context) error) error {
	return apply(ctx)
}
