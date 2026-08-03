package agentbackend

import "testing"

// A pool is a shared bucket: every sandbox reserves an equal fraction and may
// burst to the whole pool. Agents have no size of their own.
func TestSandboxShareDividesThePool(t *testing.T) {
	capacity := ResourceQuantity{CPUVCPUs: 4, MemoryBytes: 16 << 30}

	requests, limits, effectiveMax := SandboxShare(capacity, 8)

	if requests.CPUVCPUs != 0.5 {
		t.Errorf("request cpu = %v, want 0.5", requests.CPUVCPUs)
	}
	if requests.MemoryBytes != 2<<30 {
		t.Errorf("request memory = %d, want %d", requests.MemoryBytes, int64(2)<<30)
	}
	// The ceiling is the whole pool, which is what makes it shared rather than
	// a set of fixed-size slots.
	if limits.CPUVCPUs != 4 || limits.MemoryBytes != 16<<30 {
		t.Errorf("limits = %+v, want the whole pool", limits)
	}
	if effectiveMax != 8 {
		t.Errorf("effectiveMax = %d, want 8", effectiveMax)
	}
}

// Dividing a pool too far produces a share below what any agent image needs to
// start. Such a sandbox schedules and then dies, which reads as a broken agent
// rather than an oversubscribed pool.
func TestSandboxShareAppliesMemoryFloor(t *testing.T) {
	// 4Gi across 100 would be ~41MB each.
	capacity := ResourceQuantity{CPUVCPUs: 4, MemoryBytes: 4 << 30}

	requests, _, effectiveMax := SandboxShare(capacity, 100)

	if requests.MemoryBytes != MinSandboxMemoryBytes {
		t.Errorf("request memory = %d, want the floor %d", requests.MemoryBytes, MinSandboxMemoryBytes)
	}
	// Once the floor applies, the pool genuinely holds fewer sandboxes than
	// asked for, and the caller needs the true number to bound the pool by.
	if want := int((4 << 30) / MinSandboxMemoryBytes); effectiveMax != want {
		t.Errorf("effectiveMax = %d, want %d", effectiveMax, want)
	}
	if effectiveMax >= 100 {
		t.Error("the floor must reduce how many sandboxes the pool admits")
	}
}

func TestSandboxShareNeverReservesMoreThanItAllows(t *testing.T) {
	// A pool smaller than one floor's worth of memory.
	capacity := ResourceQuantity{CPUVCPUs: 0.001, MemoryBytes: 50 << 20}

	requests, limits, effectiveMax := SandboxShare(capacity, 4)

	if limits.MemoryBytes < requests.MemoryBytes {
		t.Errorf("limit %d is below request %d", limits.MemoryBytes, requests.MemoryBytes)
	}
	if limits.CPUVCPUs < requests.CPUVCPUs {
		t.Errorf("limit %v is below request %v", limits.CPUVCPUs, requests.CPUVCPUs)
	}
	// Reporting zero would make the pool silently accept nothing; one sandbox
	// surfaces the problem as a scheduling failure instead.
	if effectiveMax < 1 {
		t.Errorf("effectiveMax = %d, want at least 1", effectiveMax)
	}
}

func TestSandboxShareDefaultsMaxSandboxes(t *testing.T) {
	capacity := ResourceQuantity{CPUVCPUs: 10, MemoryBytes: 100 << 30}

	requests, _, effectiveMax := SandboxShare(capacity, 0)

	if effectiveMax != DefaultMaxSandboxes {
		t.Errorf("effectiveMax = %d, want %d", effectiveMax, DefaultMaxSandboxes)
	}
	if requests.CPUVCPUs != 1 {
		t.Errorf("request cpu = %v, want 1", requests.CPUVCPUs)
	}
}
