package skillrepository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// mockFetcher implements the repositoryFetcher interface for testing.
type mockFetcher struct {
	fetchFn             func(ctx context.Context, repoURL, ref string) (*fetchedRepository, error)
	materializeCommitFn func(ctx context.Context, repoURL, commitSHA string) (*fetchedRepository, error)
}

func (m *mockFetcher) Fetch(ctx context.Context, repoURL, ref string) (*fetchedRepository, error) {
	if m.fetchFn != nil {
		return m.fetchFn(ctx, repoURL, ref)
	}
	return nil, fmt.Errorf("fetchFn not set")
}

func (m *mockFetcher) MaterializeCommit(ctx context.Context, repoURL, commitSHA string) (*fetchedRepository, error) {
	if m.materializeCommitFn != nil {
		return m.materializeCommitFn(ctx, repoURL, commitSHA)
	}
	return nil, fmt.Errorf("materializeCommitFn not set")
}

// newFakeClient creates a fake k8s client with the storage scheme and status subresources.
func newFakeClient(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.SkillRepository{}, &v1.Skill{}).
		WithObjects(objects...).
		Build()
}

// newSkillRepository creates a test SkillRepository resource.
func newSkillRepository(name, namespace string) *v1.SkillRepository {
	return &v1.SkillRepository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "SkillRepository",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.SkillRepositorySpec{
			SkillRepositoryManifest: types.SkillRepositoryManifest{
				RepoURL:     "https://github.com/owner/repo",
				Ref:         "main",
				DisplayName: "Test Repo",
			},
		},
	}
}

// newSkill creates a test Skill resource for the given repo.
func newSkill(name, namespace, repoID, description string) *v1.Skill {
	return &v1.Skill{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Skill",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.SkillSpec{
			SkillManifest: types.SkillManifest{
				Name:        name,
				Description: description,
			},
			RepoID: repoID,
		},
		Status: v1.SkillStatus{
			Valid: true,
		},
	}
}

// createFetchedRepo creates a temp directory with SKILL.md files for use as a fetchedRepository.
func createFetchedRepo(t *testing.T, skills map[string]string) *fetchedRepository {
	t.Helper()
	root := t.TempDir()
	for dirName, description := range skills {
		dir := filepath.Join(root, dirName)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n# %s\n", dirName, description, dirName)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
	}
	return &fetchedRepository{
		RepoRoot:  root,
		CommitSHA: "abc123def456",
		cleanup:   func() {},
	}
}

func TestUpsertSkills(t *testing.T) {
	ctx := context.Background()

	t.Run("create from scratch", func(t *testing.T) {
		c := newFakeClient(t)

		skills := []*v1.Skill{
			newSkill("skill-a", "default", "repo1", "Skill A"),
			newSkill("skill-b", "default", "repo1", "Skill B"),
		}

		err := upsertSkills(ctx, c, "default", "repo1", skills)
		require.NoError(t, err)

		// Verify both were created
		var list v1.SkillList
		require.NoError(t, c.List(ctx, &list, kclient.InNamespace("default")))
		assert.Len(t, list.Items, 2)
	})

	t.Run("update existing", func(t *testing.T) {
		existing := newSkill("skill-a", "default", "repo1", "Old description")
		c := newFakeClient(t, existing)

		updated := newSkill("skill-a", "default", "repo1", "New description")
		err := upsertSkills(ctx, c, "default", "repo1", []*v1.Skill{updated})
		require.NoError(t, err)

		var got v1.Skill
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: "default", Name: "skill-a"}, &got))
		assert.Equal(t, "New description", got.Spec.Description)
	})

	t.Run("prune removed", func(t *testing.T) {
		existing1 := newSkill("skill-a", "default", "repo1", "A")
		existing2 := newSkill("skill-b", "default", "repo1", "B")
		c := newFakeClient(t, existing1, existing2)

		// Only skill-a in desired set
		err := upsertSkills(ctx, c, "default", "repo1", []*v1.Skill{
			newSkill("skill-a", "default", "repo1", "A"),
		})
		require.NoError(t, err)

		var list v1.SkillList
		require.NoError(t, c.List(ctx, &list, kclient.InNamespace("default")))
		assert.Len(t, list.Items, 1)
		assert.Equal(t, "skill-a", list.Items[0].Name)
	})

	t.Run("create update prune combo", func(t *testing.T) {
		existing1 := newSkill("skill-a", "default", "repo1", "Old A")
		existing2 := newSkill("skill-c", "default", "repo1", "C to prune")
		c := newFakeClient(t, existing1, existing2)

		desired := []*v1.Skill{
			newSkill("skill-a", "default", "repo1", "Updated A"),
			newSkill("skill-b", "default", "repo1", "New B"),
		}
		err := upsertSkills(ctx, c, "default", "repo1", desired)
		require.NoError(t, err)

		var list v1.SkillList
		require.NoError(t, c.List(ctx, &list, kclient.InNamespace("default")))
		assert.Len(t, list.Items, 2)

		names := make(map[string]bool)
		for _, s := range list.Items {
			names[s.Name] = true
		}
		assert.True(t, names["skill-a"])
		assert.True(t, names["skill-b"])
		assert.False(t, names["skill-c"])
	})

	t.Run("empty desired prunes all for repo", func(t *testing.T) {
		existing := newSkill("skill-a", "default", "repo1", "A")
		c := newFakeClient(t, existing)

		err := upsertSkills(ctx, c, "default", "repo1", nil)
		require.NoError(t, err)

		var list v1.SkillList
		require.NoError(t, c.List(ctx, &list, kclient.InNamespace("default")))
		assert.Empty(t, list.Items)
	})

	t.Run("does not touch other repos skills", func(t *testing.T) {
		repo1Skill := newSkill("skill-r1", "default", "repo1", "Repo 1")
		repo2Skill := newSkill("skill-r2", "default", "repo2", "Repo 2")
		c := newFakeClient(t, repo1Skill, repo2Skill)

		// Prune all for repo1
		err := upsertSkills(ctx, c, "default", "repo1", nil)
		require.NoError(t, err)

		// repo2's skill should still exist
		var list v1.SkillList
		require.NoError(t, c.List(ctx, &list, kclient.InNamespace("default")))
		require.Len(t, list.Items, 1)
		assert.Equal(t, "skill-r2", list.Items[0].Name)
	})

	t.Run("empty both no-op", func(t *testing.T) {
		c := newFakeClient(t)
		err := upsertSkills(ctx, c, "default", "repo1", nil)
		require.NoError(t, err)
	})
}

func TestListSkillsForRepo(t *testing.T) {
	ctx := context.Background()

	t.Run("filters by repoID", func(t *testing.T) {
		skills := []kclient.Object{
			newSkill("s1", "default", "repo1", "S1"),
			newSkill("s2", "default", "repo1", "S2"),
			newSkill("s3", "default", "repo2", "S3"),
		}
		c := newFakeClient(t, skills...)

		result, err := listSkillsForRepo(ctx, c, "default", "repo1")
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Contains(t, result, "s1")
		assert.Contains(t, result, "s2")
	})

	t.Run("empty namespace returns empty", func(t *testing.T) {
		c := newFakeClient(t)
		result, err := listSkillsForRepo(ctx, c, "default", "repo1")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("different namespace returns empty", func(t *testing.T) {
		skill := newSkill("s1", "ns-a", "repo1", "S1")
		c := newFakeClient(t, skill)

		result, err := listSkillsForRepo(ctx, c, "ns-b", "repo1")
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestSync(t *testing.T) {
	fixedTime := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)

	t.Run("happy path", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		c := newFakeClient(t, repo)

		fetched := createFetchedRepo(t, map[string]string{
			"skill-a": "Skill A",
			"skill-b": "Skill B",
		})

		h := &Handler{
			fetcher: &mockFetcher{
				fetchFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
					return fetched, nil
				},
			},
			now: func() time.Time { return fixedTime },
		}

		req := router.Request{
			Client:    c,
			Object:    repo,
			Ctx:       context.Background(),
			Namespace: repo.Namespace,
			Name:      repo.Name,
			Key:       repo.Namespace + "/" + repo.Name,
		}
		resp := &router.ResponseWrapper{}

		err := h.Sync(req, resp)
		require.NoError(t, err)

		// Verify status
		var updated v1.SkillRepository
		require.NoError(t, c.Get(context.Background(), kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
		assert.Empty(t, updated.Status.SyncError)
		assert.Equal(t, "abc123def456", updated.Status.ResolvedCommitSHA)
		assert.Equal(t, 2, updated.Status.DiscoveredSkillCount)
		assert.False(t, updated.Status.IsSyncing)

		// Verify skills created
		var skills v1.SkillList
		require.NoError(t, c.List(context.Background(), &skills, kclient.InNamespace("default")))
		assert.Len(t, skills.Items, 2)

		// Verify retry
		assert.Equal(t, syncInterval, resp.Delay)
	})

	t.Run("skip when recently synced", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		repo.Status.LastSyncTime = metav1.NewTime(fixedTime.Add(-30 * time.Minute))
		c := newFakeClient(t, repo)

		fetchCalled := false
		h := &Handler{
			fetcher: &mockFetcher{
				fetchFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
					fetchCalled = true
					return nil, fmt.Errorf("should not be called")
				},
			},
			now: func() time.Time { return fixedTime },
		}

		req := router.Request{
			Client:    c,
			Object:    repo,
			Ctx:       context.Background(),
			Namespace: repo.Namespace,
			Name:      repo.Name,
			Key:       repo.Namespace + "/" + repo.Name,
		}
		resp := &router.ResponseWrapper{}

		err := h.Sync(req, resp)
		require.NoError(t, err)
		assert.False(t, fetchCalled)
		// Should retry after remaining interval (~30min)
		assert.True(t, resp.Delay > 0 && resp.Delay <= 30*time.Minute+time.Second,
			"expected retry delay ~30min, got %v", resp.Delay)
	})

	t.Run("force sync bypasses interval", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		repo.Status.LastSyncTime = metav1.NewTime(fixedTime.Add(-30 * time.Minute))
		repo.Annotations = map[string]string{
			v1.SkillRepositorySyncAnnotation: "true",
		}
		c := newFakeClient(t, repo)

		fetched := createFetchedRepo(t, map[string]string{
			"skill-a": "Skill A",
		})
		fetchCalled := false
		h := &Handler{
			fetcher: &mockFetcher{
				fetchFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
					fetchCalled = true
					return fetched, nil
				},
			},
			now: func() time.Time { return fixedTime },
		}

		req := router.Request{
			Client:    c,
			Object:    repo,
			Ctx:       context.Background(),
			Namespace: repo.Namespace,
			Name:      repo.Name,
			Key:       repo.Namespace + "/" + repo.Name,
		}
		resp := &router.ResponseWrapper{}

		err := h.Sync(req, resp)
		require.NoError(t, err)
		assert.True(t, fetchCalled)

		// Verify annotation was cleared
		var updated v1.SkillRepository
		require.NoError(t, c.Get(context.Background(), kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
		_, hasAnnotation := updated.Annotations[v1.SkillRepositorySyncAnnotation]
		assert.False(t, hasAnnotation, "sync annotation should be cleared after successful force sync")
	})

	t.Run("fetch failure records error", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		c := newFakeClient(t, repo)

		h := &Handler{
			fetcher: &mockFetcher{
				fetchFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
					return nil, fmt.Errorf("network timeout")
				},
			},
			now: func() time.Time { return fixedTime },
		}

		req := router.Request{
			Client:    c,
			Object:    repo,
			Ctx:       context.Background(),
			Namespace: repo.Namespace,
			Name:      repo.Name,
			Key:       repo.Namespace + "/" + repo.Name,
		}
		resp := &router.ResponseWrapper{}

		err := h.Sync(req, resp)
		require.NoError(t, err) // handler returns nil, records error in status

		var updated v1.SkillRepository
		require.NoError(t, c.Get(context.Background(), kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
		assert.Contains(t, updated.Status.SyncError, "network timeout")
		assert.False(t, updated.Status.IsSyncing)
		assert.Equal(t, syncInterval, resp.Delay)
	})

	t.Run("build failure records error", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		c := newFakeClient(t, repo)

		// Create a fetched repo with an oversized SKILL.md that will cause buildSkillsFromRepository to error
		root := t.TempDir()
		skillDir := filepath.Join(root, "bad-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		bigContent := make([]byte, maxSkillMDBytes+1)
		for i := range bigContent {
			bigContent[i] = 'x'
		}
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), bigContent, 0o644))

		h := &Handler{
			fetcher: &mockFetcher{
				fetchFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
					return &fetchedRepository{
						RepoRoot:  root,
						CommitSHA: "abc123",
						cleanup:   func() {},
					}, nil
				},
			},
			now: func() time.Time { return fixedTime },
		}

		req := router.Request{
			Client:    c,
			Object:    repo,
			Ctx:       context.Background(),
			Namespace: repo.Namespace,
			Name:      repo.Name,
			Key:       repo.Namespace + "/" + repo.Name,
		}
		resp := &router.ResponseWrapper{}

		err := h.Sync(req, resp)
		require.NoError(t, err)

		var updated v1.SkillRepository
		require.NoError(t, c.Get(context.Background(), kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
		assert.NotEmpty(t, updated.Status.SyncError)
		assert.False(t, updated.Status.IsSyncing)
	})
}

func TestClearIsSyncing(t *testing.T) {
	fixedTime := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	h := &Handler{now: func() time.Time { return fixedTime }}
	ctx := context.Background()

	t.Run("clears when true", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		repo.Status.IsSyncing = true
		c := newFakeClient(t, repo)

		// Set IsSyncing via status update first since fake client needs it
		require.NoError(t, c.Status().Update(ctx, repo))

		h.clearIsSyncing(ctx, c, "default", "repo1")

		var updated v1.SkillRepository
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
		assert.False(t, updated.Status.IsSyncing)
	})

	t.Run("no-op when already false", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		repo.Status.IsSyncing = false
		c := newFakeClient(t, repo)

		// Should not panic or error
		h.clearIsSyncing(ctx, c, "default", "repo1")

		var updated v1.SkillRepository
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
		assert.False(t, updated.Status.IsSyncing)
	})

	t.Run("handles not found gracefully", func(t *testing.T) {
		c := newFakeClient(t) // no objects
		// Should not panic
		h.clearIsSyncing(ctx, c, "default", "nonexistent")
	})
}

func TestClearSyncAnnotation(t *testing.T) {
	ctx := context.Background()

	t.Run("removes annotation", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		repo.Annotations = map[string]string{
			v1.SkillRepositorySyncAnnotation: "true",
			"other-annotation":               "keep",
		}
		c := newFakeClient(t, repo)

		err := clearSyncAnnotation(ctx, c, "default", "repo1")
		require.NoError(t, err)

		var updated v1.SkillRepository
		require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
		_, hasSync := updated.Annotations[v1.SkillRepositorySyncAnnotation]
		assert.False(t, hasSync)
		assert.Equal(t, "keep", updated.Annotations["other-annotation"])
	})

	t.Run("nil annotations map", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		repo.Annotations = nil
		c := newFakeClient(t, repo)

		err := clearSyncAnnotation(ctx, c, "default", "repo1")
		require.NoError(t, err)
	})

	t.Run("annotation not present", func(t *testing.T) {
		repo := newSkillRepository("repo1", "default")
		repo.Annotations = map[string]string{"other": "value"}
		c := newFakeClient(t, repo)

		err := clearSyncAnnotation(ctx, c, "default", "repo1")
		require.NoError(t, err)
	})
}

func TestRecordFailure(t *testing.T) {
	fixedTime := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	h := &Handler{now: func() time.Time { return fixedTime }}
	ctx := context.Background()

	repo := newSkillRepository("repo1", "default")
	c := newFakeClient(t, repo)

	err := h.recordFailure(ctx, c, "default", "repo1", fmt.Errorf("sync failed: timeout"))
	require.NoError(t, err)

	var updated v1.SkillRepository
	require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
	assert.Equal(t, "sync failed: timeout", updated.Status.SyncError)
	assert.True(t, updated.Status.LastSyncTime.Time.Equal(fixedTime),
		"expected LastSyncTime %v, got %v", fixedTime, updated.Status.LastSyncTime.Time)
}

func TestRecordSuccess(t *testing.T) {
	fixedTime := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	h := &Handler{now: func() time.Time { return fixedTime }}
	ctx := context.Background()

	repo := newSkillRepository("repo1", "default")
	repo.Status.SyncError = "previous error"
	c := newFakeClient(t, repo)
	// Persist the initial status
	require.NoError(t, c.Status().Update(ctx, repo))

	err := h.recordSuccess(ctx, c, "default", "repo1", "sha256abc", 5)
	require.NoError(t, err)

	var updated v1.SkillRepository
	require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: "default", Name: "repo1"}, &updated))
	assert.Empty(t, updated.Status.SyncError)
	assert.Equal(t, "sha256abc", updated.Status.ResolvedCommitSHA)
	assert.Equal(t, 5, updated.Status.DiscoveredSkillCount)
	assert.True(t, updated.Status.LastSyncTime.Time.Equal(fixedTime),
		"expected LastSyncTime %v, got %v", fixedTime, updated.Status.LastSyncTime.Time)
}

func TestMaterializeSkillSource(t *testing.T) {
	ctx := context.Background()

	t.Run("missing repoURL", func(t *testing.T) {
		skill := &v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "test-skill"},
			Spec: v1.SkillSpec{
				CommitSHA:    "abc123",
				RelativePath: "my-skill",
			},
		}
		_, _, err := materializeSkillSource(ctx, &mockFetcher{}, skill)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing repoURL")
	})

	t.Run("missing commitSHA", func(t *testing.T) {
		skill := &v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "test-skill"},
			Spec: v1.SkillSpec{
				RepoURL:      "https://github.com/owner/repo",
				RelativePath: "my-skill",
			},
		}
		_, _, err := materializeSkillSource(ctx, &mockFetcher{}, skill)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing commitSHA")
	})

	t.Run("missing relativePath", func(t *testing.T) {
		skill := &v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "test-skill"},
			Spec: v1.SkillSpec{
				RepoURL:   "https://github.com/owner/repo",
				CommitSHA: "abc123",
			},
		}
		_, _, err := materializeSkillSource(ctx, &mockFetcher{}, skill)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing relativePath")
	})

	t.Run("valid skill returns correct path", func(t *testing.T) {
		root := t.TempDir()
		skillDir := filepath.Join(root, "my-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))

		fetcher := &mockFetcher{
			materializeCommitFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
				return &fetchedRepository{
					RepoRoot:  root,
					CommitSHA: "abc123",
					cleanup:   func() {},
				}, nil
			},
		}

		skill := &v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "test-skill"},
			Spec: v1.SkillSpec{
				RepoURL:      "https://github.com/owner/repo",
				CommitSHA:    "abc123",
				RelativePath: "my-skill",
			},
		}

		fetched, path, err := materializeSkillSource(ctx, fetcher, skill)
		require.NoError(t, err)
		defer fetched.Cleanup()
		assert.DirExists(t, path)
		absSkillDir, _ := filepath.Abs(skillDir)
		assert.Equal(t, absSkillDir, path)
	})

	t.Run("relativePath is file not dir", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "not-a-dir"), []byte("file"), 0o644))

		fetcher := &mockFetcher{
			materializeCommitFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
				return &fetchedRepository{
					RepoRoot:  root,
					CommitSHA: "abc123",
					cleanup:   func() {},
				}, nil
			},
		}

		skill := &v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "test-skill"},
			Spec: v1.SkillSpec{
				RepoURL:      "https://github.com/owner/repo",
				CommitSHA:    "abc123",
				RelativePath: "not-a-dir",
			},
		}

		_, _, err := materializeSkillSource(ctx, fetcher, skill)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("path traversal in relativePath", func(t *testing.T) {
		root := t.TempDir()

		fetcher := &mockFetcher{
			materializeCommitFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
				return &fetchedRepository{
					RepoRoot:  root,
					CommitSHA: "abc123",
					cleanup:   func() {},
				}, nil
			},
		}

		skill := &v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "test-skill"},
			Spec: v1.SkillSpec{
				RepoURL:      "https://github.com/owner/repo",
				CommitSHA:    "abc123",
				RelativePath: "../../../etc",
			},
		}

		_, _, err := materializeSkillSource(ctx, fetcher, skill)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "escapes")
	})

	t.Run("relativePath is symlink", func(t *testing.T) {
		root := t.TempDir()
		realDir := filepath.Join(root, "real-dir")
		require.NoError(t, os.MkdirAll(realDir, 0o755))
		linkPath := filepath.Join(root, "link-dir")
		if err := os.Symlink(realDir, linkPath); err != nil {
			t.Skip("symlinks not supported on this platform")
		}

		fetcher := &mockFetcher{
			materializeCommitFn: func(_ context.Context, _, _ string) (*fetchedRepository, error) {
				return &fetchedRepository{
					RepoRoot:  root,
					CommitSHA: "abc123",
					cleanup:   func() {},
				}, nil
			},
		}

		skill := &v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "test-skill"},
			Spec: v1.SkillSpec{
				RepoURL:      "https://github.com/owner/repo",
				CommitSHA:    "abc123",
				RelativePath: "link-dir",
			},
		}

		// Note: os.Lstat on symlink will NOT set ModeSymlink because safeJoinWithin
		// resolves via filepath.Abs. The Lstat in materializeSkillSource checks the
		// resolved path. Since the symlink target is a real directory, Lstat on the
		// symlink itself should show ModeSymlink.
		_, _, err := materializeSkillSource(ctx, fetcher, skill)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "symbolic link")
	})
}
