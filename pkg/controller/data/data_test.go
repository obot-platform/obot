package data

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestDataCreatesDefaultModelAccessPolicyWithLLMAliases(t *testing.T) {
	ctx := t.Context()
	client := newFakeClient(t)

	require.NoError(t, Data(ctx, client, Defaults{}))

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

func TestDataSeedsDefaultAgentCatalogOnFirstBoot(t *testing.T) {
	ctx := t.Context()
	const repoURL = "https://github.com/obot-platform/hosted-agents-catalog"

	t.Run("seeds on first boot", func(t *testing.T) {
		c := newFakeClient(t)
		require.NoError(t, Data(ctx, c, Defaults{HostedAgentsCatalogURL: repoURL}))

		var source v1.AgentCatalog
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultAgentCatalog,
		}, &source))
		assert.Equal(t, repoURL, source.Spec.RepoURL)
	})

	// An existing MCPCatalog stands in for "this server has booted before", so a
	// catalog an admin deleted is not resurrected.
	t.Run("does not seed when catalogs already exist", func(t *testing.T) {
		c := newFakeClient(t, &v1.MCPCatalog{
			Name:      system.DefaultCatalog,
			Namespace: system.DefaultNamespace,
		})
		require.NoError(t, Data(ctx, c, Defaults{HostedAgentsCatalogURL: repoURL}))

		var list v1.AgentCatalogList
		require.NoError(t, c.List(ctx, &list))
		assert.Empty(t, list.Items)
	})
}
