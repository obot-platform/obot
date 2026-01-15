package types

// CapacitySource indicates where the capacity data comes from
type CapacitySource string

const (
	CapacitySourceResourceQuota CapacitySource = "resourceQuota"
	CapacitySourceDeployments   CapacitySource = "deployments"
)

// MCPCapacityInfo represents MCP namespace capacity information
type MCPCapacityInfo struct {
	// Source indicates where the capacity data comes from (graceful degradation)
	Source CapacitySource `json:"source"`

	// CPURequested is the total CPU requested by MCP deployments
	CPURequested string `json:"cpuRequested,omitempty"`
	// CPULimit is the CPU limit from ResourceQuota
	CPULimit string `json:"cpuLimit,omitempty"`

	// MemoryRequested is the total memory requested by MCP deployments
	MemoryRequested string `json:"memoryRequested,omitempty"`
	// MemoryLimit is the memory limit from ResourceQuota
	MemoryLimit string `json:"memoryLimit,omitempty"`

	// ActiveDeployments is the number of active MCP server deployments
	ActiveDeployments int `json:"activeDeployments"`

	// Error message if capacity info couldn't be fully retrieved
	Error string `json:"error,omitempty"`
}

// MCPResourceRequests represents the resource requests for an MCP server deployment
type MCPResourceRequests struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}
