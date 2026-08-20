package hostedagentpool

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentbackend"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestDesiredPoolIsStableAndUsesObotID(t *testing.T) {
	pool := validPool()

	first, err := desiredPool(pool, poolRef(pool))
	if err != nil {
		t.Fatal(err)
	}
	second, err := desiredPool(pool, poolRef(pool))
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision == "" || first.Revision != second.Revision {
		t.Fatalf("revision is not deterministic: %q, %q", first.Revision, second.Revision)
	}
	if first.Ref.ID != "pool-1" {
		t.Fatalf("backend did not receive Obot-managed ID: %q", first.Ref.ID)
	}

	pool.Spec.Manifest.Suspended = true
	changed, err := desiredPool(pool, poolRef(pool))
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision == first.Revision {
		t.Fatal("suspension did not change desired revision")
	}
}

func TestDesiredPoolRejectsZeroCapacity(t *testing.T) {
	pool := validPool()
	pool.Spec.Manifest.Capacity.MemoryBytes = 0
	if _, err := desiredPool(pool, poolRef(pool)); err == nil {
		t.Fatal("expected zero memory capacity to be rejected")
	}
}

func TestApplyObservationRequiresMatchingReadyRevision(t *testing.T) {
	pool := validPool()
	observation := agentbackend.PoolObservation{
		Ref:               agentbackend.PoolRef{ID: "a1", BackendID: "pool-1"},
		Exists:            true,
		ObservedRevision:  "old",
		State:             agentbackend.StateReady,
		Schedulable:       true,
		BackendGeneration: 3,
		Capacity: agentbackend.ResourceQuantity{
			CPUVCPUs: 2, MemoryBytes: 1024, StorageBytes: 2048,
		},
	}
	applyObservation(pool, "wanted", observation)
	if pool.Status.Ready {
		t.Fatal("pool was ready with a stale revision")
	}
	if pool.Status.BackendPoolID != "pool-1" || !pool.Status.Schedulable {
		t.Fatalf("backend observation was not retained: %#v", pool.Status)
	}

	observation.ObservedRevision = "wanted"
	applyObservation(pool, "wanted", observation)
	if !pool.Status.Ready || pool.Status.Degraded {
		t.Fatalf("matching observation was not ready: %#v", pool.Status)
	}

	observation.State = agentbackend.StateError
	observation.Reason = "PoolFailed"
	applyObservation(pool, "wanted", observation)
	if !pool.Status.Degraded || pool.Status.Ready {
		t.Fatalf("error observation was not degraded: %#v", pool.Status)
	}
}

func validPool() *v1.HostedAgentPool {
	return &v1.HostedAgentPool{
		Name: "pool-1",
		Spec: v1.HostedAgentPoolSpec{
			Manifest: types.HostedAgentPoolManifest{
				Capacity: types.HostedAgentResourceQuantity{
					CPUVCPUs: 2, MemoryBytes: 1024, StorageBytes: 2048,
				},
			},
		},
	}
}
