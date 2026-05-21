package devicescan

import (
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeDesktopScanSkillsPlugin covers the Cowork skills-plugin
// scan: two snapshots of the same plugin-uuid (older and newer
// manifest.json:lastUpdated) plus an ephemeral session sibling. The
// active snapshot wins; the ephemeral sibling is ignored entirely.
//
// The skills-plugin bundle is intentionally NOT surfaced as a plugin
// row — it's a delivery vehicle for bundled skills, not a plugin in
// the sense of MCP servers/commands/hooks. Only skill rows are
// emitted; real plugins come through scanRpmPlugins.
func TestClaudeDesktopScanSkillsPlugin(t *testing.T) {
	sessionsDir := path.Join(claudeDesktopAppSupportDir, "local-agent-mode-sessions")
	skillsPluginDir := path.Join(sessionsDir, "skills-plugin")
	installA := path.Join(skillsPluginDir, "install-a", "plugin-x")
	installB := path.Join(skillsPluginDir, "install-b", "plugin-x")
	ephemeralDir := path.Join(sessionsDir, "session-uuid", "inner-uuid", "local_abc")

	pluginManifest := `{"name":"anthropic-skills","version":"1.0.0","description":"Bundled skills"}`
	skillNewer := "---\nname: foo\ndescription: newer copy\n---\nbody\n"
	skillOlder := "---\nname: foo\ndescription: older copy\n---\nbody\n"
	skillEphemeral := "---\nname: bad\ndescription: ephemeral\n---\nbody\n"

	scan := runScanFS(t, map[string]string{
		// Newer snapshot (lastUpdated=2000).
		path.Join(installA, ".claude-plugin", "plugin.json"): pluginManifest,
		path.Join(installA, "manifest.json"):                 `{"lastUpdated":2000}`,
		path.Join(installA, "skills", "foo", "SKILL.md"):     skillNewer,
		// Older snapshot of the same plugin-uuid (lastUpdated=1000).
		path.Join(installB, ".claude-plugin", "plugin.json"): pluginManifest,
		path.Join(installB, "manifest.json"):                 `{"lastUpdated":1000}`,
		path.Join(installB, "skills", "foo", "SKILL.md"):     skillOlder,
		// Ephemeral session sandbox — must not produce any rows.
		path.Join(ephemeralDir, ".claude", "skills", "bad", "SKILL.md"): skillEphemeral,
	})

	require.Len(t, scan.Plugins, 0)
	require.Len(t, scan.Skills, 1)

	skill := scan.Skills[0]
	require.Equal(t, "foo", skill.Name)
	require.Equal(t, "newer copy", skill.Description, "active snapshot should win")
}

// TestClaudeDesktopScanRpmPlugin covers Cowork's RPM (server-pushed)
// plugin scan. Fixture mirrors a real install: a plugin_<id>/ with
// .claude-plugin/plugin.json, .mcp.json (HTTP transport), a nested
// skill, and a sibling rpm/manifest.json carrying the marketplace name.
// Plugin name/version/description/author come from plugin.json;
// Marketplace comes from rpm/manifest.json joined by plugin id.
func TestClaudeDesktopScanRpmPlugin(t *testing.T) {
	// The two UUID levels under local-agent-mode-sessions/ are opaque
	// (one is the account/user id, the other the org id, in an order
	// that has varied between Cowork versions). The scanner treats them
	// as opaque, so we use stand-in labels.
	rpmDir := path.Join(claudeDesktopAppSupportDir, "local-agent-mode-sessions", "outerUUID", "innerUUID", "rpm")
	pluginID := "plugin_01XXJmxLXPEhPMmnxmrgntNw"
	installDir := path.Join(rpmDir, pluginID)

	// Note: rpm/manifest.json's plugins[].name is intentionally a decoy —
	// the scanner sources Name from .claude-plugin/plugin.json, and only
	// reads this file for marketplaceName joined by plugin id. If a
	// refactor ever starts pulling Name from here, the assertion below
	// will catch it.
	rpmManifest := `{
		"lastUpdated": 1779337664941,
		"plugins": [{
			"id": "` + pluginID + `",
			"name": "design-from-rpm-manifest",
			"marketplaceId": "marketplace_01QRn9XAjzzeAokB5nPWVMxP",
			"marketplaceName": "knowledge-work-plugins",
			"installedBy": "user"
		}]
	}`
	pluginManifest := `{"name":"design","version":"1.2.0","description":"Design workflows","author":{"name":"Anthropic"}}`
	mcpConfig := `{"mcpServers":{"figma":{"type":"http","url":"https://mcp.figma.com/mcp"}}}`
	skillBody := "---\nname: design-critique\ndescription: Get structured design feedback\n---\nbody\n"

	scan := runScanFS(t, map[string]string{
		path.Join(rpmDir, "manifest.json"):                             rpmManifest,
		path.Join(installDir, ".claude-plugin", "plugin.json"):         pluginManifest,
		path.Join(installDir, ".mcp.json"):                             mcpConfig,
		path.Join(installDir, "skills", "design-critique", "SKILL.md"): skillBody,
	})

	require.Len(t, scan.Plugins, 1)

	plugin := scan.Plugins[0]
	require.Equal(t, "design", plugin.Name, "should come from .claude-plugin/plugin.json, not rpm/manifest.json or opaque dir name")
	require.Equal(t, "1.2.0", plugin.Version)
	require.Equal(t, "Design workflows", plugin.Description)
	require.Equal(t, "Anthropic", plugin.Author)
	require.Equal(t, "knowledge-work-plugins", plugin.Marketplace, "should come from rpm/manifest.json")
	require.True(t, plugin.HasMCPServers)
	require.True(t, plugin.HasSkills)

	require.Len(t, scan.MCPServers, 1)

	server := scan.MCPServers[0]
	require.Equal(t, "figma", server.Name)
	require.Equal(t, "http", server.Transport)
	require.Equal(t, "https://mcp.figma.com/mcp", server.URL)

	require.Len(t, scan.Skills, 1)
	require.Equal(t, "design-critique", scan.Skills[0].Name)
}
