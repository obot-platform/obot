package handlers

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/agentbackend"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestConvertPoolUtilization(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	got := convertPoolUtilization(agentbackend.UtilizationSnapshot{
		Timestamp: now,
		Pool:      agentbackend.ResourceUtilization{CPUVCPUs: 1.5, MemoryBytes: 2, StorageBytes: 3},
		Instances: []agentbackend.InstanceUtilization{{
			Ref:         agentbackend.InstanceRef{ID: "instance-1", BackendID: "backend-1"},
			State:       agentbackend.StateReady,
			Utilization: agentbackend.ResourceUtilization{CPUVCPUs: 0.5, MemoryBytes: 1, StorageBytes: 2},
		}},
		Pressure: agentbackend.Pressure{Memory: true},
	})

	if !got.Timestamp.Equal(now) {
		t.Fatalf("Timestamp = %v, want %v", got.Timestamp, now)
	}
	if got.Pool.CPUVCPUs != 1.5 || got.Pool.MemoryBytes != 2 || got.Pool.StorageBytes != 3 {
		t.Fatalf("Pool = %#v", got.Pool)
	}
	if len(got.Instances) != 1 || got.Instances[0].InstanceID != "instance-1" || got.Instances[0].State != agentbackend.StateReady {
		t.Fatalf("Instances = %#v", got.Instances)
	}
	if !got.Pressure.Memory || got.Pressure.CPU || got.Pressure.Storage {
		t.Fatalf("Pressure = %#v", got.Pressure)
	}
}

func TestConvertHostedAgentInstanceIncludesPool(t *testing.T) {
	got := convertHostedAgentInstance(v1.HostedAgentInstance{
		Name: "instance-1",
		Spec: v1.HostedAgentInstanceSpec{
			UserID:          "user-1",
			HostedAgentName: "agent-1",
			PoolID:          "pool-1",
		},
	})

	if got.PoolID != "pool-1" {
		t.Fatalf("PoolID = %q, want pool-1", got.PoolID)
	}
}

// A pool is shared across users, so its utilization describes
// co-tenants' sandboxes too. Pool totals are reported separately and stay
// whole; the per-instance breakdown must not expose anyone else's instances.
func TestNarrowInstanceUtilizationHidesOtherUsersInstances(t *testing.T) {
	mine := v1.HostedAgentInstance{
		Name: "hai-mine", UID: "uid-mine",
	}
	usages := []agentbackend.InstanceUtilization{
		{Ref: agentbackend.InstanceRef{ID: "uid-theirs-1"}, Utilization: agentbackend.ResourceUtilization{CPUVCPUs: 1}},
		{Ref: agentbackend.InstanceRef{ID: "uid-mine"}, Utilization: agentbackend.ResourceUtilization{CPUVCPUs: 2}},
		{Ref: agentbackend.InstanceRef{ID: "uid-theirs-2"}, Utilization: agentbackend.ResourceUtilization{CPUVCPUs: 3}},
	}

	got := narrowInstanceUtilization(usages, []v1.HostedAgentInstance{mine})

	if len(got) != 1 {
		t.Fatalf("expected only the caller's instance, got %d entries: %+v", len(got), got)
	}
	// Rewritten from the backend UID to the API resource ID.
	if got[0].Ref.ID != "hai-mine" {
		t.Errorf("instance ID = %q, want the API resource ID hai-mine", got[0].Ref.ID)
	}
	if got[0].Utilization.CPUVCPUs != 2 {
		t.Errorf("kept the wrong entry: cpu = %v, want 2", got[0].Utilization.CPUVCPUs)
	}
}

// An admin passes the full instance list, so nothing is dropped.
func TestNarrowInstanceUtilizationKeepsEverythingVisible(t *testing.T) {
	instances := []v1.HostedAgentInstance{
		{Name: "hai-a", UID: "uid-a"},
		{Name: "hai-b", UID: "uid-b"},
	}
	usages := []agentbackend.InstanceUtilization{
		{Ref: agentbackend.InstanceRef{ID: "uid-a"}},
		{Ref: agentbackend.InstanceRef{ID: "uid-b"}},
	}

	got := narrowInstanceUtilization(usages, instances)

	if len(got) != 2 {
		t.Fatalf("expected both instances, got %d", len(got))
	}
}

// A sandbox the backend reports but Obot has no record of, such as one left
// behind by a failed delete, must not surface with a raw backend identifier.
func TestNarrowInstanceUtilizationDropsUnknownSandboxes(t *testing.T) {
	usages := []agentbackend.InstanceUtilization{
		{Ref: agentbackend.InstanceRef{ID: "uid-orphan"}},
	}

	if got := narrowInstanceUtilization(usages, nil); len(got) != 0 {
		t.Fatalf("expected orphaned sandboxes to be dropped, got %+v", got)
	}
}
