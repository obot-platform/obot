package types

type DefaultModelAliasType string

const (
	DefaultModelAliasTypeTextEmbedding   DefaultModelAliasType = "text-embedding"
	DefaultModelAliasTypeLLM             DefaultModelAliasType = "llm"
	DefaultModelAliasTypeLLMMini         DefaultModelAliasType = "llm-mini"
	DefaultModelAliasTypeImageGeneration DefaultModelAliasType = "image-generation"
	DefaultModelAliasTypeVision          DefaultModelAliasType = "vision"
	DefaultModelAliasTypeUnknown         DefaultModelAliasType = "unknown"
)

type DefaultModelAlias struct {
	DefaultModelAliasManifest
}

type DefaultModelAliasManifest struct {
	Alias string `json:"alias"`
	Model string `json:"model"`
}

type DefaultModelAliasList List[DefaultModelAlias]

func DefaultModelAliasTypeFromString(str string) DefaultModelAliasType {
	t := DefaultModelAliasType(str)
	switch t {
	case DefaultModelAliasTypeTextEmbedding,
		DefaultModelAliasTypeLLM,
		DefaultModelAliasTypeLLMMini,
		DefaultModelAliasTypeImageGeneration,
		DefaultModelAliasTypeVision,
		DefaultModelAliasTypeUnknown:
	default:
		t = DefaultModelAliasTypeUnknown
	}

	return t
}
