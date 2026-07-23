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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/skillrepository"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeSkillRepositoryCredentialClient struct {
	credential gatewaytypes.Credential
}

func newHandlerTestGateway(t *testing.T) *gclient.Client {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway
}

func (f *fakeSkillRepositoryCredentialClient) RevealCredential(context.Context, []string, string) (gatewaytypes.Credential, error) {
	return f.credential, nil
}

func TestRevealSkillRepositoryToken(t *testing.T) {
	skill := &v1.Skill{Spec: v1.SkillSpec{
		RepoID:  "repo-1",
		RepoURL: "https://git.example.com/org/repo.git",
	}}
	client := &fakeSkillRepositoryCredentialClient{credential: gatewaytypes.Credential{
		Secrets: map[string]string{skill.Spec.RepoURL: "private-token"},
	}}

	token, err := revealSkillRepositoryToken(t.Context(), client, skill)
	require.NoError(t, err)
	assert.Equal(t, "private-token", token)

	_, err = revealSkillRepositoryToken(t.Context(), credentialNotFoundClient{}, skill)
	require.NoError(t, err)
}

type credentialNotFoundClient struct{}

func (credentialNotFoundClient) RevealCredential(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
	return gatewaytypes.Credential{}, gclient.CredentialNotFoundError{Contexts: contexts, Name: name}
}

func TestParseSkillRepositoryRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/skill-repositories", strings.NewReader(`{"displayName":"Repo","repoURL":"github.com/example/repo","ref":" main ","gitCredentialID":" gc1-test ","sourceURLCredentials":{"github.com/example/repo":"secret"}}`))
	rec := httptest.NewRecorder()

	manifest, credentials, err := parseSkillRepositoryRequest(api.Context{
		ResponseWriter: rec,
		Request:        req,
	})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/example/repo", manifest.RepoURL)
	assert.Equal(t, "main", manifest.Ref)
	assert.Equal(t, "gc1-test", manifest.GitCredentialID)
	assert.Nil(t, manifest.SourceURLCredentials)
	assert.Equal(t, map[string]string{"https://github.com/example/repo": "secret"}, credentials)

	req = httptest.NewRequest(http.MethodPost, "/api/skill-repositories", strings.NewReader(`{"displayName":"Repo","repoURL":"http://github.com/example/repo"}`))
	rec = httptest.NewRecorder()
	_, _, err = parseSkillRepositoryRequest(api.Context{
		ResponseWriter: rec,
		Request:        req,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid repoURL")

	req = httptest.NewRequest(http.MethodPost, "/api/skill-repositories", strings.NewReader(`{"displayName":"Repo","repoURL":"https://github.com/example/repo","ref":"   "}`))
	rec = httptest.NewRecorder()
	_, _, err = parseSkillRepositoryRequest(api.Context{
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

	req := httptest.NewRequest(http.MethodPost, "/api/skill-access-rules", strings.NewReader(`{
		"subjects":[{"type":"user","id":"123"}],
		"resources":[
			{"type":"skill","id":"sk1"},
			{"type":"skillRepository","id":"skr1"}
		]
	}`))
	rec := httptest.NewRecorder()

	manifest, err := readAndValidateManifest(api.Context{
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
	_, err = readAndValidateManifest(api.Context{
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
			RepoURL: "https://github.com/example/repo",
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
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Name: "skr1", Namespace: system.DefaultNamespace}, &repo))
	assert.Equal(t, "true", repo.Annotations[v1.SkillRepositorySyncAnnotation])
}

func TestSkillRepositoryHandlerRejectsDuplicateNameAndURL(t *testing.T) {
	for _, test := range []struct {
		name         string
		manifest     string
		errorMessage string
	}{
		{
			name:         "display name",
			manifest:     `{"displayName":"Existing Source","repoURL":"https://github.com/example/other"}`,
			errorMessage: `a skill source named "Existing Source" already exists`,
		},
		{
			name:         "normalized repository URL",
			manifest:     `{"displayName":"Other Source","repoURL":"github.com/example/repo"}`,
			errorMessage: `a skill source with repository URL "https://github.com/example/repo" already exists`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := newFakeStorage(t, &v1.SkillRepository{
				ObjectMeta: metav1.ObjectMeta{Name: "skr-existing", Namespace: system.DefaultNamespace},
				Spec: v1.SkillRepositorySpec{
					DisplayName: "Existing Source",
					RepoURL:     "https://github.com/example/repo",
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/skill-repositories", strings.NewReader(test.manifest))

			err := NewSkillRepositoryHandler().Create(api.Context{Request: req, Storage: storage})
			require.Error(t, err)
			assert.ErrorContains(t, err, test.errorMessage)

			var httpErr *types.ErrHTTP
			require.ErrorAs(t, err, &httpErr)
			assert.Equal(t, http.StatusConflict, httpErr.Code)

			var repositories v1.SkillRepositoryList
			require.NoError(t, storage.List(t.Context(), &repositories))
			assert.Len(t, repositories.Items, 1)
		})
	}
}

func TestSkillRepositoryHandlerUpdateRejectsAnotherSourceNameAndURL(t *testing.T) {
	for _, test := range []struct {
		name         string
		manifest     string
		errorMessage string
	}{
		{
			name:         "display name",
			manifest:     `{"displayName":"Existing Source","repoURL":"https://github.com/example/other"}`,
			errorMessage: `a skill source named "Existing Source" already exists`,
		},
		{
			name:         "repository URL",
			manifest:     `{"displayName":"Updated Source","repoURL":"https://github.com/example/repo"}`,
			errorMessage: `a skill source with repository URL "https://github.com/example/repo" already exists`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := newFakeStorage(t,
				&v1.SkillRepository{
					ObjectMeta: metav1.ObjectMeta{Name: "skr-existing", Namespace: system.DefaultNamespace},
					Spec: v1.SkillRepositorySpec{
						DisplayName: "Existing Source",
						RepoURL:     "https://github.com/example/repo",
					},
				},
				&v1.SkillRepository{
					ObjectMeta: metav1.ObjectMeta{Name: "skr-updating", Namespace: system.DefaultNamespace},
					Spec: v1.SkillRepositorySpec{
						DisplayName: "Current Source",
						RepoURL:     "https://github.com/example/current",
					},
				},
			)
			req := httptest.NewRequest(http.MethodPut, "/api/skill-repositories/skr-updating", strings.NewReader(test.manifest))
			req.SetPathValue("skill_repository_id", "skr-updating")

			err := NewSkillRepositoryHandler().Update(api.Context{Request: req, Storage: storage})
			require.Error(t, err)
			assert.ErrorContains(t, err, test.errorMessage)

			var httpErr *types.ErrHTTP
			require.ErrorAs(t, err, &httpErr)
			assert.Equal(t, http.StatusConflict, httpErr.Code)
		})
	}
}

func TestSkillRepositoryHandlerUpdateAllowsOwnNameAndURL(t *testing.T) {
	storage := newFakeStorage(t, &v1.SkillRepository{
		ObjectMeta: metav1.ObjectMeta{Name: "skr-existing", Namespace: system.DefaultNamespace},
		Spec: v1.SkillRepositorySpec{
			DisplayName: "Existing Source",
			RepoURL:     "https://github.com/example/repo",
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/skill-repositories/skr-existing", strings.NewReader(`{"displayName":"Existing Source","repoURL":"github.com/example/repo"}`))
	req.SetPathValue("skill_repository_id", "skr-existing")
	rec := httptest.NewRecorder()

	err := NewSkillRepositoryHandler().Update(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
		GatewayClient:  newHandlerTestGateway(t),
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
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

	t.Run("basic user without all=true sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUser("user1"), "")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})
	t.Run("basic user with all=true still sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUser("user1"), "?all=true")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})

	t.Run("admin without all=true sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupAdmin), "")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})
	t.Run("admin with all=true sees all skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupAdmin), "?all=true")
		assert.ElementsMatch(t, []string{"sk-in-scope", "sk-out-scope"}, ids)
	})

	t.Run("owner without all=true sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupOwner, types.GroupAdmin), "")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})
	t.Run("owner with all=true sees all skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupOwner, types.GroupAdmin), "?all=true")
		assert.ElementsMatch(t, []string{"sk-in-scope", "sk-out-scope"}, ids)
	})

	t.Run("auditor without all=true sees only in-scope skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupAuditor), "")
		assert.ElementsMatch(t, []string{"sk-in-scope"}, ids)
	})
	t.Run("auditor with all=true sees all skills", func(t *testing.T) {
		ids := listSkills(t, testUserWithRole("user1", types.GroupAuditor), "?all=true")
		assert.ElementsMatch(t, []string{"sk-in-scope", "sk-out-scope"}, ids)
	})
}

func TestSkillHandlerDownloadPackagesMaterializedSkill(t *testing.T) {
	skill := &v1.Skill{
		ObjectMeta: metav1.ObjectMeta{Name: "sk1", Namespace: system.DefaultNamespace},
		Spec: v1.SkillSpec{
			SkillManifest: types.SkillManifest{Name: "postgres-helper"},
			RepoID:        "repo-1",
			RepoURL:       "https://git.example.com/org/repo.git",
			CommitSHA:     "abc123",
			RelativePath:  "skills/postgres-helper",
		},
		Status: v1.SkillStatus{Valid: true},
	}
	storage := newFakeStorage(t, skill)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: skill.Spec.RepoID,
		Name:    skillrepository.SkillRepositoryCredentialToolName,
		Secrets: map[string]string{skill.Spec.RepoURL: "private-token"},
	}))

	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(tempDir+"/SKILL.md", []byte("---\nname: postgres-helper\ndescription: Test\n---\n"), 0o644))
	require.NoError(t, os.MkdirAll(tempDir+"/scripts", 0o755))
	require.NoError(t, os.WriteFile(tempDir+"/scripts/run.sh", []byte("echo hi\n"), 0o755))

	handler := NewSkillHandler(newSkillAccessRuleHelper(t,
		newSkillRule("rule1", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-1"}}),
	))
	handler.materializeSkillSource = func(_ context.Context, got *v1.Skill, token string) (func(), string, error) {
		assert.Equal(t, "abc123", got.Spec.CommitSHA)
		assert.Equal(t, "skills/postgres-helper", got.Spec.RelativePath)
		assert.Equal(t, "private-token", token)
		return func() {}, tempDir, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/skills/sk1/download", nil)
	req.SetPathValue("id", "sk1")
	rec := httptest.NewRecorder()

	err := handler.Download(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
		GatewayClient:  gatewayClient,
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

func TestSkillHandlerPreviewReturnsSkillMD(t *testing.T) {
	want := []byte("---\nname: postgres-helper\ndescription: Preview me\n---\n\n# Instructions\n")
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
	gatewayClient := newHandlerTestGateway(t)

	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(tempDir+"/SKILL.md", want, 0o644))

	handler := NewSkillHandler(newSkillAccessRuleHelper(t,
		newSkillRule("rule1", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-1"}}),
	))
	handler.materializeSkillSource = func(_ context.Context, got *v1.Skill, token string) (func(), string, error) {
		assert.Equal(t, "abc123", got.Spec.CommitSHA)
		assert.Empty(t, token)
		return func() {}, tempDir, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/skills/sk1/preview", nil)
	req.SetPathValue("id", "sk1")
	rec := httptest.NewRecorder()

	err := handler.Preview(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           testUser("user1"),
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/markdown; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, string(want), rec.Body.String())
}

func newFakeStorage(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()

	builder := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.SkillRepository{}, "spec.gitCredentialID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.SkillRepository).Spec.GitCredentialID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.UserID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.Template)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.CompositeName}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerInstance).Spec.UserID}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerInstance).Spec.MCPServerName}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServerInstance).Spec.Template)}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerInstance).Spec.CompositeName}
		}).
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
