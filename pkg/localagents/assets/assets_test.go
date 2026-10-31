package assets

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderAgentSkillsForClaudeCode(t *testing.T) {
	rendered := renderAgentSkillsForTest(t, adminTemplateData(ClaudeCodeTemplateData()))

	if len(rendered) != 5 {
		t.Fatalf("expected 5 rendered assets, got %d", len(rendered))
	}

	install := renderedByName(t, rendered, "obot-install-skill")
	assertContains(t, string(install.Content), "obot skills install --non-interactive --destination ~/.claude/skills <skill>")
	assertNotContains(t, string(install.Content), "--json")

	search := renderedByName(t, rendered, "obot-search-skills")
	assertContains(t, string(search.Content), "obot skills search --non-interactive \"<query>\"")
	assertContains(t, string(search.Content), "obot skills search --non-interactive\n")

	mcpSearch := renderedByName(t, rendered, "obot-search-mcp-servers")
	assertContains(t, string(mcpSearch.Content), "Search the configured Obot server for available MCP servers.")
	assertContains(t, string(mcpSearch.Content), "obot mcp search --non-interactive \"<query>\"")
	assertContains(t, string(mcpSearch.Content), "obot mcp search --non-interactive\n")
	assertContains(t, string(mcpSearch.Content), "configuration required")

	authorVMCPs := renderedByName(t, rendered, "obot-author-vmcps")
	assertContains(t, string(authorVMCPs.Content), "name: obot-author-vmcps")
	assertContains(t, string(authorVMCPs.Content), "obot mcp list-tools <entry-id> --non-interactive")

	bootstrap := renderedByName(t, rendered, "obot")
	assertContains(t, string(bootstrap.Content), "rendered for `claude-code`")
	assertContains(t, string(bootstrap.Content), "load the `obot-author-vmcps` skill")
	assertContains(t, string(bootstrap.Content), "obot mcp search")
}

func TestRenderAgentSkillsForSharedAgents(t *testing.T) {
	rendered := renderAgentSkillsForTest(t, SharedAgentsTemplateData())

	install := renderedByName(t, rendered, "obot-install-skill")
	assertContains(t, string(install.Content), "obot skills install --non-interactive --destination ~/.agents/skills <skill>")

	bootstrap := renderedByName(t, rendered, "obot")
	assertContains(t, string(bootstrap.Content), "rendered for `agents`")
	assertContains(t, string(bootstrap.Content), "obot mcp search")
}

func TestRenderedAssetsHaveDeterministicRelativePaths(t *testing.T) {
	rendered := renderAgentSkillsForTest(t, adminTemplateData(ClaudeCodeTemplateData()))

	got := make([]string, 0, len(rendered))
	for _, asset := range rendered {
		got = append(got, asset.RelPath)
		if filepath.IsAbs(asset.RelPath) {
			t.Fatalf("asset path should be relative: %s", asset.RelPath)
		}
	}

	want := []string{
		"obot-author-vmcps/SKILL.md",
		"obot-install-skill/SKILL.md",
		"obot-search-mcp-servers/SKILL.md",
		"obot-search-skills/SKILL.md",
		"obot/SKILL.md",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected paths:\n%s", strings.Join(got, "\n"))
	}
}

func TestRenderAgentSkillsRejectsIncompleteTemplateData(t *testing.T) {
	tests := []TemplateData{
		{
			InstallDestination: "~/.claude/skills",
		},
		{
			AgentID: "claude-code",
		},
	}

	for _, data := range tests {
		if _, err := RenderAgentSkills(data); err == nil {
			t.Fatalf("expected error for data %#v", data)
		}
	}
}

func TestRenderedTemplatesDoNotContainUnexpandedActions(t *testing.T) {
	rendered := renderAgentSkillsForTest(t, ClaudeCodeTemplateData())
	for _, asset := range rendered {
		content := string(asset.Content)
		assertNotContains(t, content, "{{")
		assertNotContains(t, content, "}}")
	}
}

// Admins and owners get the admin skills too.
func TestRenderAgentSkillsLeavesOutAdminSkillsForOthers(t *testing.T) {
	rendered := renderAgentSkillsForTest(t, ClaudeCodeTemplateData())
	if len(rendered) != 4 {
		t.Fatalf("expected 4 rendered assets, got %d", len(rendered))
	}
	for _, asset := range rendered {
		if asset.SkillName == "obot-author-vmcps" {
			t.Fatal("obot-author-vmcps rendered for a non-admin")
		}
	}
	assertNotContains(t, string(renderedByName(t, rendered, "obot").Content), "obot-author-vmcps")
}

func adminTemplateData(data TemplateData) TemplateData {
	data.Admin = true
	return data
}

func renderAgentSkillsForTest(t *testing.T, data TemplateData) []SkillAsset {
	t.Helper()

	rendered, err := RenderAgentSkills(data)
	if err != nil {
		t.Fatal(err)
	}
	return rendered
}

func renderedByName(t *testing.T, rendered []SkillAsset, skillName string) SkillAsset {
	t.Helper()

	for _, asset := range rendered {
		if asset.SkillName == skillName {
			return asset
		}
	}
	t.Fatalf("missing rendered skill %s", skillName)
	return SkillAsset{}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Fatalf("expected content to contain %q:\n%s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Fatalf("expected content not to contain %q:\n%s", substr, s)
	}
}
