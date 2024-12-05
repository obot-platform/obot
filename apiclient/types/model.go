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
	Usage         ModelUsage `json:"usage,omitempty"`
}

type ModelList List[Model]

type ModelStatus struct {
	ModelProviderStatus
	AliasAssigned *bool `json:"aliasAssigned,omitempty"`
}

type ModelProviderStatus struct {
	Configured                      bool     `json:"configured"`
	RequiredConfigurationParameters []string `json:"requiredConfigurationParameters,omitempty"`
	MissingConfigurationParameters  []string `json:"missingConfigurationParameters,omitempty"`
}

type ModelUsage string

const (
	ModelUsageLLM       ModelUsage = "llm"
	ModelUsageEmbedding ModelUsage = "text-embedding"
	ModelUsageImage     ModelUsage = "image-generation"
	ModelUsageVision    ModelUsage = "vision"
	ModelUsageOther     ModelUsage = "other"
)
