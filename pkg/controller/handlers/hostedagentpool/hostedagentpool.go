// Package hostedagentpool reconciles hosted-agent capacity pools
// through the implementation-neutral agent backend contract.
package hostedagentpool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/agentbackend"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const pollInterval = 10 * time.Second

type Handler struct {
	backend agentbackend.PoolBackend
}

func New(backend agentbackend.PoolBackend) *Handler {
	return &Handler{backend: backend}
}

// Orchestrate converges one pool with its backend pool.
func (h *Handler) Orchestrate(req router.Request, resp router.Response) error {
	pool := req.Object.(*v1.HostedAgentPool)
	if h.backend == nil {
		return fmt.Errorf("hosted agent backend is not configured")
	}

	ref := poolRef(pool)
	desired, err := desiredPool(pool, ref)
	if err != nil {
		return err
	}
	observation, err := h.backend.ReconcilePool(req.Ctx, desired)
	if err != nil {
		return err
	}
	previous := pool.Status
	applyObservation(pool, desired.Revision, observation)
	resp.RetryAfter(pollInterval)

	// Writing an unchanged status still emits a change event, which reconciles
	// this pool again immediately and defeats RetryAfter. The status has
	// no heartbeat field, so an exact comparison is enough.
	if pool.Status == previous {
		return nil
	}
	return req.Client.Status().Update(req.Ctx, pool)
}

// Remove asks the backend to remove a pool. The router retains the
// finalizer while this handler requests a retry.
func (h *Handler) Remove(req router.Request, resp router.Response) error {
	pool := req.Object.(*v1.HostedAgentPool)
	if h.backend == nil {
		return fmt.Errorf("hosted agent backend is not configured")
	}
	ref := poolRef(pool)
	result, err := h.backend.DeletePool(req.Ctx, ref)
	if err != nil {
		return err
	}
	if !result.Complete {
		resp.RetryAfter(pollInterval)
		return nil
	}
	return nil
}

func desiredPool(pool *v1.HostedAgentPool, ref agentbackend.PoolRef) (agentbackend.DesiredPool, error) {
	if err := pool.Spec.Manifest.Validate(); err != nil {
		return agentbackend.DesiredPool{}, fmt.Errorf("invalid hosted agent pool: %w", err)
	}
	desired := agentbackend.DesiredPool{
		Ref:          ref,
		MaxSandboxes: pool.Spec.Manifest.MaxSandboxes,
		Capacity: agentbackend.ResourceQuantity{
			CPUVCPUs:     pool.Spec.Manifest.Capacity.CPUVCPUs,
			MemoryBytes:  pool.Spec.Manifest.Capacity.MemoryBytes,
			StorageBytes: pool.Spec.Manifest.Capacity.StorageBytes,
		},
		Suspended: pool.Spec.Manifest.Suspended,
	}
	revisionValue := struct {
		Capacity  agentbackend.ResourceQuantity `json:"capacity"`
		Suspended bool                          `json:"suspended"`
	}{
		Capacity:  desired.Capacity,
		Suspended: desired.Suspended,
	}
	data, err := json.Marshal(revisionValue)
	if err != nil {
		return agentbackend.DesiredPool{}, fmt.Errorf("marshal pool revision input: %w", err)
	}
	sum := sha256.Sum256(data)
	desired.Revision = hex.EncodeToString(sum[:])
	return desired, nil
}

func poolRef(pool *v1.HostedAgentPool) agentbackend.PoolRef {
	return agentbackend.PoolRef{
		// Pool assignments and HostedAgentInstance.PoolID refer to
		// the pool resource ID (its storage name), so the backend's
		// managed pool identity must use that same stable value.
		ID:        pool.Name,
		BackendID: pool.Status.BackendPoolID,
	}
}

func applyObservation(pool *v1.HostedAgentPool, desiredRevision string, observation agentbackend.PoolObservation) {
	if observation.Ref.BackendID != "" {
		pool.Status.BackendPoolID = observation.Ref.BackendID
	}
	pool.Status.ObservedRevision = observation.ObservedRevision
	pool.Status.Schedulable = observation.Schedulable
	pool.Status.Reason = observation.Reason
	pool.Status.Message = observation.Message
	pool.Status.Capacity.CPUVCPUs = observation.Capacity.CPUVCPUs
	pool.Status.Capacity.MemoryBytes = observation.Capacity.MemoryBytes
	pool.Status.Capacity.StorageBytes = observation.Capacity.StorageBytes
	pool.Status.Ready = observation.Exists &&
		observation.State == agentbackend.StateReady &&
		observation.ObservedRevision == desiredRevision
	pool.Status.Degraded = observation.State == agentbackend.StateError
}
