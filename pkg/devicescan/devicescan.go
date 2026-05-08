// Package devicescan inventories local AI client configuration on a
// device.
//
// Scan reads known config locations under a home directory (provided as
// an fs.FS), parses MCP server, skill, and plugin observations, and
// returns a types.DeviceScan suitable for submission to the Obot
// backend.
//
// Each client is integrated as a value type implementing ClientScanner
// in its own file (claudecode.go, codex.go, …). The orchestrator below
// runs every scanner through a fixed pipeline: globals → glob walk →
// project hits → plugins → skills → presence → build.
package devicescan

import (
	"context"
	"io/fs"
	"sort"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
)

var log = logger.Package()

// Scan runs the full collection pipeline against fsys (rooted at
// homeAbs) and returns the assembled DeviceScan. Per-phase errors are
// dropped (logged at debug level) so a missing or malformed config
// never aborts the rest of the scan. Context cancellation propagates.
//
// Server-assigned envelope fields (ScannerVersion, ScannedAt, DeviceID,
// Hostname, OS, Arch, Username, ID, ReceivedAt, SubmittedBy) are left
// zero; the caller fills them in.
//
// maxDepth caps how deep the project walk descends from the home root
// when looking for project-scope configs and SKILL.md files.
func Scan(ctx context.Context, fsys fs.FS, homeAbs string, maxDepth int) (types.DeviceScanManifest, error) {
	s := newScanState(fsys, homeAbs)

	var (
		mcps    []types.DeviceScanMCPServer
		skills  []types.DeviceScanSkill
		plugins []types.DeviceScanPlugin
	)

	// Phase 1: per-client global configs. Collect known global paths
	// to skip during the walk.
	skipPaths := map[string]bool{}
	for _, c := range allScanners {
		if err := ctx.Err(); err != nil {
			return types.DeviceScanManifest{}, err
		}
		mcps = append(mcps, c.ScanGlobal(s)...)
		for _, p := range c.GlobalConfigPaths() {
			skipPaths[p] = true
		}
	}

	// Phase 2: single project walk against all scanner globs +
	// SKILL.md.
	if err := ctx.Err(); err != nil {
		return types.DeviceScanManifest{}, err
	}
	hits, skillHits := walkProject(ctx, s.fsys, allScanners, maxDepth, skipPaths)

	// Phase 3: dispatch project hits to their owning scanner.
	for _, h := range hits {
		if err := ctx.Err(); err != nil {
			return types.DeviceScanManifest{}, err
		}
		mcps = append(mcps, h.scanner.ScanProject(s, h.path)...)
	}

	// Phase 4: per-client plugin scans (only scanners that also
	// implement PluginScanner).
	for _, c := range allScanners {
		pc, ok := c.(PluginScanner)
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return types.DeviceScanManifest{}, err
		}
		ps, ms, sks := pc.ScanPlugins(s)
		plugins = append(plugins, ps...)
		mcps = append(mcps, ms...)
		skills = append(skills, sks...)
	}

	// Phase 5: skills (global dirs first, then walk hits).
	if err := ctx.Err(); err != nil {
		return types.DeviceScanManifest{}, err
	}
	skills = append(skills, scanGlobalSkills(s)...)
	skills = append(skills, scanProjectSkills(s, skillHits)...)

	// Phase 6: client presence detection (uses real OS access for
	// $PATH and absolute paths like /Applications).
	if err := ctx.Err(); err != nil {
		return types.DeviceScanManifest{}, err
	}
	scanClientPresence(s, homeAbs)

	// Phase 7: assemble.
	return build(s, mcps, skills, plugins), nil
}

// build flattens the accumulated state and observation slices into a
// submission manifest. Files are path-sorted, clients are name-sorted;
// observations stay in the order they were emitted.
//
// Synthesises a clients[] entry for any client name referenced by an
// observation that doesn't already have a presence-detected row, except
// for the multiClient sentinel.
func build(s *scanState, mcps []types.DeviceScanMCPServer, skills []types.DeviceScanSkill, plugins []types.DeviceScanPlugin) types.DeviceScanManifest {
	out := types.DeviceScanManifest{
		Files:      make([]types.DeviceScanFile, 0, len(s.files)),
		Clients:    make([]types.DeviceScanClient, 0, len(s.clients)),
		MCPServers: mcps,
		Skills:     skills,
		Plugins:    plugins,
	}

	paths := make([]string, 0, len(s.files))
	for p := range s.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		out.Files = append(out.Files, s.files[p])
	}

	hasMCP := map[string]bool{}
	hasSkill := map[string]bool{}
	hasPlugin := map[string]bool{}
	for _, m := range out.MCPServers {
		if m.Client != "" {
			hasMCP[m.Client] = true
		}
	}
	for _, sk := range out.Skills {
		if sk.Client != "" {
			hasSkill[sk.Client] = true
		}
	}
	for _, p := range out.Plugins {
		if p.Client != "" {
			hasPlugin[p.Client] = true
		}
	}
	for _, set := range []map[string]bool{hasMCP, hasSkill, hasPlugin} {
		for n := range set {
			if n == multiClient {
				continue
			}
			if _, ok := s.clients[n]; !ok {
				s.clients[n] = types.DeviceScanClient{Name: n}
			}
		}
	}

	names := make([]string, 0, len(s.clients))
	for n := range s.clients {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		c := s.clients[n]
		c.HasMCPServers = hasMCP[n]
		c.HasSkills = hasSkill[n]
		c.HasPlugins = hasPlugin[n]
		out.Clients = append(out.Clients, c)
	}
	return out
}
