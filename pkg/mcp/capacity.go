package mcp

import (
	"github.com/obot-platform/obot/apiclient/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	mebibyte = 1 << 20
)

// podRequests are the requests of a deployment's live pods.
type podRequests struct {
	memory, cpu resource.Quantity
}

// RunningUsage counts each deployment as one hosted MCP server. Its memory and CPU are the larger of its template's
// requests and its live pods' requests, so new deployments and rollout pods both count.
func RunningUsage(deployments []appsv1.Deployment, pods []corev1.Pod) types.EntitlementsUsage {
	live := make(map[string]podRequests, len(deployments))
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		memory, cpu := podTemplateRequests(pod.Spec)
		requests := live[pod.Labels["app"]]
		requests.memory.Add(memory)
		requests.cpu.Add(cpu)
		live[pod.Labels["app"]] = requests
	}

	var usage types.EntitlementsUsage
	for _, deployment := range deployments {
		var memory, cpu resource.Quantity
		addDeploymentResourceRequests(deployment, &cpu, &memory)
		pods := live[deployment.Name]
		usage.HostedMCPServers++
		memory, cpu = larger(memory, pods.memory), larger(cpu, pods.cpu)
		usage.MemoryMiB += mebibytes(memory)
		usage.CPUMillicores += cpu.MilliValue()
	}
	return usage
}

// startUsage returns what applying the desired deployment adds: a server if it's new, and the requests of any new pod.
func startUsage(desired, existing *appsv1.Deployment) types.EntitlementsUsage {
	memory, cpu := additionalDeploymentRequests(desired, existing)
	usage := types.EntitlementsUsage{
		MemoryMiB:     mebibytes(memory),
		CPUMillicores: cpu.MilliValue(),
	}
	if existing == nil {
		usage.HostedMCPServers = 1
	}
	return usage
}

// planHasCapacity returns whether start fits within the plan's limits. Only limits that start adds to are checked.
func planHasCapacity(limits types.Entitlements, running, start types.EntitlementsUsage) bool {
	fits := func(running, start int64, limit *int64) bool {
		return start <= 0 || limit == nil || running+start <= *limit
	}
	return fits(running.HostedMCPServers, start.HostedMCPServers, limits.HostedMCPServers) &&
		fits(running.MemoryMiB, start.MemoryMiB, limits.MemoryMiB) &&
		fits(running.CPUMillicores, start.CPUMillicores, limits.CPUMillicores)
}

// mebibytes rounds a quantity up to whole MiB.
func mebibytes(q resource.Quantity) int64 {
	return (q.Value() + mebibyte - 1) / mebibyte
}

func larger(a, b resource.Quantity) resource.Quantity {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}

// desiredDeployment returns the deployment in objs, or nil.
func desiredDeployment(objs []kclient.Object) *appsv1.Deployment {
	for _, obj := range objs {
		if deployment, ok := obj.(*appsv1.Deployment); ok {
			return deployment
		}
	}
	return nil
}
