package devicescan

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// mcpServerSpec is the canonical MCP server JSON shape shared by
// Cursor / VS Code / Claude Desktop / Claude Code / Windsurf. Clients
// that extend it (e.g. Windsurf's serverUrl) embed this struct; clients
// that diverge (OpenCode, Hermes, Codex, Goose, Zed) declare their own.
// Env / Headers are map[string]any since only keys are extracted.
type mcpServerSpec struct {
	Type      string         `json:"type" yaml:"type"`
	Transport string         `json:"transport" yaml:"transport"`
	Command   string         `json:"command" yaml:"command"`
	Args      []string       `json:"args" yaml:"args"`
	URL       string         `json:"url" yaml:"url"`
	Env       map[string]any `json:"env" yaml:"env"`
	Headers   map[string]any `json:"headers" yaml:"headers"`
	Enabled   *bool          `json:"enabled" yaml:"enabled"` // pointer: absence ≠ false
}

// normalizeTransport returns the wire transport string. Explicit
// type/transport wins; otherwise sse if a URL is present; otherwise
// stdio.
func normalizeTransport(typeField, transportField, urlField string) string {
	explicit := cmp.Or(typeField, transportField)
	if explicit != "" {
		n := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(explicit)), "_", "-")
		if n == "streamablehttp" {
			n = "streamable-http"
		}
		return n
	}
	if urlField != "" {
		return "sse"
	}
	return "stdio"
}

// mcpConfigHash returns a stable SHA256 over (name, type, command,
// args, url). Env and headers are excluded — they vary per machine.
// Nil args collapses to []; map keys are sorted by json.Marshal.
func mcpConfigHash(name, transport, command string, args []string, url string) string {
	if args == nil {
		args = []string{}
	}
	data, _ := json.Marshal(map[string]any{
		"name":    name,
		"type":    transport,
		"command": command,
		"args":    args,
		"url":     url,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
