package mcp

import (
	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PlanCapacity shows info against a billing plan: the plan's memory and CPU limits replace the cluster's, and when usage
// is set, billing's count of what hosted MCP servers request replaces what the deployments request. An unlimited plan
// shows no limit.
func PlanCapacity(info types.MCPCapacityInfo, plan types.Entitlements, usage *types.EntitlementsUsage) types.MCPCapacityInfo {
	info.Source = types.CapacitySourcePlan
	info.CPULimit = planLimit(plan.CPUMillicores, cpuMillicores)
	info.MemoryLimit = planLimit(plan.MemoryMiB, memoryMiB)
	if usage != nil {
		info.CPURequested = cpuMillicores(usage.CPUMillicores)
		info.MemoryRequested = memoryMiB(usage.MemoryMiB)
	}
	return info
}

// planLimit formats a plan's limit, or nothing when it's unlimited. A limit of 0 still shows, since nothing fits.
func planLimit(limit *int64, format func(int64) string) string {
	switch {
	case limit == nil:
		return ""
	case *limit == 0:
		return "0"
	default:
		return format(*limit)
	}
}

// cpuMillicores and memoryMiB format billing's units the way the cluster's quantities are formatted.
func cpuMillicores(n int64) string {
	return formatCPU(*resource.NewMilliQuantity(n, resource.DecimalSI))
}

func memoryMiB(n int64) string {
	return formatMemory(*resource.NewQuantity(n*mebibyte, resource.BinarySI))
}
