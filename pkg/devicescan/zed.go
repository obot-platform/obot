package devicescan

import (
	"io/fs"
	"path"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// Zed on-disk layout. macOS-style extensions path. Other platforms expose
// their extensions under different roots that we do not currently scan.
const (
	zedGlobalConfigRel = ".config/zed/settings.json"
	zedExtensionsRel   = "Library/Application Support/Zed/extensions/installed"
	zedExtensionPrefix = "mcp-server-"
)

// zedSettings has only the field we care about. Zed's `context_servers`
// map keys use opaque server names; values follow Zed's own schema:
// either {url, env, headers} for SSE or {command, args, env} for stdio,
// with an optional explicit `enabled: false` skip.
type zedSettings struct {
	ContextServers map[string]zedContextServer `json:"context_servers"`
}

type zedContextServer struct {
	URL     string         `json:"url"`
	Command string         `json:"command"`
	Args    []string       `json:"args"`
	Env     map[string]any `json:"env"`
	Headers map[string]any `json:"headers"`
	Enabled *bool          `json:"enabled"`
}

type zedScanner struct{}

func (zedScanner) Name() string { return "zed" }

func (zedScanner) Presence() clientPresenceDef {
	return clientPresenceDef{
		binaries:    []string{"zed"},
		appBundles:  []string{"Zed.app"},
		configPaths: []string{".config/zed"},
	}
}

func (zedScanner) GlobalConfigPaths() []string { return []string{zedGlobalConfigRel} }

func (zedScanner) ProjectGlobs() []string { return []string{"**/.zed/settings.json"} }

func (zedScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	cfg, ok := readJSON[zedSettings](s.fsys, zedGlobalConfigRel)
	configAbs := ""
	if ok {
		configAbs = s.addFileOrAbs(zedGlobalConfigRel)
	}
	emitted, out := emitZedContextServers(cfg.ContextServers, configAbs, "")
	out = append(out, mergeZedExtensions(s, configAbs, emitted)...)
	return out
}

func (zedScanner) ScanProject(s *scanState, configRel string) []types.DeviceScanMCPServer {
	cfg, ok := readJSON[zedSettings](s.fsys, configRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(configRel)
	projectAbs := s.abs(path.Dir(path.Dir(configRel)))
	_, out := emitZedContextServers(cfg.ContextServers, configAbs, projectAbs)
	return out
}

// emitZedContextServers parses Zed's context_servers map. Returns the set
// of server names emitted (so the extensions merge can dedupe) and the
// observation slice.
func emitZedContextServers(servers map[string]zedContextServer, configAbs, projectAbs string) (map[string]bool, []types.DeviceScanMCPServer) {
	emitted := map[string]bool{}
	out := make([]types.DeviceScanMCPServer, 0, len(servers))
	for name, e := range servers {
		if e.Enabled != nil && !*e.Enabled {
			continue
		}
		obs, ok := e.toServer(name, configAbs, projectAbs)
		if !ok {
			continue
		}
		out = append(out, obs)
		emitted[name] = true
	}
	return emitted, out
}

// toServer parses a Zed context-server entry. URL → sse; command → stdio;
// neither (settings-only extension placeholders) → drop.
func (e zedContextServer) toServer(name, configAbs, projectAbs string) (types.DeviceScanMCPServer, bool) {
	if e.URL != "" {
		return types.DeviceScanMCPServer{
			Client:      "zed",
			ProjectPath: projectAbs,
			File:        configAbs,
			Name:        name,
			Transport:   "sse",
			URL:         e.URL,
			EnvKeys:     sortedMapKeys(e.Env),
			HeaderKeys:  sortedMapKeys(e.Headers),
			ConfigHash:  mcpConfigHash(name, "sse", "", nil, e.URL),
		}, true
	}
	if e.Command != "" {
		return types.DeviceScanMCPServer{
			Client:      "zed",
			ProjectPath: projectAbs,
			File:        configAbs,
			Name:        name,
			Transport:   "stdio",
			Command:     e.Command,
			Args:        e.Args,
			EnvKeys:     sortedMapKeys(e.Env),
			HeaderKeys:  []string{},
			ConfigHash:  mcpConfigHash(name, "stdio", e.Command, e.Args, ""),
		}, true
	}
	return types.DeviceScanMCPServer{}, false
}

// mergeZedExtensions scans the macOS extensions tree for folders prefixed
// with mcp-server- and emits a stdio observation for each name not
// already present in `existing`. The extension itself supplies command/
// args at runtime, so we leave those blank.
func mergeZedExtensions(s *scanState, configAbs string, existing map[string]bool) []types.DeviceScanMCPServer {
	entries, err := fs.ReadDir(s.fsys, zedExtensionsRel)
	if err != nil {
		return nil
	}
	var out []types.DeviceScanMCPServer
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), zedExtensionPrefix) {
			continue
		}
		name := e.Name()
		if existing[name] {
			continue
		}
		out = append(out, types.DeviceScanMCPServer{
			Client:     "zed",
			File:       configAbs,
			Name:       name,
			Transport:  "stdio",
			EnvKeys:    []string{},
			HeaderKeys: []string{},
			ConfigHash: mcpConfigHash(name, "stdio", "", nil, ""),
		})
	}
	return out
}
