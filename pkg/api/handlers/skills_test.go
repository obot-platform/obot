package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadAndValidateSkillRepositoryManifest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/skill-repositories", strings.NewReader(`{"displayName":"Repo","repoURL":"https://github.com/example/repo","ref":" main "}`))
	rec := httptest.NewRecorder()

	manifest, err := readAndValidateSkillRepositoryManifest(api.Context{
		ResponseWriter: rec,
		Request:        req,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/example/repo", manifest.RepoURL)
	assert.Equal(t, "main", manifest.Ref)

	req = httptest.NewRequest(http.MethodPost, "/api/skill-repositories", strings.NewReader(`{"displayName":"Repo","repoURL":"http://github.com/example/repo"}`))
	rec = httptest.NewRecorder()
	_, err = readAndValidateSkillRepositoryManifest(api.Context{
		ResponseWriter: rec,
		Request:        req,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid repoURL")

	req = httptest.NewRequest(http.MethodPost, "/api/skill-repositories", strings.NewReader(`{"displayName":"Repo","repoURL":"https://github.com/example/repo","ref":"   "}`))
	rec = httptest.NewRecorder()
	_, err = readAndValidateSkillRepositoryManifest(api.Context{
		ResponseWriter: rec,
		Request:        req,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ref must not be empty")
}

func TestSkillAccessRuleHandlerReadAndValidateManifest(t *testing.T) {
	storage := newFakeStorage(t,
		&v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "sk1", Namespace: system.DefaultNamespace},
			Spec:       v1.SkillSpec{RepoID: "skr1"},
		},
		&v1.SkillRepository{
			ObjectMeta: metav1.ObjectMeta{Name: "skr1", Namespace: system.DefaultNamespace},
		},
	)

	handler := NewSkillAccessRuleHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/skill-access-rules", strings.NewReader(`{
		"subjects":[{"type":"user","id":"123"}],
		"resources":[
			{"type":"skill","id":"sk1"},
			{"type":"skillRepository","id":"skr1"}
		]
	}`))
	rec := httptest.NewRecorder()

	manifest, err := handler.readAndValidateManifest(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
	})
	require.NoError(t, err)
	require.Len(t, manifest.Resources, 2)

	req = httptest.NewRequest(http.MethodPost, "/api/skill-access-rules", strings.NewReader(`{
		"subjects":[{"type":"user","id":"123"}],
		"resources":[{"type":"skill","id":"missing"}]
	}`))
	rec = httptest.NewRecorder()
	_, err = handler.readAndValidateManifest(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill missing not found")
}

func TestSkillRepositoryHandlerRefresh(t *testing.T) {
	storage := newFakeStorage(t, &v1.SkillRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "skr1",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.SkillRepositorySpec{
			SkillRepositoryManifest: types.SkillRepositoryManifest{
				RepoURL: "https://github.com/example/repo",
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/skill-repositories/skr1/refresh", nil)
	req.SetPathValue("skill_repository_id", "skr1")
	rec := httptest.NewRecorder()

	err := NewSkillRepositoryHandler().Refresh(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	var repo v1.SkillRepository
	require.NoError(t, storage.Get(context.Background(), kclient.ObjectKey{Name: "skr1", Namespace: system.DefaultNamespace}, &repo))
	assert.Equal(t, "true", repo.Annotations[v1.SkillRepositorySyncAnnotation])
}

func TestSkillHandlerListFiltersByAccessAndValidity(t *testing.T) {
	storage := newFakeStorage(t,
		&v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "sk-allowed", Namespace: system.DefaultNamespace},
			Spec: v1.SkillSpec{
				SkillManifest: types.SkillManifest{
					Name:        "postgres-helper",
					DisplayName: "Postgres Helper",
					Description: "Postgres utilities",
				},
				RepoID: "repo-1",
			},
			Status: v1.SkillStatus{Valid: true},
		},
		&v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "sk-invalid", Namespace: system.DefaultNamespace},
			Spec: v1.SkillSpec{
				SkillManifest: types.SkillManifest{
					Name: "broken-helper",
				},
				RepoID: "repo-1",
			},
			Status: v1.SkillStatus{Valid: false},
		},
		&v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "sk-direct", Namespace: system.DefaultNamespace},
			Spec: v1.SkillSpec{
				SkillManifest: types.SkillManifest{
					Name:        "redis-helper",
					DisplayName: "Redis Helper",
					Description: "Redis utilities",
				},
				RepoID: "repo-2",
			},
			Status: v1.SkillStatus{Valid: true},
		},
		&v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "sk-denied", Namespace: system.DefaultNamespace},
			Spec: v1.SkillSpec{
				SkillManifest: types.SkillManifest{
					Name: "denied-helper",
				},
				RepoID: "repo-3",
			},
			Status: v1.SkillStatus{Valid: true},
		},
	)

	handler := NewSkillHandler(newSkillAccessRuleHelper(t,
		newSkillRule("rule-repo", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-1"}}),
		newSkillRule("rule-skill", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk-direct"}}),
	))

	req := httptest.NewRequest(http.MethodGet, "/api/skills?q=helper&limit=10", nil)
	rec := httptest.NewRecorder()
	err := handler.List(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
		User:           testUser("user1"),
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var list types.SkillList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Items, 2)
	assert.Equal(t, "sk-allowed", list.Items[0].ID)
	assert.Equal(t, "sk-direct", list.Items[1].ID)

	req = httptest.NewRequest(http.MethodGet, "/api/skills?repoID=repo-2&q=redis", nil)
	rec = httptest.NewRecorder()
	err = handler.List(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
		User:           testUser("user1"),
	})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, "sk-direct", list.Items[0].ID)
}

func TestSkillHandlerListAllTrueScopeBypass(t *testing.T) {
	// Skills in repo-1 (in scope for user1) and repo-3 (out of scope)
	storage := newFakeStorage(t,
		&v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "sk-in-scope", Namespace: system.DefaultNamespace},
			Spec: v1.SkillSpec{
				SkillManifest: types.SkillManifest{Name: "in-scope-skill"},
				RepoID:        "repo-1",
			},
			Status: v1.SkillStatus{Valid: true},
		},
		&v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "sk-out-scope", Namespace: system.DefaultNamespace},
			Spec: v1.SkillSpec{
				SkillManifest: types.SkillManifest{Name: "out-of-scope-skill"},
				RepoID:        "repo-3",
			},
			Status: v1.SkillStatus{Valid: true},
		},
	)

	// user1 has access only to repo-1 via skill access rules
	handler := NewSkillHandler(newSkillAccessRuleHelper(t,
		newSkillRule("rule-repo", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-1"}}),
	))

	listSkills := func(t *testing.T, user kuser.Info, query string) []string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/skills"+query, nil)
		rec := httptest.NewRecorder()
		err := handler.List(api.Context{
			ResponseWriter: rec,
			Request:        req,
			Storage:        storage,
			User:           user,
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		var list types.SkillList
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		ids := make([]string, len(list.Items))
		for i := range list.Items {
			ids[i] = list.Items[i].ID
		}
		return ids
	}

	// Basic user: without all=true sees only in-scope; with all=true still only in-scope (no bypass)
	t.Run("basic user without all=true sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUser("user1"), "")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})
	t.Run("basic user with all=true still sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUser("user1"), "?all=true")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})

	// Admin: without all=true sees only in-scope; with all=true sees all (bypass)
	t.Run("admin without all=true sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupAdmin), "")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})
	t.Run("admin with all=true sees all skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupAdmin), "?all=true")
		assert.ElementsMatch(t, []string{"sk-in-scope", "sk-out-scope"}, ids)
	})

	// Owner: same as admin for scope bypass
	t.Run("owner without all=true sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupOwner), "")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})
	t.Run("owner with all=true sees all skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupOwner), "?all=true")
		assert.ElementsMatch(t, []string{"sk-in-scope", "sk-out-scope"}, ids)
	})

	// Auditor: same as admin for scope bypass
	t.Run("auditor without all=true sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupAuditor), "")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})
	t.Run("auditor with all=true sees all skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupAuditor), "?all=true")
		assert.ElementsMatch(t, []string{"sk-in-scope", "sk-out-scope"}, ids)
	})
}

func TestSkillHandlerGetReturnsNotFoundWhenUnauthorized(t *testing.T) {
	storage := newFakeStorage(t, &v1.Skill{
		ObjectMeta: metav1.ObjectMeta{Name: "sk1", Namespace: system.DefaultNamespace},
		Spec: v1.SkillSpec{
			SkillManifest: types.SkillManifest{Name: "private-skill"},
			RepoID:        "repo-1",
		},
		Status: v1.SkillStatus{Valid: true},
	})

	handler := NewSkillHandler(newSkillAccessRuleHelper(t))
	req := httptest.NewRequest(http.MethodGet, "/api/skills/sk1", nil)
	req.SetPathValue("id", "sk1")
	rec := httptest.NewRecorder()

	err := handler.Get(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
		User:           testUser("user1"),
	})
	require.Error(t, err)
	assert.True(t, types.IsNotFound(err))
}

func TestSkillHandlerDownloadPackagesMaterializedSkill(t *testing.T) {
	skill := &v1.Skill{
		ObjectMeta: metav1.ObjectMeta{Name: "sk1", Namespace: system.DefaultNamespace},
		Spec: v1.SkillSpec{
			SkillManifest: types.SkillManifest{Name: "postgres-helper"},
			RepoID:        "repo-1",
			CommitSHA:     "abc123",
			RelativePath:  "skills/postgres-helper",
		},
		Status: v1.SkillStatus{Valid: true},
	}
	storage := newFakeStorage(t, skill)

	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(tempDir+"/SKILL.md", []byte("---\nname: postgres-helper\ndescription: Test\n---\n"), 0o644))
	require.NoError(t, os.MkdirAll(tempDir+"/scripts", 0o755))
	require.NoError(t, os.WriteFile(tempDir+"/scripts/run.sh", []byte("echo hi\n"), 0o755))

	handler := NewSkillHandler(newSkillAccessRuleHelper(t,
		newSkillRule("rule1", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-1"}}),
	))
	handler.materializeSkillSource = func(_ context.Context, got *v1.Skill) (func(), string, error) {
		assert.Equal(t, "abc123", got.Spec.CommitSHA)
		assert.Equal(t, "skills/postgres-helper", got.Spec.RelativePath)
		return func() {}, tempDir, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/skills/sk1/download", nil)
	req.SetPathValue("id", "sk1")
	rec := httptest.NewRecorder()

	err := handler.Download(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
		User:           testUser("user1"),
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/zip", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "postgres-helper.zip")

	reader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	require.NoError(t, err)

	names := map[string]struct{}{}
	for _, file := range reader.File {
		names[file.Name] = struct{}{}
	}
	assert.Contains(t, names, "SKILL.md")
	assert.Contains(t, names, "scripts/run.sh")
}

func newFakeStorage(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()

	builder := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.Skill{}, "spec.repoID", func(obj kclient.Object) []string {
			skill := obj.(*v1.Skill)
			if skill.Spec.RepoID == "" {
				return nil
			}
			return []string{skill.Spec.RepoID}
		})

	return builder.Build()
}

func testUser(userID string, groups ...string) kuser.Info {
	return &kuser.DefaultInfo{
		Name:   userID,
		UID:    userID,
		Groups: []string{types.GroupBasic, types.GroupAuthenticated},
		Extra: map[string][]string{
			"auth_provider_groups": groups,
		},
	}
}

// testUserWithRole returns a user with the given ID and role groups (e.g. GroupAdmin).
// Used to test role-based behavior like ?all=true scope bypass; skill access still uses
// access rules keyed by user ID.
func testUserWithRole(userID string, roleGroups ...string) kuser.Info {
	groups := make([]string, 0, len(roleGroups)+1)
	groups = append(groups, roleGroups...)
	if !slices.Contains(groups, types.GroupAuthenticated) {
		groups = append(groups, types.GroupAuthenticated)
	}
	return &kuser.DefaultInfo{
		Name:   userID,
		UID:    userID,
		Groups: groups,
	}
}

func newSkillAccessRuleHelper(t *testing.T, rules ...*v1.SkillAccessRule) *skillaccessrule.Helper {
	t.Helper()

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"skill-ids": func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSkill {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"repository-ids": func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSkillRepository {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"selectors": func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"user-ids": func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeUser {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		"group-ids": func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeGroup {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		"subject-selectors": func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeSelector {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
	})

	for _, rule := range rules {
		require.NoError(t, indexer.Add(rule))
	}

	return skillaccessrule.NewHelper(indexer)
}

func newSkillRule(name string, subjects []types.Subject, resources []types.SkillResource) *v1.SkillAccessRule {
	return &v1.SkillAccessRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.SkillAccessRuleSpec{
			Manifest: types.SkillAccessRuleManifest{
				Subjects:  subjects,
				Resources: resources,
			},
		},
	}
}
