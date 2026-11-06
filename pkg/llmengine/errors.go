package llmengine

import (
	"regexp"
)

var (
	engineName = regexp.MustCompile(`(?i)\bbifrost\b`)
)

// ScrubEngineName replaces the engine's name in an error message with the product's own, so
// errors read as coming from Obot's LLM gateway.
func ScrubEngineName(message string) string {
	return engineName.ReplaceAllString(message, "LLM gateway")
}

// ScrubEngineNameBytes is ScrubEngineName for a response body.
func ScrubEngineNameBytes(body []byte) []byte {
	return engineName.ReplaceAll(body, []byte("LLM gateway"))
}
