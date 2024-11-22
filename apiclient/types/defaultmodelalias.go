package types

type DefaultModelAliasType string

const (
	DefaultModelAliasTypeTextEmbedding   DefaultModelAliasType = "text-embedding"
	DefaultModelAliasTypeLLM             DefaultModelAliasType = "llm"
	DefaultModelAliasTypeLLMMini         DefaultModelAliasType = "llm-mini"
	DefaultModelAliasTypeImageGeneration DefaultModelAliasType = "image-generation"
)

type DefaultModelAlias struct {
	DefaultModelAliasManifest
}

type DefaultModelAliasManifest struct {
	Alias string `json:"alias"`
	Model string `json:"model"`
}

type DefaultModelAliasList List[DefaultModelAlias]
