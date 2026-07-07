package types

// AppK8sSettings surfaces Obot server pod scheduling configuration.
// Values are read-only and sourced from the Obot Deployment pod template at request time.
type AppK8sSettings struct {
	// Affinity rules (YAML)
	Affinity string `json:"affinity,omitempty"`

	// Tolerations (YAML)
	Tolerations string `json:"tolerations,omitempty"`

	// Resources configuration (YAML)
	Resources string `json:"resources,omitempty"`

	// RuntimeClassName specifies the RuntimeClass for Obot server pods
	RuntimeClassName string `json:"runtimeClassName,omitempty"`
}
