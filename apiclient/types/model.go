package types

type Model struct {
	Metadata
	ModelManifest
	ModelProviderStatus
}

type ModelManifest struct {
	Name          string     `json:"name,omitempty"`
	TargetModel   string     `json:"targetModel,omitempty"`
	ModelProvider string     `json:"modelProvider,omitempty"`
	Active        bool       `json:"active"`
	Default       bool       `json:"default"`
	Usage         ModelUsage `json:"usage,omitempty"`
}

type ModelList List[Model]

type ModelProviderStatus struct {
	Configured     bool     `json:"configured"`
	MissingEnvVars []string `json:"missingEnvVars,omitempty"`
}

type ModelUsage string

const (
	ModelUsageAgent     ModelUsage = "agent"
	ModelUsageEmbedding ModelUsage = "text-embedding"
	ModelUsageImage     ModelUsage = "image-generation"
)
