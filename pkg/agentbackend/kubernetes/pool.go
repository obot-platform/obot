package kubernetes

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/obot/pkg/agentbackend"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *Backend) ReconcilePool(ctx context.Context, desired agentbackend.DesiredPool) (agentbackend.PoolObservation, error) {
	if desired.Ref.ID == "" {
		return agentbackend.PoolObservation{}, fmt.Errorf("pool ID is required")
	}
	if desired.Revision == "" {
		return agentbackend.PoolObservation{}, fmt.Errorf("pool revision is required")
	}

	objs, err := b.poolObjects(ctx, desired)
	if err != nil {
		return agentbackend.PoolObservation{}, err
	}

	if err := b.applyPool(ctx, desired.Ref.ID, objs...); err != nil {
		return agentbackend.PoolObservation{}, err
	}

	return b.ObservePool(ctx, desired.Ref)
}

func (b *Backend) ObservePool(ctx context.Context, ref agentbackend.PoolRef) (agentbackend.PoolObservation, error) {
	if ref.ID == "" {
		return agentbackend.PoolObservation{}, fmt.Errorf("pool ID is required")
	}

	observation := agentbackend.PoolObservation{Ref: ref}

	var quota corev1.ResourceQuota
	if err := b.cachedClient.Get(ctx, kclient.ObjectKey{Name: poolName(ref.ID), Namespace: b.opts.Namespace}, &quota); apierrors.IsNotFound(err) {
		return observation, nil
	} else if err != nil {
		return agentbackend.PoolObservation{}, fmt.Errorf("failed to read pool quota for pool %s: %w", ref.ID, err)
	}

	observation.Ref.BackendID = poolName(ref.ID)
	observation.Exists = true
	observation.ObservedRevision = quota.Annotations[revisionAnnotation]
	observation.BackendGeneration = quota.Generation
	observation.Capacity = capacityFromQuota(quota)

	// The pool volume is provisioned lazily: with WaitForFirstConsumer it stays
	// Pending until the first sandbox schedules, so a Pending claim is expected
	// and is not a degraded pool.
	var claim corev1.PersistentVolumeClaim
	if err := b.cachedClient.Get(ctx, kclient.ObjectKey{Name: poolName(ref.ID), Namespace: b.opts.Namespace}, &claim); err != nil {
		if !apierrors.IsNotFound(err) {
			return agentbackend.PoolObservation{}, fmt.Errorf("failed to read pool volume for pool %s: %w", ref.ID, err)
		}
		observation.State = agentbackend.StatePending
		observation.Reason = "VolumeMissing"
		observation.Message = "the pool volume has not been created yet"
		return observation, nil
	}
	if claim.Status.Phase == corev1.ClaimLost {
		observation.State = agentbackend.StateError
		observation.Reason = "VolumeLost"
		observation.Message = "the pool volume is no longer bound to a persistent volume"
		return observation, nil
	}

	observation.State = agentbackend.StateReady
	observation.Schedulable = !isSuspended(quota)
	if !observation.Schedulable {
		observation.Reason = "Suspended"
		observation.Message = "the pool does not admit new sandboxes"
	}
	return observation, nil
}

func (b *Backend) DeletePool(ctx context.Context, ref agentbackend.PoolRef) (agentbackend.DeleteResult, error) {
	if ref.ID == "" {
		return agentbackend.DeleteResult{}, fmt.Errorf("pool ID is required")
	}

	// Refuse while sandboxes remain, so a pool volume is never removed out from
	// under a running agent.
	var deployments appsv1.DeploymentList
	if err := b.cachedClient.List(ctx, &deployments, kclient.InNamespace(b.opts.Namespace), kclient.MatchingLabels{
		poolLabel: sanitize(ref.ID),
	}); err != nil {
		return agentbackend.DeleteResult{}, fmt.Errorf("failed to list sandboxes for pool %s: %w", ref.ID, err)
	}
	if len(deployments.Items) > 0 {
		return agentbackend.DeleteResult{}, fmt.Errorf("pool %s still has %d sandbox(es)", ref.ID, len(deployments.Items))
	}

	if err := b.applyPool(ctx, ref.ID); err != nil {
		return agentbackend.DeleteResult{}, err
	}
	return agentbackend.DeleteResult{Complete: true}, nil
}

// applyPool reconciles the pool's object set. Passing no objects prunes
// the set, which is how deletion works.
func (b *Backend) applyPool(ctx context.Context, poolID string, objs ...kclient.Object) error {
	err := apply.New(b.client).
		WithNamespace(b.opts.Namespace).
		WithOwnerSubContext("agent-pool-"+sanitize(poolID)).
		WithPruneTypes(
			new(schedulingv1.PriorityClass),
			new(corev1.ResourceQuota),
			new(corev1.PersistentVolumeClaim),
		).
		Apply(ctx, nil, objs...)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to apply pool objects for pool %s: %w", poolID, err)
	}
	return nil
}

func (b *Backend) poolObjects(ctx context.Context, desired agentbackend.DesiredPool) ([]kclient.Object, error) {
	pool := poolName(desired.Ref.ID)
	labels := poolLabels(desired.Ref.ID)
	annotations := map[string]string{revisionAnnotation: desired.Revision}

	owner, err := b.namespaceOwner(ctx)
	if err != nil {
		return nil, err
	}

	priorityClass := &schedulingv1.PriorityClass{
		Name:        pool,
		Labels:      labels,
		Annotations: annotations,
		// Cluster-scoped dependents may only name cluster-scoped owners, and
		// a Namespace is cluster-scoped, so this is a valid reference.
		OwnerReferences:  []metav1.OwnerReference{*owner},
		Value:            poolPriorityValue,
		GlobalDefault:    false,
		PreemptionPolicy: new(corev1.PreemptNever),
		Description:      fmt.Sprintf("Obot hosted agent pool %s. Used to scope a ResourceQuota, not to order scheduling.", desired.Ref.ID),
	}

	quota := &corev1.ResourceQuota{
		Name:        pool,
		Namespace:   b.opts.Namespace,
		Labels:      labels,
		Annotations: annotations,
		Spec: corev1.ResourceQuotaSpec{
			Hard: quotaHard(desired),
			// PriorityClass is the only per-pod attribute a ResourceQuota can
			// select on, which is what lets pools share a namespace.
			ScopeSelector: &corev1.ScopeSelector{
				MatchExpressions: []corev1.ScopedResourceSelectorRequirement{{
					ScopeName: corev1.ResourceQuotaScopePriorityClass,
					Operator:  corev1.ScopeSelectorOpIn,
					Values:    []string{pool},
				}},
			},
		},
	}

	claimAnnotations := map[string]string{revisionAnnotation: desired.Revision}
	// A bound claim is largely immutable, and resizing it is not something a
	// revision bump should attempt.
	claimAnnotations[apply.AnnotationUpdate] = "false"
	claim := &corev1.PersistentVolumeClaim{
		Name:        pool,
		Namespace:   b.opts.Namespace,
		Labels:      labels,
		Annotations: claimAnnotations,
		Spec: corev1.PersistentVolumeClaimSpec{
			// ReadWriteOnce is node-scoped, not pod-scoped: every sandbox in the
			// pool mounts this claim from the same node. That co-location is the
			// point, and it is what makes the pool a physical boundary.
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: memoryQuantity(desired.Capacity.StorageBytes),
				},
			},
		},
	}
	if b.opts.StorageClassName != "" {
		claim.Spec.StorageClassName = new(b.opts.StorageClassName)
	}

	return []kclient.Object{priorityClass, quota, claim}, nil
}

// quotaHard expresses the pool budget.
//
// Only requests are bounded. Every sandbox may burst to the whole pool, so
// sum(limits) is MaxSandboxes times the capacity by construction; a limits
// quota would therefore either reject the second sandbox or be set so high it
// enforces nothing. What bounds the pool is the reservation budget plus the
// sandbox count.
//
// A suspended pool zeroes the budget, which stops new sandboxes being admitted.
// Quota is an admission check, so pods already running keep running.
func quotaHard(desired agentbackend.DesiredPool) corev1.ResourceList {
	if desired.Suspended {
		zero := *resource.NewQuantity(0, resource.DecimalSI)
		return corev1.ResourceList{
			corev1.ResourceRequestsCPU:    zero,
			corev1.ResourceRequestsMemory: zero,
			corev1.ResourcePods:           zero,
		}
	}

	_, _, effectiveMax := agentbackend.SandboxShare(desired.Capacity, desired.MaxSandboxes)
	return corev1.ResourceList{
		corev1.ResourceRequestsCPU:    cpuQuantity(desired.Capacity.CPUVCPUs),
		corev1.ResourceRequestsMemory: memoryQuantity(desired.Capacity.MemoryBytes),
		corev1.ResourcePods:           *resource.NewQuantity(int64(effectiveMax), resource.DecimalSI),
	}
}

func isSuspended(quota corev1.ResourceQuota) bool {
	value, ok := quota.Spec.Hard[corev1.ResourceRequestsCPU]
	return ok && value.IsZero()
}

func capacityFromQuota(quota corev1.ResourceQuota) agentbackend.ResourceQuantity {
	cpu := quota.Spec.Hard[corev1.ResourceRequestsCPU]
	memory := quota.Spec.Hard[corev1.ResourceRequestsMemory]
	return agentbackend.ResourceQuantity{
		CPUVCPUs:    float64(cpu.MilliValue()) / 1000,
		MemoryBytes: memory.Value(),
	}
}
