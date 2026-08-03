package agentbackend

const (
	// MinSandboxMemoryBytes is the smallest memory reservation a sandbox may be
	// given. Dividing a pool between many sandboxes can produce a share below
	// what any real agent image needs to start, and such a sandbox schedules
	// successfully and then dies immediately, which reads as a broken agent
	// rather than an over-subscribed pool.
	MinSandboxMemoryBytes int64 = 200 << 20 // 200MB

	// MinSandboxCPUVCPUs keeps a reservation from rounding to zero. CPU is
	// compressible, so a small share only means a slow sandbox, but a zero
	// request means no guarantee at all.
	MinSandboxCPUVCPUs = 0.01

	// DefaultMaxSandboxes applies when a pool does not say. It is a capacity
	// planning decision, so there is no good universal answer; this only keeps
	// an unconfigured pool usable.
	DefaultMaxSandboxes = 10
)

// SandboxShare divides a pool between its sandboxes.
//
// A pool is a shared bucket. Each sandbox reserves an equal fraction of it, so
// that it can always be scheduled, and may burst to the whole pool when its
// neighbours are idle. Agents therefore have no size of their own: raising a
// pool's sandbox count shrinks everyone's guaranteed share and nothing else.
//
// This is an approximation of a single shared cgroup, which Kubernetes cannot
// express across pods. The difference that remains is that reservations are
// summed against the node, so a pool cannot be oversubscribed as freely as one
// cgroup would allow.
//
// The floors mean effectiveMax can be lower than the configured maximum: once a
// share would fall below a floor, the pool holds fewer sandboxes than asked
// for. Returning that, rather than silently admitting sandboxes that cannot
// run, lets the caller bound the pool by a number that is actually true.
func SandboxShare(capacity ResourceQuantity, maxSandboxes int) (requests, limits InstanceResources, effectiveMax int) {
	if maxSandboxes <= 0 {
		maxSandboxes = DefaultMaxSandboxes
	}

	limits = InstanceResources{
		CPUVCPUs:    capacity.CPUVCPUs,
		MemoryBytes: capacity.MemoryBytes,
	}

	requests = InstanceResources{
		CPUVCPUs:    capacity.CPUVCPUs / float64(maxSandboxes),
		MemoryBytes: capacity.MemoryBytes / int64(maxSandboxes),
	}

	effectiveMax = maxSandboxes
	if requests.MemoryBytes < MinSandboxMemoryBytes {
		requests.MemoryBytes = MinSandboxMemoryBytes
		effectiveMax = min(effectiveMax, int(capacity.MemoryBytes/MinSandboxMemoryBytes))
	}
	if requests.CPUVCPUs < MinSandboxCPUVCPUs {
		requests.CPUVCPUs = MinSandboxCPUVCPUs
		effectiveMax = min(effectiveMax, int(capacity.CPUVCPUs/MinSandboxCPUVCPUs))
	}

	// A pool too small to hold even one sandbox at the floor still reports one,
	// so the failure surfaces as that sandbox failing to schedule against the
	// quota rather than as a pool that silently accepts nothing.
	effectiveMax = max(effectiveMax, 1)

	// A sandbox may never be limited below what it reserves.
	limits.CPUVCPUs = max(limits.CPUVCPUs, requests.CPUVCPUs)
	limits.MemoryBytes = max(limits.MemoryBytes, requests.MemoryBytes)

	return requests, limits, effectiveMax
}
