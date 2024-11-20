package types

type Model struct {
	Metadata
	ModelManifest
	ModelStatus
}

type ModelManifest struct {
	Name          string     `json:"name,omitempty"`
	TargetModel   string     `json:"targetModel,omitempty"`
	ModelProvider string     `json:"modelProvider,omitempty"`
	Alias         string     `json:"alias,omitempty"`
	Active        bool       `json:"active"`
	Default       bool       `json:"default"`
	Usage         ModelUsage `json:"usage,omitempty"`
}

type ModelList List[Model]

type ModelStatus struct {
	ModelProviderStatus
	AliasAssigned bool `json:"aliasAssigned,omitempty"`
}

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
