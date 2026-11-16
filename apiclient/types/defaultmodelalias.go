package types

const (
	DefaultModelAliasTypeLLM     DefaultModelAliasType = "llm"
	DefaultModelAliasTypeLLMMini DefaultModelAliasType = "llm-mini"
	DefaultModelAliasTypeUnknown DefaultModelAliasType = "unknown"
)

type DefaultModelAliasType string

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
	case DefaultModelAliasTypeLLM,
		DefaultModelAliasTypeLLMMini,
		DefaultModelAliasTypeUnknown:
	default:
		t = DefaultModelAliasTypeUnknown
	}

	return t
}

// IsRemovedDefaultModelAlias identifies retired built-ins without restricting custom aliases.
func IsRemovedDefaultModelAlias(alias string) bool {
	switch alias {
	case "vision", "image-generation", "text-embedding":
		return true
	default:
		return false
	}
}
