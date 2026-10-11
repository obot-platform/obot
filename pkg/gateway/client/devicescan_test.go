package client

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func insertScan(t *testing.T, c *Client, scan types.DeviceScan) types.DeviceScan {
	t.Helper()
	if err := c.db.WithContext(t.Context()).Create(&scan).Error; err != nil {
		t.Fatalf("failed to insert scan: %v", err)
	}
	return scan
}

// TestGetDeviceScanStats exercises the dashboard rollup: device_count
// from the latest-scan-per-device subset, ConfigHash dedup, name dedup
// for clients/skills, and time-window bounding.
func TestGetDeviceScanStats(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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
	insertScan(t, c, types.DeviceScan{
		SubmittedBy: "charlie", DeviceID: "dev-3", ScannedAt: now.Add(2 * time.Hour),
		Clients: []types.DeviceScanClient{{Name: "cursor"}},
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
	if list2[0].Name != "codex" {
		t.Errorf("default sort page 2: want codex, got %q", list2[0].Name)
	}

	byUsers, _, err := c.ListDeviceClientFleetSummaries(ctx, DeviceClientFleetListOptions{SortBy: "user_count", SortOrder: "desc", Limit: 50})
	if err != nil {
		t.Fatalf("list sort user_count: %v", err)
	}
	if len(byUsers) == 0 || byUsers[0].Name != "cursor" {
		t.Errorf("sort user_count desc: want cursor first, got %+v", byUsers)
	}

	bySkills, _, err := c.ListDeviceClientFleetSummaries(ctx, DeviceClientFleetListOptions{SortBy: "skill_count", SortOrder: "desc", Limit: 50})
	if err != nil {
		t.Fatalf("list sort skill_count: %v", err)
	}
	if len(bySkills) < 3 || bySkills[0].Name != "claude-code" || bySkills[1].Name != "codex" || bySkills[2].Name != "cursor" {
		t.Errorf("sort skill_count desc: got %+v", bySkills)
	}

	byMCPServers, _, err := c.ListDeviceClientFleetSummaries(ctx, DeviceClientFleetListOptions{SortBy: "mcp_server_count", SortOrder: "desc", Limit: 50})
	if err != nil {
		t.Fatalf("list sort mcp_server_count: %v", err)
	}
	if len(byMCPServers) == 0 || byMCPServers[0].Name != "claude-code" {
		t.Errorf("sort mcp_server_count desc: want claude-code first, got %+v", byMCPServers)
	}
}

// TestListDeviceScans_SortAndSearch exercises obot-platform/obot#6739: sort
// and search must cover the full result set, not just whatever page the
// caller asked for, and "user" has to match the same displayName -> email
// -> username -> id -> device-scan-username precedence Devices.svelte uses
// to render the User column.
func TestListDeviceScans_SortAndSearch(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	alice := types.User{Username: "asmith", HashedUsername: "asmith", DisplayName: "Alice Smith", Email: "alice@example.com"}
	if err := c.db.WithContext(ctx).Create(&alice).Error; err != nil {
		t.Fatalf("failed to create user alice: %v", err)
	}
	// No DisplayName, so her effective display falls through to Email.
	bob := types.User{Username: "bsmith", HashedUsername: "bsmith", Email: "bob@example.com"}
	if err := c.db.WithContext(ctx).Create(&bob).Error; err != nil {
		t.Fatalf("failed to create user bob: %v", err)
	}

	now := time.Now().UTC()
	// Inserted oldest-first so the historical "created_at DESC, page 1"
	// window would put aaa-1 and bbb-2 on the LAST page, not the first --
	// exactly the rows a naive current-page-only search would miss.
	insertScan(t, c, types.DeviceScan{SubmittedBy: idStr(alice.ID), DeviceID: "aaa-1", Hostname: "host-a", OS: "linux", Arch: "amd64", ScannedAt: now})
	insertScan(t, c, types.DeviceScan{SubmittedBy: idStr(bob.ID), DeviceID: "bbb-2", Hostname: "host-b", OS: "darwin", Arch: "arm64", ScannedAt: now.Add(time.Minute)})
	// Device-only scan: empty SubmittedBy exercises the LEFT JOIN miss and
	// the final fallback to the scan's own OS-level Username.
	insertScan(t, c, types.DeviceScan{DeviceID: "ccc-3", Hostname: "host-c", OS: "windows", Arch: "amd64", Username: "device-os-user", ScannedAt: now.Add(2 * time.Minute)})
	insertScan(t, c, types.DeviceScan{SubmittedBy: idStr(alice.ID), DeviceID: "ddd-4", Hostname: "host-d", OS: "linux", Arch: "arm64", ScannedAt: now.Add(3 * time.Minute)})
	insertScan(t, c, types.DeviceScan{SubmittedBy: idStr(bob.ID), DeviceID: "eee-5", Hostname: "host-e", OS: "darwin", Arch: "amd64", ScannedAt: now.Add(4 * time.Minute)})

	// Total must reflect every matching row regardless of Limit.
	page1, total, err := c.ListDeviceScans(ctx, DeviceScanListOptions{GroupByDevice: true, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("list page1: %v", err)
	}
	if total != 5 {
		t.Fatalf("total: want 5, got %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len: want 2, got %d", len(page1))
	}

	// Sorting by device_id must hold across page boundaries, not just
	// within whichever page happened to be requested.
	assertDeviceIDPage(t, c, "device_id", "asc", 0, []string{"aaa-1", "bbb-2"})
	assertDeviceIDPage(t, c, "device_id", "asc", 2, []string{"ccc-3", "ddd-4"})
	assertDeviceIDPage(t, c, "device_id", "asc", 4, []string{"eee-5"})

	// Searching for a device that would sit on the LAST page under the old
	// unconditional created_at DESC order must still be found.
	searched, searchTotal, err := c.ListDeviceScans(ctx, DeviceScanListOptions{GroupByDevice: true, Limit: 2, Search: "aaa"})
	if err != nil {
		t.Fatalf("search device id: %v", err)
	}
	if searchTotal != 1 || len(searched) != 1 || searched[0].DeviceID != "aaa-1" {
		t.Fatalf("search %q: want exactly aaa-1, got total=%d rows=%+v", "aaa", searchTotal, searched)
	}

	// Search matches the User column's displayName.
	byDisplayName, _, err := c.ListDeviceScans(ctx, DeviceScanListOptions{GroupByDevice: true, Limit: 10, Search: "Alice"})
	if err != nil {
		t.Fatalf("search displayName: %v", err)
	}
	if ids := deviceIDs(byDisplayName); !sameSet(ids, []string{"aaa-1", "ddd-4"}) {
		t.Errorf("search Alice: want [aaa-1 ddd-4], got %v", ids)
	}

	// Search matches the User column's email fallback (bob has no DisplayName).
	byEmail, _, err := c.ListDeviceScans(ctx, DeviceScanListOptions{GroupByDevice: true, Limit: 10, Search: "bob@example"})
	if err != nil {
		t.Fatalf("search email: %v", err)
	}
	if ids := deviceIDs(byEmail); !sameSet(ids, []string{"bbb-2", "eee-5"}) {
		t.Errorf("search bob@example: want [bbb-2 eee-5], got %v", ids)
	}

	// Search matches the device-only scan's OS-level username fallback,
	// proving the LEFT JOIN miss (empty SubmittedBy) doesn't break search.
	byOSUsername, _, err := c.ListDeviceScans(ctx, DeviceScanListOptions{GroupByDevice: true, Limit: 10, Search: "device-os-user"})
	if err != nil {
		t.Fatalf("search os username: %v", err)
	}
	if ids := deviceIDs(byOSUsername); !sameSet(ids, []string{"ccc-3"}) {
		t.Errorf("search device-os-user: want [ccc-3], got %v", ids)
	}

	// Sorting by "user" follows the same precedence: "Alice Smith" <
	// "bob@example.com" < "device-os-user".
	byUser, _, err := c.ListDeviceScans(ctx, DeviceScanListOptions{GroupByDevice: true, Limit: 10, SortBy: "user", SortOrder: "asc"})
	if err != nil {
		t.Fatalf("sort by user: %v", err)
	}
	if got := deviceIDs(byUser); !equalOrdered(got, []string{"aaa-1", "ddd-4", "bbb-2", "eee-5", "ccc-3"}) {
		t.Errorf("sort by user asc: got %v", got)
	}
}

func idStr(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func deviceIDs(scans []types.DeviceScan) []string {
	ids := make([]string, len(scans))
	for i, s := range scans {
		ids[i] = s.DeviceID
	}
	return ids
}

func equalOrdered(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(want))
	for _, w := range want {
		seen[w] = true
	}
	for _, g := range got {
		if !seen[g] {
			return false
		}
	}
	return true
}

func assertDeviceIDPage(t *testing.T, c *Client, sortBy, sortOrder string, offset int, want []string) {
	t.Helper()
	scans, _, err := c.ListDeviceScans(t.Context(), DeviceScanListOptions{
		GroupByDevice: true,
		Limit:         2,
		Offset:        offset,
		SortBy:        sortBy,
		SortOrder:     sortOrder,
	})
	if err != nil {
		t.Fatalf("list sorted page at offset %d: %v", offset, err)
	}
	if got := deviceIDs(scans); !equalOrdered(got, want) {
		t.Errorf("page at offset %d: want %v, got %v", offset, want, got)
	}
}
