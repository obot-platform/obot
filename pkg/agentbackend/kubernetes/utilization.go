package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/obot-platform/obot/pkg/agentbackend"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// GetPoolUtilization reports live resource consumption for a pool.
//
// Usage is measured per sandbox through metrics.k8s.io. Where measurement is
// unavailable -- no metrics-server, or a pod too new to have been sampled --
// that sandbox falls back to its committed request, so a pool always reports a
// usable number rather than understating itself to zero. Falling back to the
// request rather than to zero is deliberate: a sandbox that exists has
// certainly reserved its request, so that is the floor of what it is using.
func (b *Backend) GetPoolUtilization(ctx context.Context, ref agentbackend.PoolRef) (agentbackend.UtilizationSnapshot, error) {
	if ref.ID == "" {
		return agentbackend.UtilizationSnapshot{}, fmt.Errorf("pool ID is required")
	}

	var quota corev1.ResourceQuota
	if err := b.cachedClient.Get(ctx, kclient.ObjectKey{Name: poolName(ref.ID), Namespace: b.opts.Namespace}, &quota); apierrors.IsNotFound(err) {
		return agentbackend.UtilizationSnapshot{}, fmt.Errorf("pool %s does not exist", ref.ID)
	} else if err != nil {
		return agentbackend.UtilizationSnapshot{}, fmt.Errorf("failed to read pool quota for pool %s: %w", ref.ID, err)
	}

	var deployments appsv1.DeploymentList
	if err := b.cachedClient.List(ctx, &deployments, kclient.InNamespace(b.opts.Namespace), kclient.MatchingLabels{
		poolLabel: sanitize(ref.ID),
	}); err != nil {
		return agentbackend.UtilizationSnapshot{}, fmt.Errorf("failed to list sandboxes for pool %s: %w", ref.ID, err)
	}

	// Measured usage is keyed by pod, so the pods of this pool are needed to
	// join it back to instances.
	var pods corev1.PodList
	if err := b.cachedClient.List(ctx, &pods, kclient.InNamespace(b.opts.Namespace), kclient.MatchingLabels{
		poolLabel: sanitize(ref.ID),
	}); err != nil {
		return agentbackend.UtilizationSnapshot{}, fmt.Errorf("failed to list pods for pool %s: %w", ref.ID, err)
	}

	var measured map[string]agentbackend.ResourceUtilization
	if b.usage != nil {
		var err error
		measured, err = b.usage.PodUsage(ctx, b.opts.Namespace, poolLabel+"="+sanitize(ref.ID))
		if err != nil {
			return agentbackend.UtilizationSnapshot{}, fmt.Errorf("failed to read pool usage for %s: %w", ref.ID, err)
		}
	}

	// instanceID -> measured usage, summed over that instance's running pods.
	byInstance := make(map[string]agentbackend.ResourceUtilization, len(pods.Items))
	for _, pod := range pods.Items {
		id := pod.Labels[instanceLabel]
		if id == "" || !pod.DeletionTimestamp.IsZero() {
			continue
		}
		if usage, ok := measured[pod.Name]; ok {
			current := byInstance[id]
			current.CPUVCPUs += usage.CPUVCPUs
			current.MemoryBytes += usage.MemoryBytes
			byInstance[id] = current
		}
	}

	snapshot := agentbackend.UtilizationSnapshot{Timestamp: time.Now()}
	for _, deployment := range deployments.Items {
		instanceID := deployment.Labels[instanceLabel]
		if instanceID == "" || !deployment.DeletionTimestamp.IsZero() {
			continue
		}

		usage, ok := byInstance[instanceID]
		if !ok {
			usage = requestsOf(&deployment)
		}

		snapshot.Instances = append(snapshot.Instances, agentbackend.InstanceUtilization{
			Ref: agentbackend.InstanceRef{
				ID:        instanceID,
				Namespace: deployment.Namespace,
				UserID:    deployment.Labels[userLabel],
				BackendID: deployment.Name,
			},
			State:       stateOf(&deployment),
			Utilization: usage,
		})
		snapshot.Pool.CPUVCPUs += usage.CPUVCPUs
		snapshot.Pool.MemoryBytes += usage.MemoryBytes
	}
	// Storage is reported for the pool only. Sandboxes share one volume and are
	// separated by subPath, which kubelet does not measure individually, so
	// per-instance disk needs a node agent running du. Pool-level usage is left
	// at zero rather than guessed when it cannot be attributed to the pool.
	if b.usage != nil {
		var claim corev1.PersistentVolumeClaim
		if err := b.cachedClient.Get(ctx, kclient.ObjectKey{Name: poolName(ref.ID), Namespace: b.opts.Namespace}, &claim); err != nil && !apierrors.IsNotFound(err) {
			return agentbackend.UtilizationSnapshot{}, fmt.Errorf("failed to read pool volume for %s: %w", ref.ID, err)
		}
		if node := poolNode(pods.Items); node != "" {
			used, trusted, err := b.usage.PoolVolumeUsage(ctx, node, b.opts.Namespace, poolName(ref.ID), claimCapacityBytes(&claim))
			if err != nil {
				return agentbackend.UtilizationSnapshot{}, fmt.Errorf("failed to read pool volume usage for %s: %w", ref.ID, err)
			}
			if trusted {
				snapshot.Pool.StorageBytes = used
				snapshot.StorageMeasured = true
			}
		}
	}

	sort.Slice(snapshot.Instances, func(i, j int) bool {
		return snapshot.Instances[i].Ref.ID < snapshot.Instances[j].Ref.ID
	})
	return snapshot, nil
}

// requestsOf reports what a sandbox has actually reserved from the pool.
//
// Requests rather than limits, because a pool's Capacity is the requests
// budget: the quota's limits budget is that capacity multiplied by the
// overcommit ratio. Reporting limits against a requests-denominated capacity
// makes any pool with sandboxes in it read as fully consumed, which is what the
// admin UI showed before this was corrected.
func requestsOf(deployment *appsv1.Deployment) agentbackend.ResourceUtilization {
	var usage agentbackend.ResourceUtilization
	replicas := int64(1)
	if deployment.Spec.Replicas != nil {
		replicas = int64(*deployment.Spec.Replicas)
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
			usage.CPUVCPUs += float64(cpu.MilliValue()) / 1000 * float64(replicas)
		}
		if memory, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
			usage.MemoryBytes += memory.Value() * replicas
		}
	}
	return usage
}

func stateOf(deployment *appsv1.Deployment) agentbackend.State {
	if !deployment.DeletionTimestamp.IsZero() {
		return agentbackend.StateDeleting
	}
	if deployment.Status.ReadyReplicas > 0 {
		return agentbackend.StateReady
	}
	return agentbackend.StatePending
}

// poolNode returns the node a pool's sandboxes run on. A pool is confined to
// one node by its ReadWriteOnce volume, so any running pod identifies it.
func poolNode(pods []corev1.Pod) string {
	for i := range pods {
		if pods[i].Spec.NodeName != "" && pods[i].DeletionTimestamp.IsZero() {
			return pods[i].Spec.NodeName
		}
	}
	return ""
}

// claimCapacityBytes prefers the bound capacity over the request, since a
// provisioner may round up and the bound value is what actually exists.
func claimCapacityBytes(claim *corev1.PersistentVolumeClaim) int64 {
	if capacity, ok := claim.Status.Capacity[corev1.ResourceStorage]; ok {
		return capacity.Value()
	}
	if request, ok := claim.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		return request.Value()
	}
	return 0
}
