package data

import (
	"context"
	"testing"

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

func TestCreateDefaultSkillRepository(t *testing.T) {
	ctx := context.Background()

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
