package hostedagentrefs

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func clientWith(objs ...kclient.Object) kclient.Client {
	return fakeclient.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		// The skill lookup reads by path, which needs the same index the real
		// storage provides.
		WithIndex(&v1.Skill{}, "spec.relativePath", func(o kclient.Object) []string {
			return []string{o.(*v1.Skill).Spec.RelativePath}
		}).
		WithObjects(objs...).
		Build()
}

func entry(name, sourceURL, entryKey string) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		Name: name, Namespace: "obot",
		Spec: v1.MCPServerCatalogEntrySpec{
			SourceURL: sourceURL,
			Manifest:  types.MCPServerCatalogEntryManifest{EntryKey: entryKey},
		},
	}
}

func skill(name, repoURL, relPath string) *v1.Skill {
	return &v1.Skill{
		Name: name, Namespace: "obot",
		Spec: v1.SkillSpec{RepoURL: repoURL, RelativePath: relPath},
	}
}

// The point of a reference: a template names something by its source and key,
// which is the same on every installation, and resolution finds whatever ID
// this one generated.
func TestMCPReferenceResolvesToTheLocalID(t *testing.T) {
	client := clientWith(entry("default-atlassian-ad9a4f19",
		"https://github.com/obot-platform/mcp-catalog", "obot-atlassian"))

	got, err := ResolveMCP(context.Background(), client, "obot",
		"github.com/obot-platform/mcp-catalog"+Separator+"obot-atlassian")
	if err != nil {
		t.Fatalf("ResolveMCP: %v", err)
	}
	if got != "default-atlassian-ad9a4f19" {
		t.Errorf("resolved to %q", got)
	}
}

// An entry key is unique only within its source, so the source half decides.
func TestMCPReferenceIsScopedToItsSource(t *testing.T) {
	client := clientWith(
		entry("a-atlassian", "https://github.com/obot-platform/mcp-catalog", "obot-atlassian"),
		entry("b-atlassian", "https://example.com/other-catalog", "obot-atlassian"),
	)

	got, err := ResolveMCP(context.Background(), client, "obot",
		"example.com/other-catalog"+Separator+"obot-atlassian")
	if err != nil {
		t.Fatalf("ResolveMCP: %v", err)
	}
	if got != "b-atlassian" {
		t.Errorf("resolved to %q, want the entry from the named source", got)
	}
}

// A bare ID is not a reference and must pass through, so templates and agents
// that name IDs directly keep working.
func TestPlainIDsPassThrough(t *testing.T) {
	client := clientWith()
	for _, id := range []string{"ms1abc", "default-atlassian-ad9a4f19", "sk1-default-doc-1234"} {
		if got, err := ResolveMCP(context.Background(), client, "obot", id); err != nil || got != id {
			t.Errorf("ResolveMCP(%q) = %q, %v; want it untouched", id, got, err)
		}
		if got, err := ResolveSkill(context.Background(), client, "obot", id); err != nil || got != id {
			t.Errorf("ResolveSkill(%q) = %q, %v; want it untouched", id, got, err)
		}
	}
}

func TestSkillReferenceResolvesByPath(t *testing.T) {
	client := clientWith(
		skill("sk1-default-doc-167641d7", "https://github.com/obot-platform/skills", "skills/doc"),
		skill("sk1-other-doc-99999999", "https://example.com/other-skills", "skills/doc"),
	)

	got, err := ResolveSkill(context.Background(), client, "obot",
		"github.com/obot-platform/skills"+Separator+"skills/doc")
	if err != nil {
		t.Fatalf("ResolveSkill: %v", err)
	}
	if got != "sk1-default-doc-167641d7" {
		t.Errorf("resolved to %q, want the skill from the named repository", got)
	}
}

// The scheme is not part of a source's identity, so a reference written either
// way resolves.
func TestSourceSchemeIsIgnored(t *testing.T) {
	client := clientWith(skill("sk1doc", "https://github.com/obot-platform/skills", "skills/doc"))

	for _, ref := range []string{
		"github.com/obot-platform/skills" + Separator + "skills/doc",
		"https://github.com/obot-platform/skills" + Separator + "skills/doc",
	} {
		if _, err := ResolveSkill(context.Background(), client, "obot", ref); err != nil {
			t.Errorf("ResolveSkill(%q): %v", ref, err)
		}
	}
}

// A reference naming nothing is an error rather than a silent miss, so the
// caller can say which reference failed.
func TestUnresolvableReferencesAreReported(t *testing.T) {
	client := clientWith()

	if _, err := ResolveMCP(context.Background(), client, "obot", "some.catalog"+Separator+"nope"); err == nil {
		t.Error("expected an error for an MCP reference naming nothing")
	}
	if _, err := ResolveSkill(context.Background(), client, "obot", "some.repo"+Separator+"nope"); err == nil {
		t.Error("expected an error for a skill reference naming nothing")
	}
	if _, err := ResolveMCP(context.Background(), client, "obot", "half"+Separator); err == nil {
		t.Error("expected an error for a malformed reference")
	}
}

// The resolver keeps what resolved and reports what did not, so an agent gets
// what it can have while the rest is explained.
func TestResolverSeparatesResolvedFromMissing(t *testing.T) {
	client := clientWith(entry("here", "https://cat.example.com", "present"))
	resolver := New(client, "obot")

	resolved, unresolved := resolver.MCPServers(context.Background(), []string{
		"cat.example.com" + Separator + "present",
		"cat.example.com" + Separator + "absent",
		"ms1literal",
	})
	if len(resolved) != 2 || resolved[0] != "here" || resolved[1] != "ms1literal" {
		t.Errorf("resolved = %v", resolved)
	}
	if len(unresolved) != 1 || unresolved[0] != "cat.example.com"+Separator+"absent" {
		t.Errorf("unresolved = %v", unresolved)
	}
}
