package devicescan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// mcpConfigHash returns a stable SHA256 over the canonical (name, type,
// command, args, url) tuple of an MCP server. Env vars and headers are
// deliberately excluded — they vary per machine and would make otherwise
// identical server definitions look different across devices.
//
// Canonicalisation rules: keys are sorted (Go's json.Marshal orders
// map[string]any keys alphabetically), args order is preserved, empty
// command/url collapse to JSON null, and an empty args list collapses to [].
func mcpConfigHash(name, transport, command string, args []string, url string) string {
	canonical := map[string]any{
		"name":    name,
		"type":    transport,
		"command": stringOrNil(command),
		"args":    argsOrEmpty(args),
		"url":     stringOrNil(url),
	}
	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func stringOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func argsOrEmpty(a []string) []string {
	if a == nil {
		return []string{}
	}
	return a
}
