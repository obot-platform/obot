package types

// AppK8sSettings surfaces Helm-managed Obot server pod scheduling configuration.
// Values are read-only and sourced from the app scheduling snapshot captured at install/upgrade time.
type AppK8sSettings struct {
	// Available is false when Helm values are unavailable (for example, non-Kubernetes deployments).
	Available bool `json:"available"`

	// Affinity rules (YAML)
	Affinity string `json:"affinity,omitempty"`

	// Tolerations (YAML)
	Tolerations string `json:"tolerations,omitempty"`

	// Resources configuration (YAML)
	Resources string `json:"resources,omitempty"`

	// RuntimeClassName specifies the RuntimeClass for Obot server pods
	RuntimeClassName string `json:"runtimeClassName,omitempty"`
}
