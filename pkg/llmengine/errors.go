package llmengine

import (
	"bytes"
	"net/http"
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

// IsStoreBusy reports whether a response from the engine's API failed because its SQLite
// store was locked by another write. The write was not made, and can be sent again.
func IsStoreBusy(statusCode int, body []byte) bool {
	if statusCode < http.StatusInternalServerError {
		return false
	}
	return bytes.Contains(body, []byte("database is locked")) || bytes.Contains(body, []byte("SQLITE_BUSY"))
}
