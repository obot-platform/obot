package assets

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderClaudeSkillsForClaudeCode(t *testing.T) {
	rendered := renderClaudeSkillsForTest(t, ClaudeCodeTemplateData())

	if len(rendered) != 4 {
		t.Fatalf("expected 4 rendered assets, got %d", len(rendered))
	}

	install := renderedByName(t, rendered, "obot-install-skill")
	assertContains(t, string(install.Content), "obot skills install --agent claude-code <skill>")
	assertNotContains(t, string(install.Content), "--json")

	search := renderedByName(t, rendered, "obot-search-skills")
	assertContains(t, string(search.Content), "obot skills search <query>")

	scan := renderedByName(t, rendered, "obot-scan")
	assertContains(t, string(scan.Content), "obot scan")

	bootstrap := renderedByName(t, rendered, "obot")
	assertContains(t, string(bootstrap.Content), "rendered for `claude-code`")
}

func TestRenderClaudeSkillsForCursor(t *testing.T) {
	rendered := renderClaudeSkillsForTest(t, CursorTemplateData())

	install := renderedByName(t, rendered, "obot-install-skill")
	assertContains(t, string(install.Content), "obot skills install --agent cursor <skill>")

	bootstrap := renderedByName(t, rendered, "obot")
	assertContains(t, string(bootstrap.Content), "rendered for `cursor`")
}

func TestRenderedAssetsHaveDeterministicRelativePaths(t *testing.T) {
	rendered := renderClaudeSkillsForTest(t, ClaudeCodeTemplateData())

	got := make([]string, 0, len(rendered))
	for _, asset := range rendered {
		got = append(got, asset.RelPath)
		if filepath.IsAbs(asset.RelPath) {
			t.Fatalf("asset path should be relative: %s", asset.RelPath)
		}
	}

	want := []string{
		"obot-install-skill/SKILL.md",
		"obot-scan/SKILL.md",
		"obot-search-skills/SKILL.md",
		"obot/SKILL.md",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected paths:\n%s", strings.Join(got, "\n"))
	}
}

func TestRenderClaudeSkillsRejectsIncompleteTemplateData(t *testing.T) {
	tests := []TemplateData{
		{InstallAgentArg: "claude-code"},
		{AgentID: "claude-code"},
	}

	for _, data := range tests {
		if _, err := RenderAgentSkills(data); err == nil {
			t.Fatalf("expected error for data %#v", data)
		}
	}
}

func TestRenderedTemplatesDoNotContainUnexpandedActions(t *testing.T) {
	rendered := renderClaudeSkillsForTest(t, ClaudeCodeTemplateData())
	for _, asset := range rendered {
		content := string(asset.Content)
		assertNotContains(t, content, "{{")
		assertNotContains(t, content, "}}")
	}
}

func renderClaudeSkillsForTest(t *testing.T, data TemplateData) []SkillAsset {
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
