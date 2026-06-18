package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSkillRouteAuthorization(t *testing.T) {
	authorizer := newSkillRouteTestAuthorizer(t)

	tests := []struct {
		name    string
		method  string
		path    string
		user    user.Info
		allowed bool
	}{
		{
			name:   "admin can access skill repositories",
			method: http.MethodGet,
			path:   "/api/skill-repositories",
			user: &user.DefaultInfo{
				Name:   "admin",
				Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "basic user cannot access skill repositories",
			method: http.MethodGet,
			path:   "/api/skill-repositories",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "basic-only user cannot access skills list",
			method: http.MethodGet,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "skills-scoped user can access skills list",
			method: http.MethodGet,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "skills-scoped user can access skill detail",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "skills-scoped user can download skill",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id/download",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "skills-scoped user can preview skill",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id/preview",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "MCP-scoped user cannot access skills list",
			method: http.MethodGet,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "skills-scoped user cannot POST skills",
			method: http.MethodPost,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "key-user",
				Groups: []string{types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "auditor with skills scope can access skills list",
			method: http.MethodGet,
			path:   "/api/skills",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor with skills scope can access skill detail",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor with skills scope can download skill",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id/download",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can preview skill",
			method: http.MethodGet,
			path:   "/api/skills/some-skill-id/preview",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupSkills, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill repositories",
			method: http.MethodGet,
			path:   "/api/skill-repositories",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill repository detail",
			method: http.MethodGet,
			path:   "/api/skill-repositories/some-repo-id",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill access rules",
			method: http.MethodGet,
			path:   "/api/skill-access-rules",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor can access skill access rule detail",
			method: http.MethodGet,
			path:   "/api/skill-access-rules/some-rule-id",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "auditor cannot POST skill repositories",
			method: http.MethodPost,
			path:   "/api/skill-repositories",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "auditor cannot POST skill access rules",
			method: http.MethodPost,
			path:   "/api/skill-access-rules",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			assert.Equal(t, tt.allowed, authorizer.Authorize(req, tt.user))
		})
	}
}

func TestSkillGetAuthorizationUsesAccessRules(t *testing.T) {
	authorizer := newSkillAccessRuleTestAuthorizer(t,
		&v1.Skill{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sk1",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.SkillSpec{
				RepoID: "repo-1",
			},
			Status: v1.SkillStatus{
				Valid: true,
			},
		},
		&v1.SkillAccessRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "allow-user1-sk1",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.SkillAccessRuleSpec{
				Manifest: types.SkillAccessRuleManifest{
					Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}},
					Resources: []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}},
				},
			},
		},
	)

	for _, tt := range []struct {
		name    string
		user    user.Info
		allowed bool
	}{
		{
			name:    "allowed by matching skill access rule",
			user:    skillUser("user1"),
			allowed: true,
		},
		{
			name:    "denied without matching skill access rule",
			user:    skillUser("user2"),
			allowed: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/skills/sk1", nil)
			assert.Equal(t, tt.allowed, authorizer.Authorize(req, tt.user))
		})
	}
}

func newSkillRouteTestAuthorizer(t *testing.T) *Authorizer {
	t.Helper()

	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.Skill{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-skill-id",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.SkillSpec{
			RepoID: "repo-1",
		},
		Status: v1.SkillStatus{
			Valid: true,
		},
	}).Build()

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		skillaccessrule.ResourceSelectorIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
	})
	_ = indexer.Add(&v1.SkillAccessRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-all",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.SkillAccessRuleSpec{
			Manifest: types.SkillAccessRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				Resources: []types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}},
			},
		},
	})

	return NewAuthorizer(nil, storage, storage, false, nil, skillaccessrule.NewHelper(indexer), false)
}

func newSkillAccessRuleTestAuthorizer(t *testing.T, skill *v1.Skill, rules ...*v1.SkillAccessRule) *Authorizer {
	t.Helper()

	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(skill).Build()
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		skillaccessrule.SkillIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSkill {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		skillaccessrule.RepositoryIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSkillRepository {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		skillaccessrule.ResourceSelectorIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
	})
	for _, rule := range rules {
		if err := indexer.Add(rule); err != nil {
			t.Fatalf("add skill access rule to indexer: %v", err)
		}
	}

	return NewAuthorizer(nil, storage, storage, false, nil, skillaccessrule.NewHelper(indexer), false)
}

func skillUser(uid string) user.Info {
	return &user.DefaultInfo{
		Name:   uid,
		UID:    uid,
		Groups: []string{types.GroupSkills, types.GroupAuthenticated},
	}
}
