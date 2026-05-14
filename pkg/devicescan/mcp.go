package devicescan

import (
	"sort"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// mcpServerSpec is the JSON shape that all JSON-format MCP server entries
// share. Optional fields default to zero — empty strings, nil maps/slices.
// Per-client config structs embed maps of these (e.g. claudeCodeConfig.MCPServers
// is map[string]mcpServerSpec).
//
// Env and Headers are kept as map[string]any because we only extract their
// keys; values are opaque to the scanner.
type mcpServerSpec struct {
	Type      string         `json:"type" yaml:"type"`
	Transport string         `json:"transport" yaml:"transport"`
	Command   string         `json:"command" yaml:"command"`
	Args      []string       `json:"args" yaml:"args"`
	URL       string         `json:"url" yaml:"url"`
	ServerURL string         `json:"serverUrl" yaml:"serverUrl"`
	Env       map[string]any `json:"env" yaml:"env"`
	Headers   map[string]any `json:"headers" yaml:"headers"`
	Enabled   *bool          `json:"enabled" yaml:"enabled"` // pointer: absence ≠ false
}

// toServer converts a parsed entry into a wire DeviceScanMCPServer with
// the standard JSON-config transport rules: explicit type/transport
// (lowercased, `_`→`-`, `streamablehttp`→`streamable-http`), then sse if
// url/serverUrl is set, then stdio.
func (e mcpServerSpec) toServer(name, client, fileAbs, projectAbs string) types.DeviceScanMCPServer {
	transport := normalizeTransport(e.Type, e.Transport, e.URL, e.ServerURL)
	url := firstNonEmpty(e.URL, e.ServerURL)
	return types.DeviceScanMCPServer{
		Client:      client,
		ProjectPath: projectAbs,
		File:        fileAbs,
		Name:        name,
		Transport:   transport,
		Command:     e.Command,
		Args:        e.Args,
		URL:         url,
		EnvKeys:     sortedMapKeys(e.Env),
		HeaderKeys:  sortedMapKeys(e.Headers),
		ConfigHash:  mcpConfigHash(name, transport, e.Command, e.Args, url),
	}
}

// normalizeTransport returns the wire transport string. Explicit
// type/transport wins; otherwise sse if a URL is present; otherwise stdio.
func normalizeTransport(typeField, transportField, urlField, serverURLField string) string {
	explicit := firstNonEmpty(typeField, transportField)
	if explicit != "" {
		n := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(explicit)), "_", "-")
		if n == "streamablehttp" {
			n = "streamable-http"
		}
		return n
	}
	if firstNonEmpty(urlField, serverURLField) != "" {
		return "sse"
	}
	return "stdio"
}

// sortedMapKeys returns the keys of m alphabetically. Returns a non-nil
// empty slice for nil/empty maps so JSON serialisation produces `[]`
// rather than `null`.
func sortedMapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// firstNonEmpty returns the first non-empty string in ss, or "".
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// emitJSONServersGlobal opens a JSON file at configRel and emits one MCP
// server per entry under the given top-level dict key. Used by JSON
// clients with no per-client quirks (Cursor, VS Code, Windsurf).
//
// The returned slice is empty if the file is missing, malformed, or the
// servers key is absent.
func emitJSONServersGlobal(s *scanState, configRel, serversKey, client string) []types.DeviceScanMCPServer {
	cfg, ok := readJSON[map[string]map[string]mcpServerSpec](s.fsys, configRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(configRel)
	servers := cfg[serversKey]
	out := make([]types.DeviceScanMCPServer, 0, len(servers))
	for name, e := range servers {
		out = append(out, e.toServer(name, client, configAbs, ""))
	}
	return out
}

// emitJSONServersProject parses a project-scope JSON config file and emits
// project-scope MCP server observations. projectAbs is the project root
// (computed per-client based on the marker's parent layout).
func emitJSONServersProject(s *scanState, configRel, serversKey, client, projectAbs string) []types.DeviceScanMCPServer {
	cfg, ok := readJSON[map[string]map[string]mcpServerSpec](s.fsys, configRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(configRel)
	servers := cfg[serversKey]
	out := make([]types.DeviceScanMCPServer, 0, len(servers))
	for name, e := range servers {
		out = append(out, e.toServer(name, client, configAbs, projectAbs))
	}
	return out
}
