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
	Dialect       string     `json:"dialect,omitempty"`
	// OverrideCost, when set, replaces the synced model cost.
	OverrideCost *ModelCost `json:"overrideCost,omitempty"`
}

type ModelList List[Model]

type ModelStatus struct {
	AliasAssigned     *bool     `json:"aliasAssigned,omitempty"`
	ModelProviderName string    `json:"modelProviderName"`
	Icon              string    `json:"icon,omitempty"`
	IconDark          string    `json:"iconDark,omitempty"`
	Cost              ModelCost `json:"cost,omitzero"`
}

// ModelCost contains per-million-token rates for a model.
type ModelCost struct {
	TokenUsageCost `json:",inline"`

	// Tiers contains threshold-specific rates.
	Tiers []ModelCostTier `json:"tiers,omitempty"`
}

type ModelCostTierType string

const ModelCostTierTypeContext ModelCostTierType = "context"

// ModelCostTier contains rates for a ModelCost threshold.
type ModelCostTier struct {
	TokenUsageCost `json:",inline"`

	Type ModelCostTierType `json:"type,omitempty"`
	Size *int              `json:"size,omitempty"`
}

// TokenUsageCost contains USD-per-million-token rates by usage bucket.
type TokenUsageCost struct {
	Input        float64 `json:"input,omitzero"`
	CacheRead    float64 `json:"cacheRead,omitzero"`
	CacheWrite   float64 `json:"cacheWrite,omitzero"`
	CacheWrite1h float64 `json:"cacheWrite1h,omitzero"`
	Output       float64 `json:"output,omitzero"`
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
