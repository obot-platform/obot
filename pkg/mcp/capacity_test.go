package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestPlanCapacity(t *testing.T) {
	deployments := types.MCPCapacityInfo{
		Source:            types.CapacitySourceResourceQuota,
		CPURequested:      "250m",
		CPULimit:          "8",
		MemoryRequested:   "512Mi",
		MemoryLimit:       "16Gi",
		ActiveDeployments: 2,
	}

	for _, tt := range []struct {
		name  string
		plan  types.Entitlements
		usage *types.EntitlementsUsage
		want  types.MCPCapacityInfo
	}{
		{
			name: "the plan's limits and billing's usage replace the cluster's",
			plan: types.Entitlements{
				CPUMillicores: new(int64(2000)),
				MemoryMiB:     new(int64(8192)),
			},
			usage: &types.EntitlementsUsage{
				CPUMillicores: 1500,
				MemoryMiB:     600,
			},
			want: types.MCPCapacityInfo{
				Source:            types.CapacitySourcePlan,
				CPURequested:      "1.5",
				CPULimit:          "2",
				MemoryRequested:   "600Mi",
				MemoryLimit:       "8Gi",
				ActiveDeployments: 2,
			},
		},
		{
			name: "an unlimited plan has no limits",
			usage: &types.EntitlementsUsage{
				CPUMillicores: 30,
				MemoryMiB:     200,
			},
			want: types.MCPCapacityInfo{
				Source:            types.CapacitySourcePlan,
				CPURequested:      "30m",
				MemoryRequested:   "200Mi",
				ActiveDeployments: 2,
			},
		},
		{
			name: "a limit of 0 is shown",
			plan: types.Entitlements{
				CPUMillicores: new(int64(0)),
				MemoryMiB:     new(int64(0)),
			},
			usage: &types.EntitlementsUsage{},
			want: types.MCPCapacityInfo{
				Source:            types.CapacitySourcePlan,
				CPULimit:          "0",
				MemoryLimit:       "0",
				ActiveDeployments: 2,
			},
		},
		{
			name: "without usage the deployments' requests stay",
			plan: types.Entitlements{
				CPUMillicores: new(int64(1000)),
				MemoryMiB:     new(int64(4096)),
			},
			want: types.MCPCapacityInfo{
				Source:            types.CapacitySourcePlan,
				CPURequested:      "250m",
				CPULimit:          "1",
				MemoryRequested:   "512Mi",
				MemoryLimit:       "4Gi",
				ActiveDeployments: 2,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := PlanCapacity(deployments, tt.plan, tt.usage); got != tt.want {
				t.Errorf("PlanCapacity() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
