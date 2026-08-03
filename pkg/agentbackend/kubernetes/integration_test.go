// These tests run against a real cluster. They skip unless
// OBOT_AGENT_INTEGRATION_KUBECONFIG names one:
//
//	OBOT_AGENT_INTEGRATION_KUBECONFIG=~/.kube/config \
//	  go test ./pkg/agentbackend/kubernetes/ -run TestIntegration -v
//
// A dedicated variable rather than KUBECONFIG is deliberate: these tests create
// and delete a namespace, and must never fire against whatever cluster a
// developer happens to have configured. They are not build-tagged so that they
// keep compiling as part of the normal test run.
//
// They cover what a unit test cannot: whether apply handles a cluster-scoped
// PriorityClass alongside namespaced objects, whether a Namespace owner
// reference is accepted, and whether a quota scoped to a PriorityClass really
// rejects an over-budget sandbox.
package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/agentbackend"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace = "obot-agent-integration"
	// pause stays running with no arguments, which the contract needs because
	// DesiredInstance carries an image but no command.
	pauseImage = "registry.k8s.io/pause:3.9"
)

func testClient(t *testing.T) kclient.Client {
	t.Helper()

	kubeconfig := os.Getenv("OBOT_AGENT_INTEGRATION_KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("set OBOT_AGENT_INTEGRATION_KUBECONFIG to a kubeconfig to run agent backend integration tests")
	}
	if strings.HasPrefix(kubeconfig, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("locate home directory: %v", err)
		}
		kubeconfig = filepath.Join(home, kubeconfig[2:])
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("build rest config from %s: %v", kubeconfig, err)
	}
	client, err := kclient.New(config, kclient.Options{Scheme: k8sscheme.Scheme})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	return client
}

func setup(t *testing.T, opts Options) (*Backend, kclient.Client, context.Context) {
	t.Helper()

	client := testClient(t)
	ctx := t.Context()

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
	if err := client.Create(ctx, namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	opts.Namespace = testNamespace
	// The live client doubles as the cache here; informer behaviour is a
	// separate concern from the object semantics these tests cover.
	backend, err := New(client, client, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return backend, client, ctx
}

func eventually(t *testing.T, timeout time.Duration, describe string, condition func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		ok, detail := condition()
		if ok {
			return
		}
		last = detail
		time.Sleep(time.Second)
	}
	t.Fatalf("timed out waiting for %s: %s", describe, last)
}

func smallPool(id string) agentbackend.DesiredPool {
	return agentbackend.DesiredPool{
		Ref:      agentbackend.PoolRef{ID: id},
		Revision: "rev-1",
		Capacity: agentbackend.ResourceQuantity{
			CPUVCPUs:     2,
			MemoryBytes:  2 << 30,
			StorageBytes: 1 << 30,
		},
		MaxSandboxes: 4,
	}
}

func TestIntegrationPoolLifecycle(t *testing.T) {
	backend, client, ctx := setup(t, Options{})
	desired := smallPool("alloc-lifecycle")
	t.Cleanup(func() {
		_, _ = backend.DeletePool(context.Background(), desired.Ref)
	})

	observation, err := backend.ReconcilePool(ctx, desired)
	if err != nil {
		t.Fatalf("ReconcilePool: %v", err)
	}
	if !observation.Exists {
		t.Fatal("expected the pool to exist after reconcile")
	}
	if observation.ObservedRevision != desired.Revision {
		t.Errorf("observed revision = %q, want %q", observation.ObservedRevision, desired.Revision)
	}
	if !observation.Schedulable {
		t.Errorf("expected a fresh pool to be schedulable, got %s: %s", observation.Reason, observation.Message)
	}

	// A cluster-scoped PriorityClass owned by a Namespace is the load-bearing
	// assumption behind sharing one namespace across pools. The API server
	// rejects an invalid owner reference outright.
	var priorityClass schedulingv1.PriorityClass
	if err := client.Get(ctx, kclient.ObjectKey{Name: poolName(desired.Ref.ID)}, &priorityClass); err != nil {
		t.Fatalf("read PriorityClass: %v", err)
	}
	if len(priorityClass.OwnerReferences) != 1 || priorityClass.OwnerReferences[0].Kind != "Namespace" {
		t.Errorf("expected a Namespace owner reference, got %+v", priorityClass.OwnerReferences)
	}
	if priorityClass.PreemptionPolicy == nil || *priorityClass.PreemptionPolicy != corev1.PreemptNever {
		t.Error("pool priority classes must never preempt")
	}

	var quota corev1.ResourceQuota
	if err := client.Get(ctx, kclient.ObjectKey{Name: poolName(desired.Ref.ID), Namespace: testNamespace}, &quota); err != nil {
		t.Fatalf("read ResourceQuota: %v", err)
	}
	if quota.Spec.ScopeSelector == nil || len(quota.Spec.ScopeSelector.MatchExpressions) != 1 {
		t.Fatalf("expected a PriorityClass scope selector, got %+v", quota.Spec.ScopeSelector)
	}

	// Reconciling the same revision must not churn the objects.
	generation := quota.Generation
	if _, err := backend.ReconcilePool(ctx, desired); err != nil {
		t.Fatalf("second ReconcilePool: %v", err)
	}
	if err := client.Get(ctx, kclient.ObjectKey{Name: poolName(desired.Ref.ID), Namespace: testNamespace}, &quota); err != nil {
		t.Fatalf("re-read ResourceQuota: %v", err)
	}
	if quota.Generation != generation {
		t.Errorf("reconcile is not idempotent: generation moved %d -> %d", generation, quota.Generation)
	}

	result, err := backend.DeletePool(ctx, desired.Ref)
	if err != nil {
		t.Fatalf("DeletePool: %v", err)
	}
	if !result.Complete {
		t.Error("expected pool deletion to complete synchronously")
	}
	if err := client.Get(ctx, kclient.ObjectKey{Name: poolName(desired.Ref.ID)}, &priorityClass); !apierrors.IsNotFound(err) {
		t.Errorf("expected the PriorityClass to be pruned, got %v", err)
	}
}

func TestIntegrationInstanceReachesReady(t *testing.T) {
	backend, client, ctx := setup(t, Options{})
	pool := smallPool("alloc-ready")
	instance := agentbackend.DesiredInstance{
		Ref:      agentbackend.InstanceRef{ID: "inst-ready", UserID: "user-1"},
		Pool:     pool.Ref,
		Revision: "rev-1",
		Image:    pauseImage,
		Files: []agentbackend.File{
			{Path: "/etc/obot/agent.json", Content: []byte(`{"version":"v1"}`), Mode: 0o644},
		},
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for range 60 {
			result, err := backend.DeleteInstance(cleanupCtx, instance.Ref)
			if err != nil || result.Complete {
				break
			}
			time.Sleep(time.Second)
		}
		_, _ = backend.DeletePool(cleanupCtx, pool.Ref)
	})

	if _, err := backend.ReconcilePool(ctx, pool); err != nil {
		t.Fatalf("ReconcilePool: %v", err)
	}
	if _, err := backend.ReconcileInstance(ctx, instance); err != nil {
		t.Fatalf("ReconcileInstance: %v", err)
	}

	eventually(t, 3*time.Minute, "the sandbox to become ready", func() (bool, string) {
		observation, err := backend.ObserveInstance(ctx, instance.Ref)
		if err != nil {
			return false, err.Error()
		}
		if observation.State == agentbackend.StateReady {
			if observation.ObservedRevision != instance.Revision {
				t.Errorf("ready with revision %q, want %q", observation.ObservedRevision, instance.Revision)
			}
			if observation.URL == "" {
				t.Error("a ready sandbox must advertise a URL")
			}
			return true, ""
		}
		return false, string(observation.State) + " " + observation.Reason + ": " + observation.Message
	})

	// The sandbox has to actually be tagged into its pool, or it escapes quota.
	var deployment appsv1.Deployment
	if err := client.Get(ctx, kclient.ObjectKey{Name: instanceName(instance.Ref.ID), Namespace: testNamespace}, &deployment); err != nil {
		t.Fatalf("read Deployment: %v", err)
	}
	if got := deployment.Spec.Template.Spec.PriorityClassName; got != poolName(pool.Ref.ID) {
		t.Errorf("priorityClassName = %q, want %q", got, poolName(pool.Ref.ID))
	}

	// Deleting is asynchronous: the workspace directory is erased by a job, and
	// the finalizer must be held until that finishes.
	result, err := backend.DeleteInstance(ctx, instance.Ref)
	if err != nil {
		t.Fatalf("DeleteInstance: %v", err)
	}
	if result.Complete {
		t.Error("expected deletion to be incomplete while the cleanup job runs")
	}

	eventually(t, 3*time.Minute, "the cleanup job to finish", func() (bool, string) {
		result, err := backend.DeleteInstance(ctx, instance.Ref)
		if err != nil {
			return false, err.Error()
		}
		return result.Complete, "cleanup still running"
	})

	if err := client.Get(ctx, kclient.ObjectKey{Name: instanceName(instance.Ref.ID), Namespace: testNamespace}, &deployment); !apierrors.IsNotFound(err) {
		t.Errorf("expected the Deployment to be pruned, got %v", err)
	}
}

// A pool bounds how many sandboxes it holds. Sandboxes have no size of their
// own, so nothing can be individually "too big" -- what the quota stops is the
// pool being filled past its sandbox count. The rejection surfaces through
// observation rather than the write, because it lands on the ReplicaSet.
func TestIntegrationQuotaRejectsSandboxBeyondPoolCount(t *testing.T) {
	backend, _, ctx := setup(t, Options{})
	pool := agentbackend.DesiredPool{
		Ref:      agentbackend.PoolRef{ID: "pool-full"},
		Revision: "rev-1",
		Capacity: agentbackend.ResourceQuantity{
			CPUVCPUs:     1,
			MemoryBytes:  1 << 30,
			StorageBytes: 1 << 30,
		},
		MaxSandboxes: 1,
	}
	first := agentbackend.DesiredInstance{
		Ref:      agentbackend.InstanceRef{ID: "inst-first", UserID: "user-1"},
		Pool:     pool.Ref,
		Revision: "rev-1",
		Image:    pauseImage,
	}
	second := first
	second.Ref = agentbackend.InstanceRef{ID: "inst-second", UserID: "user-1"}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, ref := range []agentbackend.InstanceRef{first.Ref, second.Ref} {
			for range 30 {
				result, err := backend.DeleteInstance(cleanupCtx, ref)
				if err != nil || result.Complete {
					break
				}
				time.Sleep(time.Second)
			}
		}
		_, _ = backend.DeletePool(cleanupCtx, pool.Ref)
	})

	if _, err := backend.ReconcilePool(ctx, pool); err != nil {
		t.Fatalf("ReconcilePool: %v", err)
	}

	// Both sandboxes are sized identically from the pool; only the count differs.
	requests, limits, _ := agentbackend.SandboxShare(pool.Capacity, pool.MaxSandboxes)
	first.Requests, first.Limits = requests, limits
	second.Requests, second.Limits = requests, limits

	if _, err := backend.ReconcileInstance(ctx, first); err != nil {
		t.Fatalf("ReconcileInstance(first): %v", err)
	}
	eventually(t, 2*time.Minute, "the first sandbox to become ready", func() (bool, string) {
		observation, err := backend.ObserveInstance(ctx, first.Ref)
		if err != nil {
			return false, err.Error()
		}
		return observation.State == agentbackend.StateReady, string(observation.State) + " " + observation.Reason
	})

	if _, err := backend.ReconcileInstance(ctx, second); err != nil {
		t.Fatalf("ReconcileInstance(second): %v", err)
	}
	eventually(t, 2*time.Minute, "the pool to refuse a sandbox beyond its count", func() (bool, string) {
		observation, err := backend.ObserveInstance(ctx, second.Ref)
		if err != nil {
			return false, err.Error()
		}
		if observation.State == agentbackend.StateError {
			t.Logf("pool full surfaced as %s: %s", observation.Reason, observation.Message)
			return true, ""
		}
		return false, string(observation.State) + " " + observation.Reason
	})
}

// Suspending must stop new sandboxes without disturbing the pool's objects.
func TestIntegrationSuspendedPoolRefusesSandboxes(t *testing.T) {
	backend, _, ctx := setup(t, Options{})
	pool := smallPool("alloc-suspended")
	pool.Suspended = true
	t.Cleanup(func() {
		_, _ = backend.DeletePool(context.Background(), pool.Ref)
	})

	observation, err := backend.ReconcilePool(ctx, pool)
	if err != nil {
		t.Fatalf("ReconcilePool: %v", err)
	}
	if observation.Schedulable {
		t.Fatal("a suspended pool must not be schedulable")
	}

	instanceObservation, err := backend.ReconcileInstance(ctx, agentbackend.DesiredInstance{
		Ref:      agentbackend.InstanceRef{ID: "inst-suspended"},
		Pool:     pool.Ref,
		Revision: "rev-1",
		Image:    pauseImage,
	})
	if err != nil {
		t.Fatalf("ReconcileInstance: %v", err)
	}
	if instanceObservation.State != agentbackend.StateError || instanceObservation.Reason != "PoolSuspended" {
		t.Fatalf("expected PoolSuspended, got %s/%s", instanceObservation.State, instanceObservation.Reason)
	}
}
