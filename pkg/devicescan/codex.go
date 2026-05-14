package devicescan

import (
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	codexGlobalConfigRel   = ".codex/config.toml"
	codexPluginCacheRel    = ".codex/plugins/cache"
	codexPluginManifestSub = ".codex-plugin/plugin.json"
)

// codexConfig is Codex's TOML shape: a top-level mcp_servers map of
// named entries. Codex has a unique header model captured separately
// (see codexEntry.headerKeys).
type codexConfig struct {
	MCPServers map[string]codexEntry `toml:"mcp_servers"`
}

type codexEntry struct {
	Type              string         `toml:"type"`
	Transport         string         `toml:"transport"`
	Command           string         `toml:"command"`
	Args              []string       `toml:"args"`
	URL               string         `toml:"url"`
	Env               map[string]any `toml:"env"`
	HTTPHeaders       map[string]any `toml:"http_headers"`
	EnvHTTPHeaders    map[string]any `toml:"env_http_headers"`
	BearerTokenEnvVar string         `toml:"bearer_token_env_var"`
	Enabled           *bool          `toml:"enabled"`
}

type codexScanner struct{}

func (codexScanner) Name() string { return "codex" }

func (codexScanner) Presence() clientPresenceDef {
	return clientPresenceDef{binaries: []string{"codex"}, configPaths: []string{".codex"}}
}

func (codexScanner) GlobalConfigPaths() []string { return []string{codexGlobalConfigRel} }

func (codexScanner) ProjectGlobs() []string { return []string{"**/.codex/config.toml"} }

func (codexScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	cfg, ok := readTOML[codexConfig](s.fsys, codexGlobalConfigRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(codexGlobalConfigRel)
	return codexEmit(cfg.MCPServers, configAbs, "")
}

func (codexScanner) ScanProject(s *scanState, configRel string) []types.DeviceScanMCPServer {
	cfg, ok := readTOML[codexConfig](s.fsys, configRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(configRel)
	projectAbs := s.abs(path.Dir(path.Dir(configRel)))
	return codexEmit(cfg.MCPServers, configAbs, projectAbs)
}

func codexEmit(servers map[string]codexEntry, configAbs, projectAbs string) []types.DeviceScanMCPServer {
	out := make([]types.DeviceScanMCPServer, 0, len(servers))
	for name, e := range servers {
		if e.Enabled != nil && !*e.Enabled {
			continue
		}
		out = append(out, e.toServer(name, configAbs, projectAbs))
	}
	return out
}

// toServer converts a [mcp_servers.<name>] table into wire shape.
// Codex header semantics: http_headers (literal map), env_http_headers
// (header_name → env_var), and bearer_token_env_var (yields an
// "Authorization" header). Only header *names* propagate to HeaderKeys.
func (e codexEntry) toServer(name, configAbs, projectAbs string) types.DeviceScanMCPServer {
	transport := codexTransport(e.Type, e.Transport, e.URL)

	headerNames := map[string]bool{}
	for k := range e.HTTPHeaders {
		headerNames[k] = true
	}
	for k := range e.EnvHTTPHeaders {
		headerNames[k] = true
	}
	if e.BearerTokenEnvVar != "" {
		headerNames["Authorization"] = true
	}
	headerKeys := make([]string, 0, len(headerNames))
	for k := range headerNames {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)

	return types.DeviceScanMCPServer{
		Client:      "codex",
		ProjectPath: projectAbs,
		File:        configAbs,
		Name:        name,
		Transport:   transport,
		Command:     e.Command,
		Args:        e.Args,
		URL:         e.URL,
		EnvKeys:     sortedMapKeys(e.Env),
		HeaderKeys:  headerKeys,
		ConfigHash:  mcpConfigHash(name, transport, e.Command, e.Args, e.URL),
	}
}

// codexTransport differs from the standard rule because Codex defaults
// remote (URL-only) servers to streamable-http rather than sse.
func codexTransport(typeField, transportField, urlField string) string {
	if explicit := firstNonEmpty(typeField, transportField); explicit != "" {
		n := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(explicit)), "_", "-")
		if n == "streamablehttp" {
			n = "streamable-http"
		}
		return n
	}
	if urlField != "" {
		return "streamable-http"
	}
	return "stdio"
}

// ScanPlugins walks .codex/plugins/cache/<marketplace>/<plugin>/<ver>/
// and emits a Plugin observation for the highest-semver version of each
// plugin that has a manifest at .codex-plugin/plugin.json.
func (codexScanner) ScanPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	mkts, err := fs.ReadDir(s.fsys, codexPluginCacheRel)
	if err != nil {
		return nil, nil, nil
	}
	var (
		ps  []types.DeviceScanPlugin
		ms  []types.DeviceScanMCPServer
		sks []types.DeviceScanSkill
	)
	for _, mkt := range mkts {
		if !mkt.IsDir() {
			continue
		}
		mktRel := path.Join(codexPluginCacheRel, mkt.Name())
		plugins, err := fs.ReadDir(s.fsys, mktRel)
		if err != nil {
			log.Debugf("codex: skipping marketplace %q: %v", mktRel, err)
			continue
		}
		for _, p := range plugins {
			if !p.IsDir() {
				continue
			}
			pluginRel := path.Join(mktRel, p.Name())
			versionRel, version, ok := pickHighestVersionDir(s.fsys, pluginRel)
			if !ok {
				continue
			}
			manifestRel := path.Join(versionRel, codexPluginManifestSub)
			if !fileExists(s.fsys, manifestRel) {
				continue
			}
			ep := emitPlugin(s, emitPluginOpts{
				installRel:      versionRel,
				manifestRel:     manifestRel,
				pluginType:      "codex_plugin",
				client:          "codex",
				marketplace:     mkt.Name(),
				enabled:         true,
				nameFallback:    p.Name(),
				versionFallback: version,
				nestedMCPRel:    []string{"mcp.json", ".mcp.json"},
			})
			ps = append(ps, ep.plugin)
			ms = append(ms, ep.servers...)
			sks = append(sks, ep.skills...)
		}
	}
	return ps, ms, sks
}

// pickHighestVersionDir returns the version subdirectory with the
// highest semver-aware key, the directory's basename (the version
// string), and ok. Non-directory entries are ignored.
func pickHighestVersionDir(fsys fs.FS, pluginRel string) (string, string, bool) {
	entries, err := fs.ReadDir(fsys, pluginRel)
	if err != nil {
		log.Debugf("codex: skipping plugin %q: %v", pluginRel, err)
		return "", "", false
	}
	type cand struct {
		name string
		key  []vPart
	}
	cands := make([]cand, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cands = append(cands, cand{name: e.Name(), key: versionSortKey(e.Name())})
	}
	if len(cands) == 0 {
		return "", "", false
	}
	sort.Slice(cands, func(i, j int) bool {
		return compareVParts(cands[i].key, cands[j].key) > 0 // descending
	})
	top := cands[0].name
	return path.Join(pluginRel, top), top, true
}

// vPart is one segment of a parsed version string. kind 0 = numeric
// segment / numeric prerelease, kind 1 = alpha segment / alpha
// prerelease token, kind 2 = release sentinel (appended when the
// version has no prerelease suffix).
type vPart struct {
	kind int
	n    int
	s    string
}

func versionSortKey(name string) []vPart {
	versionStr, prerelease, hasDash := strings.Cut(name, "-")
	var parts []vPart
	for seg := range strings.SplitSeq(versionStr, ".") {
		if n, err := strconv.Atoi(seg); err == nil {
			parts = append(parts, vPart{0, n, ""})
		} else {
			parts = append(parts, vPart{1, 0, seg})
		}
	}
	if hasDash && prerelease != "" {
		for _, tok := range alphaNumTokens(prerelease) {
			if n, err := strconv.Atoi(tok); err == nil {
				parts = append(parts, vPart{0, n, ""})
			} else {
				parts = append(parts, vPart{1, 0, tok})
			}
		}
	} else {
		parts = append(parts, vPart{2, 0, ""})
	}
	return parts
}

var alphaNumRe = regexp.MustCompile(`[A-Za-z]+|\d+`)

func alphaNumTokens(s string) []string {
	return alphaNumRe.FindAllString(s, -1)
}

func compareVParts(a, b []vPart) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i].kind != b[i].kind {
			return a[i].kind - b[i].kind
		}
		if a[i].n != b[i].n {
			return a[i].n - b[i].n
		}
		if a[i].s != b[i].s {
			if a[i].s < b[i].s {
				return -1
			}
			return 1
		}
	}
	return len(a) - len(b)
}
