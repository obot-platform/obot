package fake

import (
	"context"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/agentbackend"
)

func TestBackendLifecycleAndOpaqueRevision(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	backend := New(Config{TransitionDelay: time.Second, Now: func() time.Time { return now }})
	ctx := context.Background()
	pool := desiredPool()

	observed, err := backend.ReconcilePool(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if observed.State != agentbackend.StatePending || observed.Ref.BackendID != "fake-pool-a1" {
		t.Fatalf("unexpected pool observation: %#v", observed)
	}
	now = now.Add(time.Second)
	observed, err = backend.ObservePool(ctx, pool.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if observed.State != agentbackend.StateReady || observed.ObservedRevision != "same-key" {
		t.Fatalf("pool did not become ready: %#v", observed)
	}

	instance := desiredInstance()
	instanceObserved, err := backend.ReconcileInstance(ctx, instance)
	if err != nil {
		t.Fatal(err)
	}
	if instanceObserved.State != agentbackend.StatePending || instanceObserved.Ref.BackendID != "fake-instance-i1" {
		t.Fatalf("unexpected instance observation: %#v", instanceObserved)
	}
	now = now.Add(time.Second)
	instanceObserved, err = backend.ObserveInstance(ctx, instance.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if instanceObserved.State != agentbackend.StateReady || instanceObserved.ObservedRevision != "same-key" {
		t.Fatalf("instance did not become ready: %#v", instanceObserved)
	}

	// A revision is opaque correlation metadata: changed content with the same
	// key is accepted and advances the backend generation.
	instance.Name = "changed"
	instanceObserved, err = backend.ReconcileInstance(ctx, instance)
	if err != nil {
		t.Fatal(err)
	}
	if instanceObserved.State != agentbackend.StatePending || instanceObserved.BackendGeneration != 2 {
		t.Fatalf("same-key update was not accepted: %#v", instanceObserved)
	}
}

func TestBackendAsyncDeletion(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	backend := New(Config{TransitionDelay: time.Second, Now: func() time.Time { return now }})
	ctx := context.Background()
	if _, err := backend.ReconcilePool(ctx, desiredPool()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if _, err := backend.ReconcileInstance(ctx, desiredInstance()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)

	deleted, err := backend.DeleteInstance(ctx, agentbackend.InstanceRef{ID: "i1"})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Complete {
		t.Fatal("delete completed before the backend resource was gone")
	}
	now = now.Add(time.Second)
	deleted, err = backend.DeleteInstance(ctx, agentbackend.InstanceRef{ID: "i1"})
	if err != nil || !deleted.Complete {
		t.Fatalf("instance deletion did not complete: %#v, %v", deleted, err)
	}
	deleted, err = backend.DeletePool(ctx, agentbackend.PoolRef{ID: "a1"})
	if err != nil || deleted.Complete {
		t.Fatalf("unexpected pool delete result: %#v, %v", deleted, err)
	}
	now = now.Add(time.Second)
	deleted, err = backend.DeletePool(ctx, agentbackend.PoolRef{ID: "a1"})
	if err != nil || !deleted.Complete {
		t.Fatalf("pool deletion did not complete: %#v, %v", deleted, err)
	}
}

func TestSuspensionAndSyntheticUtilization(t *testing.T) {
	backend := New(Config{})
	ctx := context.Background()
	pool := desiredPool()
	pool.Suspended = true
	if _, err := backend.ReconcilePool(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.ObservePool(ctx, pool.Ref); err != nil {
		t.Fatal(err)
	}
	observed, err := backend.ReconcileInstance(ctx, desiredInstance())
	if err != nil {
		t.Fatal(err)
	}
	if observed.State != agentbackend.StateError || observed.Reason != "PoolSuspended" {
		t.Fatalf("suspended pool accepted a new instance: %#v", observed)
	}

	pool.Suspended = false
	if _, err := backend.ReconcilePool(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.ReconcileInstance(ctx, desiredInstance()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.GetPoolUtilization(ctx, pool.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Instances) != 1 || snapshot.Timestamp.IsZero() ||
		snapshot.Pool.MemoryBytes > pool.Capacity.MemoryBytes {
		t.Fatalf("unexpected utilization: %#v", snapshot)
	}
}

func TestSecretReferencesAreCopiedWithoutValues(t *testing.T) {
	backend := New(Config{})
	ctx := context.Background()
	pool := desiredPool()
	if _, err := backend.ReconcilePool(ctx, pool); err != nil {
		t.Fatal(err)
	}
	instance := desiredInstance()
	instance.Secrets = []agentbackend.SecretRef{{ID: "secret-1", EnvName: "TOKEN", Version: "v1", Value: "super-secret"}}
	if _, err := backend.ReconcileInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}

	// Reconciling the identical desired state again must not look like a change.
	// Storing a redacted copy while comparing against an unredacted argument
	// would make every call differ, restarting the sandbox forever.
	if _, err := backend.ReconcileInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}

	instance.Secrets[0].ID = "mutated"
	backend.mu.Lock()
	defer backend.mu.Unlock()
	stored := backend.instances["i1"]
	if got := stored.desired.Secrets[0].ID; got != "secret-1" {
		t.Fatalf("fake backend retained caller-owned secret slice: %q", got)
	}
	if got := stored.desired.Secrets[0].Value; got != "" {
		t.Fatalf("fake backend stored a secret value: %q", got)
	}
	if got := stored.desired.Secrets[0].Version; got != "v1" {
		t.Fatalf("fake backend dropped the secret version: %q", got)
	}
	if stored.generation != 1 {
		t.Fatalf("identical desired state was treated as a change: generation %d", stored.generation)
	}
}

// A rotated secret still has to restart the sandbox even though the value never
// reaches the backend's store, which is the whole reason SecretRef carries a
// version alongside the value.
func TestRotatedSecretIsSeenAsAChange(t *testing.T) {
	backend := New(Config{})
	ctx := context.Background()
	if _, err := backend.ReconcilePool(ctx, desiredPool()); err != nil {
		t.Fatal(err)
	}
	instance := desiredInstance()
	instance.Secrets = []agentbackend.SecretRef{{ID: "secret-1", EnvName: "TOKEN", Version: "v1", Value: "old"}}
	if _, err := backend.ReconcileInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	instance.Secrets = []agentbackend.SecretRef{{ID: "secret-1", EnvName: "TOKEN", Version: "v2", Value: "new"}}
	if _, err := backend.ReconcileInstance(ctx, instance); err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if got := backend.instances["i1"].generation; got != 2 {
		t.Fatalf("rotated secret did not restart the sandbox: generation %d", got)
	}
}

func desiredPool() agentbackend.DesiredPool {
	return agentbackend.DesiredPool{
		Ref: agentbackend.PoolRef{ID: "a1"}, Revision: "same-key",
		Capacity: agentbackend.ResourceQuantity{CPUVCPUs: 2, MemoryBytes: 1024, StorageBytes: 2048},
	}
}

func desiredInstance() agentbackend.DesiredInstance {
	return agentbackend.DesiredInstance{
		Ref:      agentbackend.InstanceRef{ID: "i1", UserID: "u1"},
		Pool:     agentbackend.PoolRef{ID: "a1"},
		Revision: "same-key", Name: "agent",
	}
}
