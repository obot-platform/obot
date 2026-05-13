// Package devicescan inventories local AI client configuration on a
// device.
//
// Scan reads known config locations under the running user's home
// directory, parses MCP server, skill, and plugin observations, and
// returns a types.DeviceScanManifest suitable for submission to the
// Obot backend.
//
// Each AI client is one file (claudecode.go, cursor.go, …) defining a
// package-level value of type client. The client declares the binary
// names and macOS app bundle that signal its installation, the
// absolute paths it owns (directRules), and the path suffixes the home
// walk should match (walkRules).
//
// Parse functions are pure: they take an absolute path and return a
// parseResult. The orchestrator collects every parseResult and hands
// the slice — along with separately gathered presence rows — to
// mergeResults() for the final dedup/merge/sort.
package devicescan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
)

var log = logger.Package()

// OS-aware base paths, resolved once at package init.
//   - homeDir: $HOME (macOS/Linux) or %USERPROFILE% (Windows).
//   - configDir: ~/Library/Application Support (macOS), $XDG_CONFIG_HOME
//     or ~/.config (Linux), %APPDATA% (Windows).
//   - macAppsDir: macOS /Applications.
//
// Empty when the stdlib call fails — paths built on top stat to ENOENT
// and silently skip.
var (
	homeDir, _   = os.UserHomeDir()
	configDir, _ = os.UserConfigDir()
	macAppsDir   = "/Applications"
)

// multiClient is the synthetic client tag for SKILL.md files that can
// not be pinned to a specific AI client (e.g. .agents/skills, project
// skills outside a known client tree). It appears on observation rows
// so consumers can group them, but mergeResults() suppresses it from the
// top-level clients[] dimension.
const multiClient = "multi"

// allClients is the static registry the orchestrator walks. Adding a
// new AI client means appending here and writing one file in this
// package with a package-level var of type client.
//
// projectSkills is the synthetic last entry; it has no presence and
// no direct paths — it exists to register the multi-client
// SKILL.md walk pattern (see skills.go).
var allClients = []client{
	claudeCode,
	claudeDesktop,
	codex,
	cursor,
	goose,
	hermes,
	openclaw,
	opencode,
	vscode,
	windsurf,
	zed,
	projectSkills,
}

// walkSkipDirs are basenames the home walk prunes — dependency caches,
// build outputs, and OS app-support trees. Per-client subtrees go in
// the owning client's walkSkipPrefixes instead.
var walkSkipDirs = map[string]struct{}{
	"node_modules": {},
	".git":         {},
	".venv":        {},
	"venv":         {},
	"__pycache__":  {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	"target":       {},
	".next":        {},
	".nuxt":        {},
	".turbo":       {},
	".cache":       {},
	".npm":         {},
	".yarn":        {},
	"Library":      {},
	"AppData":      {},
	".Trash":       {},
	"tmp":          {},
	"temp":         {},
}

// parseResult is the pure output of every parse function. Each parser
// returns one of these; the orchestrator collects them all and hands
// the slice to mergeResults() for dedup, merge, rollup, and sort.
type parseResult struct {
	files   []types.DeviceScanFile
	mcps    []types.DeviceScanMCPServer
	skills  []types.DeviceScanSkill
	plugins []types.DeviceScanPlugin
	clients []types.DeviceScanClient
}

// parseFunc reads path and returns observations. No mutation, no
// shared state.
type parseFunc func(path string) parseResult

// parseRule pairs a target with the parser that should run when the
// target matches. The interpretation depends on which slice the rule
// lives in on client: directRules carry absolute filesystem paths
// stat'd once at scan start; walkRules carry path suffixes matched at
// directory boundaries during the home walk.
type parseRule struct {
	target string
	parse  parseFunc
}

// client is a per-AI-client value: everything declarative the
// orchestrator needs to scan one client. Each <client>.go file in this
// package defines one of these as a package-level var and appends it
// to allClients.
type client struct {
	// name is the wire `client` tag emitted on observations. Empty
	// names are valid for synthetic clients that contribute only walk
	// rules (e.g. projectSkills); detectPresence short-circuits and
	// emits no presence row in that case.
	name string

	// binaries is the list of executable names looked up in $PATH
	// (cross-OS). The first match becomes BinaryPath on the presence
	// row.
	binaries []string

	// appBundle is the macOS-only app bundle name (e.g. "Cursor.app")
	// checked under /Applications. Empty when the client has no macOS
	// bundle or is presence-detected by another signal.
	appBundle string

	// directRules carry absolute filesystem paths the orchestrator
	// stat()s once at scan start. If the target exists (file or dir),
	// parse runs.
	directRules []parseRule

	// walkRules carry path suffixes the home walk matches against
	// every visited file (with a directory-boundary check before the
	// suffix).
	walkRules []parseRule

	// walkSkipPrefixes are home-relative path prefixes the central
	// walk must not descend into, because a direct rule already owns
	// that subtree.
	walkSkipPrefixes []string
}

func (r *parseResult) merge(other parseResult) {
	r.files = append(r.files, other.files...)
	r.mcps = append(r.mcps, other.mcps...)
	r.skills = append(r.skills, other.skills...)
	r.plugins = append(r.plugins, other.plugins...)
	r.clients = append(r.clients, other.clients...)
}

// Scan runs the full collection pipeline against the running user's
// environment and returns the assembled DeviceScanManifest. Per-parser
// errors are dropped (logged at debug level) so a missing or malformed
// config never aborts the rest of the scan. Context cancellation
// propagates between paths and during the walk.
//
// Server-assigned envelope fields (ScannerVersion, ScannedAt,
// DeviceID, Hostname, OS, Arch, Username, ID, ReceivedAt,
// SubmittedBy) are left zero; the caller fills them in.
//
// maxDepth caps how deep the project walk descends from the home
// root. Direct paths are not subject to maxDepth — they always run
// regardless.
func Scan(ctx context.Context, maxDepth int) (types.DeviceScanManifest, error) {
	var results []parseResult

	// 1. Direct rules — known config files / dirs. Run unconditionally
	// regardless of walk depth so deeply-nested global configs (e.g.
	// macOS Application Support paths) still match.
	for _, c := range allClients {
		for _, r := range c.directRules {
			if err := ctx.Err(); err != nil {
				return types.DeviceScanManifest{}, err
			}
			if r.target == "" {
				continue
			}
			if _, err := os.Stat(r.target); err == nil {
				results = append(results, r.parse(r.target))
			}
		}
	}

	// 2. Flatten walk rules + skip prefixes for the walk loop.
	//
	// directFiles holds absolute paths already handled by directRules
	// so the walk doesn't re-dispatch them when a walkRule's suffix
	// happens to match the global path (e.g. ~/.cursor/mcp.json also
	// matches the `.cursor/mcp.json` project-scope suffix rule).
	var (
		allWalkRules []parseRule
		skipPrefixes []string
		directFiles  = map[string]struct{}{}
	)
	for _, c := range allClients {
		allWalkRules = append(allWalkRules, c.walkRules...)
		for _, p := range c.walkSkipPrefixes {
			skipPrefixes = append(skipPrefixes, filepath.Join(homeDir, p))
		}

		for _, r := range c.directRules {
			if r.target != "" {
				directFiles[r.target] = struct{}{}
			}
		}
	}

	// 3. Single recursive walk under $HOME for project-scope configs
	// and free-floating SKILL.md markers. Per-entry errors (e.g.
	// permission denied on a single subdir) are intentionally swallowed
	// so the scan is best-effort; only context cancellation aborts the
	// walk and propagates to the caller.
	if homeDir != "" {
		if err := filepath.WalkDir(homeDir, func(path string, d fs.DirEntry, err error) error {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}

			if err != nil || path == homeDir {
				return nil
			}

			if d.IsDir() {
				if _, skip := walkSkipDirs[d.Name()]; skip {
					return filepath.SkipDir
				}

				for _, p := range skipPrefixes {
					if path == p || strings.HasPrefix(path, p+string(filepath.Separator)) {
						return filepath.SkipDir
					}
				}

				// depth=1 for top-level entries under homeDir. SkipDir on a
				// dir at depth==maxDepth means we don't descend into it,
				// so files match at depths 1…maxDepth (inclusive).
				rel, _ := filepath.Rel(homeDir, path)
				if strings.Count(rel, string(filepath.Separator))+1 >= maxDepth {
					return filepath.SkipDir
				}

				return nil
			}

			if _, skip := directFiles[path]; skip {
				return nil
			}

			slash := filepath.ToSlash(path)
			for _, r := range allWalkRules {
				if strings.HasSuffix(slash, "/"+r.target) {
					results = append(results, r.parse(path))
				}
			}

			return nil
		}); err != nil {
			return types.DeviceScanManifest{}, err
		}
	}

	// 4. Presence detection (real-OS access).
	var presence []types.DeviceScanClient
	for _, c := range allClients {
		if row, ok := detectPresence(c); ok {
			presence = append(presence, row)
		}
	}

	return mergeResults(results, presence), nil
}

// mergeResults assembles the wire manifest. Files are deduped by abs
// path (first wins) and sorted. Clients merge by name with first-non-
// empty-field-wins; has_* rollups come from the observation slices;
// orphan names (referenced by observations but missing from presence)
// are synthesised except for multiClient. MCP / Skill / Plugin
// observations keep their production order.
func mergeResults(results []parseResult, presence []types.DeviceScanClient) types.DeviceScanManifest {
	var (
		files   = make(map[string]types.DeviceScanFile)
		clients = make(map[string]types.DeviceScanClient)
		mcps    []types.DeviceScanMCPServer
		skills  []types.DeviceScanSkill
		plugins []types.DeviceScanPlugin
	)

	for _, r := range results {
		for _, f := range r.files {
			if _, ok := files[f.Path]; !ok {
				files[f.Path] = f
			}
		}

		mcps = append(mcps, r.mcps...)
		skills = append(skills, r.skills...)
		plugins = append(plugins, r.plugins...)
		for _, c := range r.clients {
			mergeClient(clients, c)
		}
	}

	for _, c := range presence {
		mergeClient(clients, c)
	}

	var (
		hasMCP    = map[string]struct{}{}
		hasSkill  = map[string]struct{}{}
		hasPlugin = map[string]struct{}{}
	)
	for _, m := range mcps {
		if m.Client != "" {
			hasMCP[m.Client] = struct{}{}
		}
	}

	for _, sk := range skills {
		if sk.Client != "" {
			hasSkill[sk.Client] = struct{}{}
		}
	}

	for _, p := range plugins {
		if p.Client != "" {
			hasPlugin[p.Client] = struct{}{}
		}
	}

	for _, set := range []map[string]struct{}{
		hasMCP,
		hasSkill,
		hasPlugin,
	} {
		for n := range set {
			if n == multiClient {
				continue
			}

			if _, ok := clients[n]; !ok {
				clients[n] = types.DeviceScanClient{Name: n}
			}
		}
	}

	out := types.DeviceScanManifest{
		Files:      make([]types.DeviceScanFile, 0, len(files)),
		Clients:    make([]types.DeviceScanClient, 0, len(clients)),
		MCPServers: mcps,
		Skills:     skills,
		Plugins:    plugins,
	}

	filePaths := make([]string, 0, len(files))
	for p := range files {
		filePaths = append(filePaths, p)
	}

	sort.Strings(filePaths)
	for _, p := range filePaths {
		out.Files = append(out.Files, files[p])
	}

	names := make([]string, 0, len(clients))
	for n := range clients {
		names = append(names, n)
	}

	sort.Strings(names)
	for _, n := range names {
		c := clients[n]
		_, c.HasMCPServers = hasMCP[n]
		_, c.HasSkills = hasSkill[n]
		_, c.HasPlugins = hasPlugin[n]
		out.Clients = append(out.Clients, c)
	}

	return out
}

// mergeClient upserts c into the table keyed by name. First-non-empty
// wins for Version / BinaryPath / InstallPath / ConfigPath.
func mergeClient(table map[string]types.DeviceScanClient, c types.DeviceScanClient) {
	if c.Name == "" {
		return
	}

	existing, ok := table[c.Name]
	if !ok {
		table[c.Name] = c
		return
	}

	if existing.Version == "" {
		existing.Version = c.Version
	}

	if existing.BinaryPath == "" {
		existing.BinaryPath = c.BinaryPath
	}

	if existing.InstallPath == "" {
		existing.InstallPath = c.InstallPath
	}

	if existing.ConfigPath == "" {
		existing.ConfigPath = c.ConfigPath
	}

	table[c.Name] = existing
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
