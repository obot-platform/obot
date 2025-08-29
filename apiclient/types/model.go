package types

type Model struct {
	Metadata
	ModelManifest
	ModelStatus
}

type ModelManifest struct {
	Name          string     `json:"name,omitempty"`
	DisplayName   string     `json:"displayName,omitempty"`
	TargetModel   string     `json:"targetModel,omitempty"`
	ModelProvider string     `json:"modelProvider,omitempty"`
	Alias         string     `json:"alias,omitempty"`
	Active        bool       `json:"active"`
	Usage         ModelUsage `json:"usage"`
}

type ModelList List[Model]

type ModelStatus struct {
	AliasAssigned     *bool  `json:"aliasAssigned,omitempty"`
	ModelProviderName string `json:"modelProviderName"`
	Icon              string `json:"icon,omitempty"`
	IconDark          string `json:"iconDark,omitempty"`
}

type ModelUsage string

const (
	ModelUsageLLM       ModelUsage = "llm"
	ModelUsageEmbedding ModelUsage = "text-embedding"
	ModelUsageImage     ModelUsage = "image-generation"
	ModelUsageVision    ModelUsage = "vision"
	ModelUsageOther     ModelUsage = "other"
	ModelUsageUnknown   ModelUsage = ""
)
