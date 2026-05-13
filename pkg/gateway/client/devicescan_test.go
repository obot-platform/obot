package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func insertScan(t *testing.T, c *Client, scan types.DeviceScan) types.DeviceScan {
	t.Helper()
	if err := c.db.WithContext(context.Background()).Create(&scan).Error; err != nil {
		t.Fatalf("failed to insert scan: %v", err)
	}
	return scan
}

// TestGetDeviceScanStats exercises the dashboard rollup: device_count
// from the latest-scan-per-device subset, ConfigHash dedup, name dedup
// for clients/skills, and time-window bounding.
func TestGetDeviceScanStats(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	now := time.Now().UTC()
	old := now.Add(-180 * 24 * time.Hour)

	sharedHash := "hash-shared"
	uniqueHash := "hash-unique"
	stale := "hash-stale"

	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "user-a", DeviceID: "device-a", ScannedAt: now.Add(-1 * time.Hour),
		MCPServers: []types.DeviceScanMCPServer{
			{Client: "claude-code", Scope: "global", Name: "shared", Transport: "stdio", ConfigHash: sharedHash, Args: datatypes.JSONSlice[string]{"x"}},
			{Client: "claude-code", Scope: "global", Name: "unique", Transport: "stdio", ConfigHash: uniqueHash},
		},
		Skills: []types.DeviceScanSkill{
			{Client: "claude-code", Name: "brainstorming"},
			{Client: "claude-code", Name: "diagnose"},
		},
		Clients: []types.DeviceScanClient{
			{Name: "claude-code", BinaryPath: "/usr/local/bin/claude", HasMCPServers: true, HasSkills: true},
			{Name: "cursor", InstallPath: "/Applications/Cursor.app"},
		},
	})
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "user-b", DeviceID: "device-b", ScannedAt: now.Add(-2 * time.Hour),
		MCPServers: []types.DeviceScanMCPServer{
			{Client: "codex", Scope: "global", Name: "shared", Transport: "stdio", ConfigHash: sharedHash},
		},
		Skills: []types.DeviceScanSkill{
			{Client: "codex", Name: "brainstorming"},
		},
		Clients: []types.DeviceScanClient{
			{Name: "claude-code", BinaryPath: "/usr/local/bin/claude"},
			{Name: "codex", BinaryPath: "/usr/local/bin/codex"},
		},
	})
	// Stale device — only old scans, drops out under window-then-latest.
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "user-c", DeviceID: "device-c", ScannedAt: old,
		MCPServers: []types.DeviceScanMCPServer{
			{Client: "claude-code", Scope: "global", Name: "stale", Transport: "stdio", ConfigHash: stale},
		},
		Skills:  []types.DeviceScanSkill{{Client: "claude-code", Name: "ancient"}},
		Clients: []types.DeviceScanClient{{Name: "windsurf"}},
	})

	stats, err := c.GetDeviceScanStats(ctx, DeviceScanStatsOptions{
		StartTime: now.Add(-30 * 24 * time.Hour),
		EndTime:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("stats failed: %v", err)
	}

	// device_count = number of distinct devices with a scan in window.
	// device-c's only scan is outside the window, so 2 not 3.
	if stats.DeviceCount != 2 {
		t.Errorf("device_count: want 2, got %d", stats.DeviceCount)
	}
	// user_count = distinct submitters across the in-window subset.
	// user-a + user-b inside window; user-c is window-stale.
	if stats.UserCount != 2 {
		t.Errorf("user_count: want 2, got %d", stats.UserCount)
	}

	// MCP servers — shared hash hits both devices, unique hits one,
	// stale dropped by window.
	mcpByHash := map[string]types.MCPServerStat{}
	for _, r := range stats.MCPServers {
		mcpByHash[r.ConfigHash] = r
	}
	if len(stats.MCPServers) != 2 {
		t.Errorf("mcp servers: want 2 rows, got %d (%+v)", len(stats.MCPServers), stats.MCPServers)
	}
	if got := mcpByHash[sharedHash].DeviceCount; got != 2 {
		t.Errorf("shared hash device_count: want 2, got %d", got)
	}
	if got := mcpByHash[uniqueHash].DeviceCount; got != 1 {
		t.Errorf("unique hash device_count: want 1, got %d", got)
	}
	if _, ok := mcpByHash[stale]; ok {
		t.Errorf("stale hash should be excluded by window filter")
	}
	// Default sort: device_count DESC, so shared comes first.
	if stats.MCPServers[0].ConfigHash != sharedHash {
		t.Errorf("mcp default sort: want shared first, got %q", stats.MCPServers[0].ConfigHash)
	}

	// Clients — claude-code on both devices, cursor on one, codex on
	// one, windsurf dropped by window.
	clientByName := map[string]types.ClientStat{}
	for _, r := range stats.Clients {
		clientByName[r.Name] = r
	}
	if got := clientByName["claude-code"].DeviceCount; got != 2 {
		t.Errorf("claude-code client device_count: want 2, got %d", got)
	}
	if got := clientByName["cursor"].DeviceCount; got != 1 {
		t.Errorf("cursor client device_count: want 1, got %d", got)
	}
	if _, ok := clientByName["windsurf"]; ok {
		t.Errorf("windsurf should be excluded by window filter")
	}

	// Skills grouped strictly by name. brainstorming hits both
	// devices (different client owners — collapse), diagnose only
	// device-a, ancient is window-stale.
	skillByName := map[string]types.SkillStat{}
	for _, r := range stats.Skills {
		skillByName[r.Name] = r
	}
	if got := skillByName["brainstorming"].DeviceCount; got != 2 {
		t.Errorf("brainstorming device_count: want 2 (collapsed across owners), got %d", got)
	}
	if got := skillByName["diagnose"].DeviceCount; got != 1 {
		t.Errorf("diagnose device_count: want 1, got %d", got)
	}
	if _, ok := skillByName["ancient"]; ok {
		t.Errorf("ancient skill should be excluded by window filter")
	}
}

// TestGetDeviceScanStats_LatestScanWins verifies a newer scan that
// drops a previously-seen entity removes that device's contribution.
func TestGetDeviceScanStats_LatestScanWins(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	now := time.Now().UTC()
	hash := "hash-changing"

	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "user-a", DeviceID: "device-a", ScannedAt: now.Add(-2 * time.Hour),
		MCPServers: []types.DeviceScanMCPServer{
			{Client: "claude-code", Name: "x", Transport: "stdio", ConfigHash: hash},
		},
		Skills:  []types.DeviceScanSkill{{Client: "claude-code", Name: "diagnose"}},
		Clients: []types.DeviceScanClient{{Name: "claude-code"}},
	})
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "user-b", DeviceID: "device-b", ScannedAt: now.Add(-2 * time.Hour),
		MCPServers: []types.DeviceScanMCPServer{
			{Client: "claude-code", Name: "x", Transport: "stdio", ConfigHash: hash},
		},
		Skills:  []types.DeviceScanSkill{{Client: "claude-code", Name: "diagnose"}},
		Clients: []types.DeviceScanClient{{Name: "claude-code"}},
	})
	// device-a's newer scan drops the MCP, the skill, and even the
	// client row.
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "user-a", DeviceID: "device-a", ScannedAt: now.Add(-1 * time.Hour),
	})

	stats, err := c.GetDeviceScanStats(ctx, DeviceScanStatsOptions{})
	if err != nil {
		t.Fatalf("stats failed: %v", err)
	}
	if stats.DeviceCount != 2 {
		t.Errorf("device_count: want 2, got %d", stats.DeviceCount)
	}
	if len(stats.MCPServers) != 1 || stats.MCPServers[0].DeviceCount != 1 {
		t.Errorf("after device-a re-scan: want 1 mcp row with device_count=1, got %+v", stats.MCPServers)
	}
	if len(stats.Skills) != 1 || stats.Skills[0].DeviceCount != 1 {
		t.Errorf("after device-a re-scan: want 1 skill row with device_count=1, got %+v", stats.Skills)
	}
	if len(stats.Clients) != 1 || stats.Clients[0].DeviceCount != 1 {
		t.Errorf("after device-a re-scan: want 1 client row with device_count=1, got %+v", stats.Clients)
	}
}

// TestGetMCPServerDetail verifies single-hash detail load and that
// EnvKeys / HeaderKeys are unioned across observations.
func TestGetMCPServerDetail(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	now := time.Now().UTC()
	hash := "h"
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "u1", DeviceID: "d1", ScannedAt: now,
		MCPServers: []types.DeviceScanMCPServer{{
			Client: "claude-code", Name: "x", Transport: "stdio", ConfigHash: hash,
			Args:    datatypes.JSONSlice[string]{"--flag"},
			EnvKeys: datatypes.JSONSlice[string]{"FOO", "BAR"},
		}},
	})
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "u2", DeviceID: "d2", ScannedAt: now,
		MCPServers: []types.DeviceScanMCPServer{{
			Client: "codex", Name: "x", Transport: "stdio", ConfigHash: hash,
			Args:       datatypes.JSONSlice[string]{"--flag"},
			EnvKeys:    datatypes.JSONSlice[string]{"FOO", "BAZ"},
			HeaderKeys: datatypes.JSONSlice[string]{"X-Auth"},
		}},
	})

	d, err := c.GetMCPServerDetail(ctx, hash)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if d.DeviceCount != 2 {
		t.Errorf("device_count: want 2, got %d", d.DeviceCount)
	}
	if len(d.Args) != 1 || d.Args[0] != "--flag" {
		t.Errorf("args: want [--flag], got %v", d.Args)
	}
	envSet := map[string]bool{}
	for _, k := range d.EnvKeys {
		envSet[k] = true
	}
	for _, want := range []string{"FOO", "BAR", "BAZ"} {
		if !envSet[want] {
			t.Errorf("env keys missing %q: got %v", want, d.EnvKeys)
		}
	}
	if len(d.HeaderKeys) != 1 || d.HeaderKeys[0] != "X-Auth" {
		t.Errorf("header keys: want [X-Auth], got %v", d.HeaderKeys)
	}
}

func TestDeviceClientFleetSummaries(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	now := time.Now().UTC()
	h1 := "hash-one"
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "alice", DeviceID: "dev-1", ScannedAt: now,
		Clients: []types.DeviceScanClient{
			{Name: "claude-code"},
			{Name: "cursor"},
		},
		Skills: []types.DeviceScanSkill{
			{
				Client: "claude-code", Name: "skill-a",
				Description: "Skill A summary", HasScripts: true,
				Files: datatypes.JSONSlice[string]{"SKILL.md", "run.sh"},
			},
			{Client: "multi", Name: "floating"},
		},
		MCPServers: []types.DeviceScanMCPServer{
			{Client: "claude-code", Name: "mcp1", Transport: "stdio", ConfigHash: h1, Args: datatypes.JSONSlice[string]{"a"}},
		},
	})
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "bob", DeviceID: "dev-2", ScannedAt: now.Add(time.Hour),
		Clients: []types.DeviceScanClient{{Name: "codex"}},
		Skills: []types.DeviceScanSkill{{
			Client: "codex", Name: "skill-b", Description: "B", HasScripts: false,
			Files: datatypes.JSONSlice[string]{"SKILL.md"},
		}},
	})

	list, total, err := c.ListDeviceClientFleetSummaries(ctx, DeviceClientFleetListOptions{Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Errorf("total distinct clients: want 3, got %d", total)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 rows, got %d", len(list))
	}

	filtered, fTotal, err := c.ListDeviceClientFleetSummaries(ctx, DeviceClientFleetListOptions{Name: "claude", Limit: 50})
	if err != nil {
		t.Fatalf("list name=claude: %v", err)
	}
	if fTotal != 1 || len(filtered) != 1 || filtered[0].Name != "claude-code" {
		t.Errorf("name filter claude: want total=1 row=claude-code, got total=%d rows=%+v", fTotal, filtered)
	}

	codeMatches, codeTotal, err := c.ListDeviceClientFleetSummaries(ctx, DeviceClientFleetListOptions{Name: "code", Limit: 50})
	if err != nil {
		t.Fatalf("list name=code: %v", err)
	}
	if codeTotal != 2 || len(codeMatches) != 2 {
		t.Errorf("name filter code: want 2 clients (claude-code, codex), got total=%d len=%d", codeTotal, len(codeMatches))
	}

	var cc *DeviceClientFleetSummary
	for i := range list {
		if list[i].Name == "claude-code" {
			cc = &list[i]
			break
		}
	}
	if cc == nil {
		t.Fatal("missing claude-code")
	}
	if len(cc.Users) != 1 || cc.Users[0] != "alice" {
		t.Errorf("claude-code users: %+v", cc.Users)
	}
	if len(cc.Skills) != 1 {
		t.Fatalf("claude-code skills: want 1, got %d", len(cc.Skills))
	}
	if cc.Skills[0].Name != "skill-a" || cc.Skills[0].Description != "Skill A summary" || !cc.Skills[0].HasScripts || cc.Skills[0].Files != 2 {
		t.Errorf("claude-code skills[0]: %+v", cc.Skills[0])
	}
	if len(cc.MCPServers) != 1 || cc.MCPServers[0].ConfigHash != h1 || len(cc.MCPServers[0].Args) != 1 || cc.MCPServers[0].Args[0] != "a" {
		t.Errorf("claude-code mcps: %+v", cc.MCPServers)
	}

	_, err = c.GetDeviceClientFleetSummary(ctx, "nonexistent")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("want ErrRecordNotFound, got %v", err)
	}

	g, err := c.GetDeviceClientFleetSummary(ctx, "codex")
	if err != nil {
		t.Fatalf("get codex: %v", err)
	}
	if len(g.Users) != 1 || g.Users[0] != "bob" || len(g.Skills) != 1 {
		t.Errorf("codex summary: users=%v skills=%v", g.Users, g.Skills)
	}
	if g.Skills[0].Name != "skill-b" || g.Skills[0].Files != 1 || g.Skills[0].HasScripts {
		t.Errorf("codex skills[0]: %+v", g.Skills[0])
	}

	list2, total2, err := c.ListDeviceClientFleetSummaries(ctx, DeviceClientFleetListOptions{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if total2 != 3 {
		t.Errorf("paginated total: want 3, got %d", total2)
	}
	if len(list2) != 1 {
		t.Fatalf("limit=1 offset=1: want 1 row, got %d", len(list2))
	}
}
