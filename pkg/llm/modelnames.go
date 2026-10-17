package llm

import (
	"slices"
	"strings"
)

var (
	// firstPartyProviders are the gateway providers that serve only the models their own
	// company makes. When several providers serve a model ID, the one that made it is the one a
	// bare ID such as "gpt-4o-mini" or "claude-sonnet-4-5" means; others, such as Azure,
	// Bedrock or a custom provider, host copies of other companies' models.
	firstPartyProviders = []string{"openai", "anthropic", "gemini", "mistral", "cohere", "xai", "deepseek"}
)

// IsFirstPartyProvider reports whether a gateway provider serves only its own company's models.
func IsFirstPartyProvider(provider string) bool {
	return slices.Contains(firstPartyProviders, provider)
}

// DatedVersionOf reports whether id is a dated version of alias, as Anthropic names them:
// "claude-sonnet-4-5-20250929" is a version of "claude-sonnet-4-5", which Anthropic's API
// accepts as an alias of its latest version. It returns the version's date (YYYYMMDD), which
// orders versions.
func DatedVersionOf(alias, id string) (string, bool) {
	date, ok := strings.CutPrefix(id, alias+"-")
	if !ok || len(date) != 8 {
		return "", false
	}
	for _, c := range date {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return date, true
}
