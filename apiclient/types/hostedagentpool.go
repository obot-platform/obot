package types

import (
	"fmt"
	"math"
)

// HostedAgentResourceQuantity is the shared capacity envelope for a hosted
// agent pool. It is intentionally not a per-instance reservation.
type HostedAgentResourceQuantity struct {
	CPUVCPUs     float64 `json:"cpuVcpus,omitempty"`
	MemoryBytes  int64   `json:"memoryBytes,omitempty"`
	StorageBytes int64   `json:"storageBytes,omitempty"`
}

func (q HostedAgentResourceQuantity) Validate() error {
	if q.CPUVCPUs <= 0 || math.IsNaN(q.CPUVCPUs) || math.IsInf(q.CPUVCPUs, 0) {
		return fmt.Errorf("cpuVcpus must be a finite value greater than zero")
	}
	if q.MemoryBytes <= 0 {
		return fmt.Errorf("memoryBytes must be greater than zero")
	}
	if q.StorageBytes <= 0 {
		return fmt.Errorf("storageBytes must be greater than zero")
	}
	return nil
}

type HostedAgentPool struct {
	Metadata                `json:",inline"`
	HostedAgentPoolManifest `json:",inline"`
	Status                  HostedAgentPoolStatus `json:"status,omitempty"`
}

type HostedAgentPoolManifest struct {
	Capacity HostedAgentResourceQuantity `json:"capacity"`
	// MaxSandboxes is how many sandboxes share this pool. It is the only thing
	// that sizes them: each reserves Capacity/MaxSandboxes and may burst to the
	// whole pool, so raising it makes every sandbox's guaranteed share smaller.
	MaxSandboxes int  `json:"maxSandboxes,omitempty"`
	Suspended    bool `json:"suspended,omitempty"`
}

func (m HostedAgentPoolManifest) Validate() error {
	if err := m.Capacity.Validate(); err != nil {
		return err
	}
	if m.MaxSandboxes < 0 {
		return fmt.Errorf("maxSandboxes must be greater than or equal to zero")
	}
	return nil
}

type HostedAgentPoolStatus struct {
	BackendProjectID string `json:"backendProjectID,omitempty"`
	BackendPoolID    string `json:"backendPoolID,omitempty"`

	Ready       bool `json:"ready,omitempty"`
	Schedulable bool `json:"schedulable,omitempty"`
	Degraded    bool `json:"degraded,omitempty"`

	Capacity HostedAgentResourceQuantity `json:"capacity,omitempty"`

	ObservedRevision string `json:"observedRevision,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Message          string `json:"message,omitempty"`
}

type HostedAgentPoolList List[HostedAgentPool]

// HostedAgentPoolDefaults contains the deployment-wide pool
// settings used when Obot creates a user's initial pool.
type HostedAgentPoolDefaults struct {
	Metadata                        `json:",inline"`
	HostedAgentPoolDefaultsManifest `json:",inline"`
}

type HostedAgentPoolDefaultsManifest struct {
	Capacity     HostedAgentResourceQuantity `json:"capacity"`
	MaxSandboxes int                         `json:"maxSandboxes,omitempty"`
	Suspended    bool                        `json:"suspended,omitempty"`
}

func (m HostedAgentPoolDefaultsManifest) Validate() error {
	if err := m.Capacity.Validate(); err != nil {
		return err
	}
	if m.MaxSandboxes < 0 {
		return fmt.Errorf("maxSandboxes must be greater than or equal to zero")
	}
	return nil
}

type HostedAgentPoolDefaultsList List[HostedAgentPoolDefaults]

// HostedAgentPoolAssignment grants a user access to a pool.
// Default selects the pool Obot initially uses when no pool is
// explicitly requested.
type HostedAgentPoolAssignment struct {
	Metadata                          `json:",inline"`
	HostedAgentPoolAssignmentManifest `json:",inline"`
}

type HostedAgentPoolAssignmentManifest struct {
	UserID  string `json:"userID"`
	PoolID  string `json:"poolID"`
	Default bool   `json:"default,omitempty"`
}

func (m HostedAgentPoolAssignmentManifest) Validate() error {
	if m.UserID == "" {
		return fmt.Errorf("userID is required")
	}
	if m.PoolID == "" {
		return fmt.Errorf("poolID is required")
	}
	return nil
}

type HostedAgentPoolAssignmentList List[HostedAgentPoolAssignment]
