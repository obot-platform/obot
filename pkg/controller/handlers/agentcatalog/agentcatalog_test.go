package agentcatalog

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestBuildFromSource(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "harnesses", "basic"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "reviewer"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "harnesses", "basic", harnessDefinition), []byte(`
name: Basic
description: Test harness
image: ubuntu:24.04
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "reviewer", agentDefinition), []byte(`
name: Reviewer
description: Reviews code
harnessID: harnesses/basic/harness.yaml
questions:
  - key: focus
    type: string
`), 0o600))
	// Other YAML files are not agent-catalog definitions, matching the explicit
	// filename convention while retaining MCP catalog-style recursive loading.
	require.NoError(t, os.WriteFile(filepath.Join(root, "ignored.yaml"), []byte("not: a definition"), 0o600))

	source := &v1.AgentCatalog{
		Name: "as1source", Namespace: "default",
	}
	found, err := buildFromSource(root, source, "abc123")
	require.NoError(t, err)
	require.Len(t, found.Harnesses, 1)
	require.Len(t, found.Agents, 1)

	harness := found.Harnesses[0]
	assert.Equal(t, "Basic", harness.Spec.Manifest.Name)
	assert.Equal(t, "harnesses/basic/harness.yaml", harness.Spec.RelativePath)
	assert.Equal(t, "abc123", harness.Spec.CommitSHA)
	assert.Equal(t, source.Name, harness.Spec.SourceID)

	agent := found.Agents[0]
	assert.Equal(t, "Reviewer", agent.Spec.Manifest.Name)
	assert.Equal(t, "agents/reviewer/agent.yaml", agent.Spec.RelativePath)
	assert.Equal(t, "harnesses/basic/harness.yaml", agent.Spec.Manifest.HarnessID)

	require.NoError(t, resolveHarnessReferences(found.Agents, found.Harnesses))
	assert.Equal(t, harness.Name, agent.Spec.Manifest.HarnessID)
}

func TestBuildFromSourceRejectsInvalidDefinitions(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, harnessDefinition), []byte(`
name: Basic
image: ubuntu:24.04
unknown: value
`), 0o600))

		_, err := buildFromSource(root, &v1.AgentCatalog{
			Name: "as1source",
		}, "abc123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown field "unknown"`)
	})

	t.Run("sensitive value", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, agentDefinition), []byte(`
name: Unsafe
harnessID: hrn1external
env:
  - key: TOKEN
    sensitive: true
    value: do-not-commit
`), 0o600))

		_, err := buildFromSource(root, &v1.AgentCatalog{
			Name: "as1source",
		}, "abc123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be stored in source control")
	})
}

func TestDiscoverDefinitionsSkipsGitAndSymlinks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", agentDefinition), []byte("name: ignored"), 0o600))
	target := filepath.Join(root, "outside.yaml")
	require.NoError(t, os.WriteFile(target, []byte("name: ignored"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(root, harnessDefinition)))

	definitions, err := discoverDefinitions(root)
	require.NoError(t, err)
	assert.Empty(t, definitions)
}

func TestGitSourceFetcherLocalRepository(t *testing.T) {
	root := t.TempDir()
	repository, err := gogit.PlainInit(root, false)
	require.NoError(t, err)
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, harnessDefinition), []byte("name: Basic\nimage: ubuntu:24.04\n"), 0o600))
	_, err = worktree.Add(harnessDefinition)
	require.NoError(t, err)
	commit, err := worktree.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com"},
	})
	require.NoError(t, err)

	fetcher := gitSourceFetcher{}
	for _, repoURL := range []string{root, "file://" + filepath.ToSlash(root)} {
		fetched, err := fetcher.Fetch(t.Context(), repoURL, "")
		require.NoError(t, err)
		assert.Equal(t, root, fetched.RepoRoot)
		assert.Equal(t, commit.String(), fetched.CommitSHA)
		fetched.Cleanup()
	}
}

func discoveredHarness(sourceID, relPath string) *v1.Harness {
	return &v1.Harness{
		Name: harnessObjectName(sourceID, relPath),
		Spec: v1.HarnessSpec{
			SourceID:     sourceID,
			RelativePath: relPath,
		},
	}
}

func discoveredAgent(sourceID, relPath, harnessRef string) *v1.HostedAgent {
	agent := &v1.HostedAgent{
		Name: agentObjectName(sourceID, relPath),
		Spec: v1.HostedAgentSpec{
			SourceID:     sourceID,
			RelativePath: relPath,
		},
	}
	agent.Spec.Manifest.HarnessID = harnessRef
	return agent
}

func TestResolveHarnessReferences(t *testing.T) {
	t.Run("path reference resolves to the harness object name", func(t *testing.T) {
		harness := discoveredHarness("as1src", "harnesses/claude-code")
		agent := discoveredAgent("as1src", "agents/reviewer", "harnesses/claude-code")

		require.NoError(t, resolveHarnessReferences([]*v1.HostedAgent{agent}, []*v1.Harness{harness}))
		assert.Equal(t, harness.Name, agent.Spec.Manifest.HarnessID)
	})

	t.Run("full harness ID passes through untouched", func(t *testing.T) {
		ref := system.HarnessPrefix + "abcdef"
		agent := discoveredAgent("as1src", "agents/reviewer", ref)

		require.NoError(t, resolveHarnessReferences([]*v1.HostedAgent{agent}, nil))
		assert.Equal(t, ref, agent.Spec.Manifest.HarnessID)
	})

	t.Run("unknown path reference fails the sync", func(t *testing.T) {
		harness := discoveredHarness("as1src", "harnesses/claude-code")
		agent := discoveredAgent("as1src", "agents/reviewer", "harnesses/missing")

		err := resolveHarnessReferences([]*v1.HostedAgent{agent}, []*v1.Harness{harness})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "harnesses/missing")
		assert.Contains(t, err.Error(), "agents/reviewer")
	})

	t.Run("missing reference fails the sync", func(t *testing.T) {
		agent := discoveredAgent("as1src", "agents/reviewer", "")

		err := resolveHarnessReferences([]*v1.HostedAgent{agent}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not name a harness")
	})
}

func TestSourceObjectNames(t *testing.T) {
	t.Run("deterministic across syncs", func(t *testing.T) {
		assert.Equal(t,
			harnessObjectName("as1src", "harnesses/claude-code"),
			harnessObjectName("as1src", "harnesses/claude-code"),
			"the same definition must map to the same resource on every sync")
	})

	t.Run("kinds and paths do not collide", func(t *testing.T) {
		names := []string{
			harnessObjectName("as1src", "tools/my_thing"),
			// Sanitizes to the same visible fragment; the hash must separate them.
			harnessObjectName("as1src", "tools/my-thing"),
			harnessObjectName("as1other", "tools/my_thing"),
			agentObjectName("as1src", "tools/my_thing"),
		}
		seen := make(map[string]struct{}, len(names))
		for _, n := range names {
			if _, ok := seen[n]; ok {
				t.Fatalf("duplicate object name %q", n)
			}
			seen[n] = struct{}{}
		}
	})

	t.Run("names carry the kind prefix and are valid", func(t *testing.T) {
		harnessName := harnessObjectName("as1src", "härness/…weird path")
		agentName := agentObjectName("as1src", "härness/…weird path")
		assert.True(t, len(harnessName) > len(system.HarnessPrefix) && harnessName[:len(system.HarnessPrefix)] == system.HarnessPrefix)
		assert.True(t, len(agentName) > len(system.HostedAgentPrefix) && agentName[:len(system.HostedAgentPrefix)] == system.HostedAgentPrefix)
		assert.Empty(t, validation.IsDNS1123Subdomain(harnessName))
		assert.Empty(t, validation.IsDNS1123Subdomain(agentName))
	})
}
