package skillaccessrule

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

func TestGetSkillAccessRulesForSkillFiltersDeletedAndNamespace(t *testing.T) {
	helper := newTestHelper(t,
		newRule("match", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}),
		newRule("deleted", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}, withDeletedTimestamp()),
		newRule("other-namespace", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}, withNamespace("other")),
		newRule("other-skill", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk2"}}),
	)

	rules, err := helper.GetSkillAccessRulesForSkill(system.DefaultNamespace, "sk1")
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "match", rules[0].Name)
}

func TestUserHasAccessToSkillID(t *testing.T) {
	user := testUser("user1", "eng")

	for _, tt := range []struct {
		name    string
		rules   []*v1.SkillAccessRule
		skillID string
		repoID  string
		want    bool
	}{
		{
			name: "direct user skill grant",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name: "group repository grant",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "skr1"}}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name: "global wildcard grant",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}, []types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name: "wildcard subject repo scoped grant",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "skr1"}}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name: "different skill does not grant access",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk2"}}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    false,
		},
		{
			name: "different group does not grant access",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{{Type: types.SubjectTypeGroup, ID: "ops"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "skr1"}}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    false,
		},
		{
			name: "multiple matching rules still grant access",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "skr1"}}),
				newRule("rule2", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name:    "empty identifiers fail closed",
			rules:   nil,
			skillID: "",
			repoID:  "",
			want:    false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			helper := newTestHelper(t, tt.rules...)

			hasAccess, err := helper.UserHasAccessToSkillID(user, tt.skillID, tt.repoID)
			require.NoError(t, err)
			assert.Equal(t, tt.want, hasAccess)
		})
	}
}

func TestUserHasAccessToSkillUsesSkillObject(t *testing.T) {
	helper := newTestHelper(t,
		newRule("rule1", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}),
	)

	hasAccess, err := helper.UserHasAccessToSkill(testUser("user1"), &v1.Skill{
		ObjectMeta: metav1.ObjectMeta{Name: "sk1"},
		Spec:       v1.SkillSpec{RepoID: "skr1"},
	})
	require.NoError(t, err)
	assert.True(t, hasAccess)
}

func TestGetUserSkillAccessScopeAggregatesAndDeduplicates(t *testing.T) {
	helper := newTestHelper(t,
		newRule("wildcard-subject", []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-global"}}),
		newRule("user-rule", []types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk-user"}}),
		newRule("group-rule", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-group"}}),
		newRule("duplicate-group-rule", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{
			{Type: types.SkillResourceTypeSkillRepository, ID: "repo-group"},
			{Type: types.SkillResourceTypeSkill, ID: "sk-group"},
		}),
	)

	allowAll, repoIDs, skillIDs, err := helper.GetUserSkillAccessScope(testUser("user1", "eng"))
	require.NoError(t, err)
	assert.False(t, allowAll)
	assert.Equal(t, map[string]struct{}{
		"repo-global": {},
		"repo-group":  {},
	}, repoIDs)
	assert.Equal(t, map[string]struct{}{
		"sk-user":  {},
		"sk-group": {},
	}, skillIDs)
}

func TestGetUserSkillAccessScopeAllowAll(t *testing.T) {
	helper := newTestHelper(t,
		newRule("rule1", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}}),
	)

	allowAll, repoIDs, skillIDs, err := helper.GetUserSkillAccessScope(testUser("user1", "eng"))
	require.NoError(t, err)
	assert.True(t, allowAll)
	assert.Empty(t, repoIDs)
	assert.Empty(t, skillIDs)
}

func newTestHelper(t *testing.T, rules ...*v1.SkillAccessRule) *Helper {
	t.Helper()

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		SkillIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSkill {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		RepositoryIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSkillRepository {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		ResourceSelectorIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, resource := range rule.Spec.Manifest.Resources {
				if resource.Type == types.SkillResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		UserIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeUser {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		GroupIDIndex: func(obj any) ([]string, error) {
			rule := obj.(*v1.SkillAccessRule)
			var results []string
			for _, subject := range rule.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeGroup {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		SubjectSelectorIndex: func(obj any) ([]string, error) {
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

	return NewHelper(indexer)
}

func newRule(name string, subjects []types.Subject, resources []types.SkillResource, opts ...ruleOpt) *v1.SkillAccessRule {
	rule := &v1.SkillAccessRule{
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

	for _, opt := range opts {
		opt(rule)
	}

	return rule
}

type ruleOpt func(*v1.SkillAccessRule)

func withNamespace(namespace string) ruleOpt {
	return func(rule *v1.SkillAccessRule) {
		rule.Namespace = namespace
	}
}

func withDeletedTimestamp() ruleOpt {
	return func(rule *v1.SkillAccessRule) {
		rule.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
}

func testUser(userID string, groups ...string) kuser.Info {
	return &kuser.DefaultInfo{
		UID: userID,
		Extra: map[string][]string{
			"auth_provider_groups": groups,
		},
	}
}
