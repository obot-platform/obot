package types

// K8sSettings represents global Kubernetes configuration for MCP server deployments
type K8sSettings struct {
	// Affinity rules (JSON/YAML blob)
	Affinity string `json:"affinity,omitempty"`

	// Tolerations (JSON/YAML blob)
	Tolerations string `json:"tolerations,omitempty"`

	// Resources configuration (JSON/YAML blob)
	Resources string `json:"resources,omitempty"`

	// SetViaHelm indicates settings are from Helm (cannot be updated via API)
	SetViaHelm bool `json:"setViaHelm,omitempty"`

	Metadata Metadata `json:"metadata,omitempty"`
}
