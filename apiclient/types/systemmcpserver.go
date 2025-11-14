package types

// SystemType represents the type of system server
type SystemType string

// SystemType constants for different system server types
const (
	SystemTypeHook SystemType = "hook"
)

// SystemMCPServer represents a system-level MCP server for Obot's internal use
type SystemMCPServer struct {
	Metadata
	Manifest             MCPServerManifest    `json:"manifest"`
	SystemServerSettings SystemServerSettings `json:"systemServerSettings"`
	SourceURL            string               `json:"sourceURL,omitempty"`
	Editable             bool                 `json:"editable,omitempty"`

	// Configuration status
	Configured             bool     `json:"configured"`
	MissingRequiredEnvVars []string `json:"missingRequiredEnvVars,omitempty"`
	MissingRequiredHeaders []string `json:"missingRequiredHeaders,omitempty"`

	// Deployment status
	DeploymentStatus            string               `json:"deploymentStatus,omitempty"`
	DeploymentAvailableReplicas *int32               `json:"deploymentAvailableReplicas,omitempty"`
	DeploymentReadyReplicas     *int32               `json:"deploymentReadyReplicas,omitempty"`
	DeploymentReplicas          *int32               `json:"deploymentReplicas,omitempty"`
	DeploymentConditions        []DeploymentCondition `json:"deploymentConditions,omitempty"`
	K8sSettingsHash             string               `json:"k8sSettingsHash,omitempty"`
}

// SystemServerSettings contains settings specific to system servers
type SystemServerSettings struct {
	IsEnabled  bool       `json:"isEnabled"`
	SystemType SystemType `json:"systemType"`
}

type SystemMCPServerList List[SystemMCPServer]

// SystemMCPServerManifest is used for create/update operations
type SystemMCPServerManifest struct {
	Manifest             MCPServerManifest    `json:"manifest"`
	SystemServerSettings SystemServerSettings `json:"systemServerSettings"`
}

// SystemMCPServerSources tracks git source URLs for system servers
type SystemMCPServerSources struct {
	Metadata
	SourceURLs []string          `json:"sourceURLs"`
	LastSynced Time              `json:"lastSynced,omitzero"`
	SyncErrors map[string]string `json:"syncErrors,omitempty"`
	IsSyncing  bool              `json:"isSyncing,omitempty"`
}
