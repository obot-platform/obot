package data

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(t *testing.T, objects ...kclient.Object) kclient.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		Build()
}

// fakeOnce records which seeds have run, standing in for the row the gateway
// keeps. Seeds are identified by name, so a test can start from "never seeded"
// or from "already seeded" without a database.
type fakeOnce struct{ done map[string]bool }

func newFakeOnce(seeded ...string) *fakeOnce {
	f := &fakeOnce{done: map[string]bool{}}
	for _, name := range seeded {
		f.done[name] = true
	}
	return f
}

func (f *fakeOnce) RunOnce(ctx context.Context, name string, fn func(context.Context) error) error {
	if f.done[name] {
		return nil
	}
	if err := fn(ctx); err != nil {
		return err
	}
	f.done[name] = true
	return nil
}

func TestDataCreatesDefaultModelAccessPolicyWithLLMAliases(t *testing.T) {
	ctx := t.Context()
	client := newFakeClient(t)

	require.NoError(t, Data(ctx, client, newFakeOnce(), Defaults{}))

	var policy v1.ModelAccessPolicy
	require.NoError(t, client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ModelAccessPolicyPrefix + "-default",
	}, &policy))
	assert.Equal(t, "Default Policy", policy.Spec.Manifest.DisplayName)
	assert.Equal(t, []types.Subject{{
		Type: types.SubjectTypeSelector,
		ID:   "*",
	}}, policy.Spec.Manifest.Subjects)
	assert.Equal(t, []types.ModelResource{
		{ID: "obot://llm"},
		{ID: "obot://llm-mini"},
	}, policy.Spec.Manifest.Models)

	var aliases v1.DefaultModelAliasList
	require.NoError(t, client.List(ctx, &aliases))
	assert.Len(t, aliases.Items, 5)
}

func TestCreateDefaultSkillRepository(t *testing.T) {
	ctx := t.Context()

	t.Run("empty URL is no-op", func(t *testing.T) {
		c := newFakeClient(t)
		err := createDefaultSkillRepository(ctx, c, "", "main")
		require.NoError(t, err)

		var list v1.SkillRepositoryList
		require.NoError(t, c.List(ctx, &list))
		assert.Empty(t, list.Items)
	})

	t.Run("whitespace-only URL is no-op", func(t *testing.T) {
		c := newFakeClient(t)
		err := createDefaultSkillRepository(ctx, c, "  \n  ", "main")
		require.NoError(t, err)

		var list v1.SkillRepositoryList
		require.NoError(t, c.List(ctx, &list))
		assert.Empty(t, list.Items)
	})

	t.Run("invalid URL returns error", func(t *testing.T) {
		c := newFakeClient(t)
		err := createDefaultSkillRepository(ctx, c, "not-a-url", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid default skill repository URL")
	})

	t.Run("valid URL creates repository", func(t *testing.T) {
		c := newFakeClient(t)
		err := createDefaultSkillRepository(ctx, c, "https://github.com/obot-platform/skills", "main")
		require.NoError(t, err)

		var repo v1.SkillRepository
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultSkillRepository,
		}, &repo))
		assert.Equal(t, "Default", repo.Spec.DisplayName)
		assert.Equal(t, "https://github.com/obot-platform/skills", repo.Spec.RepoURL)
		assert.Equal(t, "main", repo.Spec.Ref)
	})

	t.Run("trims whitespace from URL and ref", func(t *testing.T) {
		c := newFakeClient(t)
		err := createDefaultSkillRepository(ctx, c, "  https://github.com/obot-platform/skills  ", "  main  ")
		require.NoError(t, err)

		var repo v1.SkillRepository
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultSkillRepository,
		}, &repo))
		assert.Equal(t, "https://github.com/obot-platform/skills", repo.Spec.RepoURL)
		assert.Equal(t, "main", repo.Spec.Ref)
	})

	t.Run("already exists is not an error", func(t *testing.T) {
		c := newFakeClient(t)

		// Create first time
		err := createDefaultSkillRepository(ctx, c, "https://github.com/obot-platform/skills", "main")
		require.NoError(t, err)

		// Create again — should succeed (idempotent)
		err = createDefaultSkillRepository(ctx, c, "https://github.com/obot-platform/skills", "v2")
		require.NoError(t, err)

		// Original should be unchanged
		var repo v1.SkillRepository
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultSkillRepository,
		}, &repo))
		assert.Equal(t, "main", repo.Spec.Ref)
	})
}

func TestCreateDefaultAgentCatalog(t *testing.T) {
	ctx := t.Context()
	const repoURL = "https://github.com/obot-platform/hosted-agents-catalog"

	t.Run("empty URL is no-op", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, createDefaultAgentCatalog(ctx, c, "", "main", false))

		var list v1.AgentCatalogList
		require.NoError(t, c.List(ctx, &list))
		assert.Empty(t, list.Items)
	})

	t.Run("whitespace-only URL is no-op", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, createDefaultAgentCatalog(ctx, c, "  \n  ", "main", false))

		var list v1.AgentCatalogList
		require.NoError(t, c.List(ctx, &list))
		assert.Empty(t, list.Items)
	})

	t.Run("invalid URL returns error", func(t *testing.T) {
		c := newFakeClient(t)
		err := createDefaultAgentCatalog(ctx, c, "not-a-url", "main", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid default agent catalog")
	})

	t.Run("local path is rejected outside development mode", func(t *testing.T) {
		c := newFakeClient(t)
		err := createDefaultAgentCatalog(ctx, c, "/home/dev/src/hosted-agents-catalog", "", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid default agent catalog")
	})

	t.Run("local path is accepted in development mode", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, createDefaultAgentCatalog(ctx, c, "/home/dev/src/hosted-agents-catalog", "", true))

		var source v1.AgentCatalog
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultAgentCatalog,
		}, &source))
		assert.Equal(t, "/home/dev/src/hosted-agents-catalog", source.Spec.RepoURL)
	})

	t.Run("invalid ref returns error", func(t *testing.T) {
		c := newFakeClient(t)
		err := createDefaultAgentCatalog(ctx, c, repoURL, "--upload-pack=evil", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gitRef must not begin with '-'")
	})

	t.Run("valid URL creates catalog", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, createDefaultAgentCatalog(ctx, c, repoURL, "main", false))

		var source v1.AgentCatalog
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultAgentCatalog,
		}, &source))
		assert.Equal(t, "Default", source.Spec.DisplayName)
		assert.Equal(t, repoURL, source.Spec.RepoURL)
		assert.Equal(t, "main", source.Spec.Ref)
	})

	t.Run("trims whitespace from URL and ref", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, createDefaultAgentCatalog(ctx, c, "  "+repoURL+"  ", "  main  ", false))

		var source v1.AgentCatalog
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultAgentCatalog,
		}, &source))
		assert.Equal(t, repoURL, source.Spec.RepoURL)
		assert.Equal(t, "main", source.Spec.Ref)
	})

	t.Run("already exists is not an error", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, createDefaultAgentCatalog(ctx, c, repoURL, "main", false))
		require.NoError(t, createDefaultAgentCatalog(ctx, c, repoURL, "v2", false))

		var source v1.AgentCatalog
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultAgentCatalog,
		}, &source))
		assert.Equal(t, "main", source.Spec.Ref)
	})
}

func TestDataSeedsHostedAgents(t *testing.T) {
	ctx := t.Context()
	const repoURL = "https://github.com/obot-platform/hosted-agents-catalog"

	seeded := func(t *testing.T, c kclient.Client) {
		t.Helper()
		var catalog v1.AgentCatalog
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultAgentCatalog,
		}, &catalog))
		assert.Equal(t, repoURL, catalog.Spec.RepoURL)

		var rules v1.HostedAgentAccessRuleList
		require.NoError(t, c.List(ctx, &rules))
		assert.NotEmpty(t, rules.Items, "seeding a catalog nobody may use leaves the feature unusable")
	}

	t.Run("seeds on a new installation", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, Data(ctx, c, newFakeOnce(), Defaults{HostedAgentsCatalogURL: repoURL}))
		seeded(t, c)
	})

	// The case this seed used to miss entirely. An installation that upgrades
	// into hosted agents already has MCP catalogs, which the surrounding seeds
	// read as "this server has run before" -- true, and beside the point: it has
	// never seen this feature. Gated that way it arrived with no harnesses, no
	// templates and nobody permitted to use them.
	t.Run("seeds on an installation that upgraded into the feature", func(t *testing.T) {
		c := newFakeClient(t, &v1.MCPCatalog{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.DefaultCatalog,
				Namespace: system.DefaultNamespace,
			},
		})
		require.NoError(t, Data(ctx, c, newFakeOnce(), Defaults{HostedAgentsCatalogURL: repoURL}))
		seeded(t, c)
	})

	// The record outlives what it created, so an administrator who deletes the
	// catalog does not find it back on the next start.
	t.Run("does not seed again once it has", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, Data(ctx, c, newFakeOnce(seedHostedAgents), Defaults{HostedAgentsCatalogURL: repoURL}))

		var list v1.AgentCatalogList
		require.NoError(t, c.List(ctx, &list))
		assert.Empty(t, list.Items)
	})
}
