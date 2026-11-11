package skillaccessrule

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

type ruleOpt func(*v1.SkillAccessRule)

func TestGetSkillAccessRulesForSkillFiltersDeletedAndNamespace(t *testing.T) {
	helper := newTestHelper(t,
		newRule("match", []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}),
		newRule("deleted", []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}, withDeletedTimestamp()),
		newRule("other-namespace", []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}, withNamespace("other")),
		newRule("other-skill", []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk2"}}),
	)

	rules, err := helper.GetSkillAccessRulesForSkill(system.DefaultNamespace, "sk1")
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "match", rules[0].Name)
}

func TestUserHasAccessToSkillID(t *testing.T) {
	user := testUser("eng")

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
				newRule("rule1", []types.Subject{
					{
						Type: types.SubjectTypeUser,
						ID:   "1",
					},
				}, []types.SkillResource{
					{
						Type: types.SkillResourceTypeSkill,
						ID:   "sk1",
					},
				}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name: "group repository grant",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{
					{
						Type: types.SubjectTypeGroup,
						ID:   "eng",
					},
				}, []types.SkillResource{
					{
						Type: types.SkillResourceTypeSkillRepository,
						ID:   "skr1",
					},
				}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name: "global wildcard grant",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{
					{
						Type: types.SubjectTypeSelector,
						ID:   "*",
					},
				}, []types.SkillResource{
					{
						Type: types.SkillResourceTypeSelector,
						ID:   "*",
					},
				}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name: "wildcard subject repo scoped grant",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{
					{
						Type: types.SubjectTypeSelector,
						ID:   "*",
					},
				}, []types.SkillResource{
					{
						Type: types.SkillResourceTypeSkillRepository,
						ID:   "skr1",
					},
				}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    true,
		},
		{
			name: "different skill does not grant access",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{
					{
						Type: types.SubjectTypeUser,
						ID:   "1",
					},
				}, []types.SkillResource{
					{
						Type: types.SkillResourceTypeSkill,
						ID:   "sk2",
					},
				}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    false,
		},
		{
			name: "different group does not grant access",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{
					{
						Type: types.SubjectTypeGroup,
						ID:   "ops",
					},
				}, []types.SkillResource{
					{
						Type: types.SkillResourceTypeSkillRepository,
						ID:   "skr1",
					},
				}),
			},
			skillID: "sk1",
			repoID:  "skr1",
			want:    false,
		},
		{
			name: "multiple matching rules still grant access",
			rules: []*v1.SkillAccessRule{
				newRule("rule1", []types.Subject{
					{
						Type: types.SubjectTypeGroup,
						ID:   "eng",
					},
				}, []types.SkillResource{
					{
						Type: types.SkillResourceTypeSkillRepository,
						ID:   "skr1",
					},
				}),
				newRule("rule2", []types.Subject{
					{
						Type: types.SubjectTypeUser,
						ID:   "1",
					},
				}, []types.SkillResource{
					{
						Type: types.SkillResourceTypeSkill,
						ID:   "sk1",
					},
				}),
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
		newRule("rule1", []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}),
	)

	hasAccess, err := helper.UserHasAccessToSkill(testUser(), &v1.Skill{
		Name: "sk1",
		Spec: v1.SkillSpec{RepoID: "skr1"},
	})
	require.NoError(t, err)
	assert.True(t, hasAccess)
}

func TestGetUserSkillAccessScopeAggregatesAndDeduplicates(t *testing.T) {
	helper := newTestHelper(t,
		newRule("wildcard-subject", []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-global"}}),
		newRule("user-rule", []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk-user"}}),
		newRule("group-rule", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-group"}}),
		newRule("duplicate-group-rule", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{
			{Type: types.SkillResourceTypeSkillRepository, ID: "repo-group"},
			{Type: types.SkillResourceTypeSkill, ID: "sk-group"},
		}),
	)

	allowAll, repoIDs, skillIDs, err := helper.GetUserSkillAccessScope(testUser("eng"))
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

func TestObotGroupSubjectGrantsSkillAccess(t *testing.T) {
	helper := newTestHelper(t,
		newRule("admin", []types.Subject{{Type: types.SubjectTypeObotGroup, ID: "admin"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk1"}}),
	)
	adminExtra := map[string][]string{"obot_groups": {"admin"}}
	principal.RecordKind(adminExtra, principal.KindUser)
	admin := &kuser.DefaultInfo{UID: "1", Extra: adminExtra}

	hasAccess, err := helper.UserHasAccessToSkillID(admin, "sk1", "")
	require.NoError(t, err)
	assert.True(t, hasAccess)

	hasAccess, err = helper.UserHasAccessToSkillID(testUser("admin"), "sk1", "")
	require.NoError(t, err)
	assert.False(t, hasAccess)

	_, _, skillIDs, err := helper.GetUserSkillAccessScope(admin)
	require.NoError(t, err)
	assert.Contains(t, skillIDs, "sk1")
}

func TestGetUserSkillAccessScopeAllowAll(t *testing.T) {
	helper := newTestHelper(t,
		newRule("rule1", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}}),
	)

	allowAll, repoIDs, skillIDs, err := helper.GetUserSkillAccessScope(testUser("eng"))
	require.NoError(t, err)
	assert.True(t, allowAll)
	assert.Empty(t, repoIDs)
	assert.Empty(t, skillIDs)
}

func newTestHelper(t *testing.T, rules ...*v1.SkillAccessRule) *Helper {
	t.Helper()

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, Indexers())

	for _, rule := range rules {
		require.NoError(t, indexer.Add(rule))
	}

	return NewHelper(indexer)
}

func newRule(name string, subjects []types.Subject, resources []types.SkillResource, opts ...ruleOpt) *v1.SkillAccessRule {
	rule := &v1.SkillAccessRule{
		Name:      name,
		Namespace: system.DefaultNamespace,
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

// testUser returns user 1, in the given auth provider groups, as an authenticator records them.
func testUser(groups ...string) kuser.Info {
	extra := map[string][]string{
		"auth_provider_groups": groups,
	}
	principal.RecordKind(extra, principal.KindUser)
	return &kuser.DefaultInfo{
		UID:   "1",
		Extra: extra,
	}
}

// testAgent returns the principal of the agent with id, as its authenticator records it, carrying the given extra
// values too.
func testAgent(id uint, extra map[string][]string) kuser.Info {
	extra = maps.Clone(extra)
	if extra == nil {
		extra = map[string][]string{}
	}
	principal.RecordKind(extra, principal.KindAgent)
	uid := principal.Agent(id).String()
	return &kuser.DefaultInfo{Name: uid, UID: uid, Extra: extra}
}

// TestAgentSkillAccess checks that agents get skills only through rules naming everyone (*), every agent, or the
// agent itself, and never through user or group subjects, even those of a user with the same numeric ID.
func TestAgentSkillAccess(t *testing.T) {
	skill := func(id string) []types.SkillResource {
		return []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: id}}
	}
	helper := newTestHelper(t,
		newRule("everyone", []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}, skill("sk-everyone")),
		newRule("users", []types.Subject{{Type: types.SubjectTypeUser, ID: "*"}}, skill("sk-users")),
		newRule("agents", []types.Subject{{Type: types.SubjectTypeAgent, ID: "*"}}, skill("sk-agents")),
		newRule("user-1", []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}}, skill("sk-user-1")),
		newRule("agent-1", []types.Subject{{Type: types.SubjectTypeAgent, ID: "1"}}, skill("sk-agent-1")),
		newRule("agent-2", []types.Subject{{Type: types.SubjectTypeAgent, ID: "2"}}, skill("sk-agent-2")),
		newRule("eng", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, skill("sk-eng")),
		newRule("admins", []types.Subject{{Type: types.SubjectTypeObotGroup, ID: "admin"}}, skill("sk-admins")),
	)
	groups := map[string][]string{"auth_provider_groups": {"eng"}, "obot_groups": {"admin"}}
	userExtra := maps.Clone(groups)
	principal.RecordKind(userExtra, principal.KindUser)

	for _, tt := range []struct {
		name string
		user kuser.Info
		want []string
	}{
		{
			name: "user 1 in groups",
			user: &kuser.DefaultInfo{UID: "1", Extra: userExtra},
			want: []string{"sk-admins", "sk-eng", "sk-everyone", "sk-user-1", "sk-users"},
		},
		{
			name: "agent 1 with groups on its user info",
			user: testAgent(1, groups),
			want: []string{"sk-agent-1", "sk-agents", "sk-everyone"},
		},
		{
			name: "a principal without a recorded kind",
			user: &kuser.DefaultInfo{UID: "1", Extra: groups},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var granted []string
			for _, id := range []string{"sk-everyone", "sk-users", "sk-agents", "sk-user-1", "sk-agent-1", "sk-agent-2", "sk-eng", "sk-admins"} {
				ok, err := helper.UserHasAccessToSkillID(tt.user, id, "")
				require.NoError(t, err)
				if ok {
					granted = append(granted, id)
				}
			}
			slices.Sort(granted)
			assert.Equal(t, tt.want, granted)

			// The scope lists the same rules.
			allowAll, _, skillIDs, err := helper.GetUserSkillAccessScope(tt.user)
			require.NoError(t, err)
			assert.False(t, allowAll)
			scoped := slices.Sorted(maps.Keys(skillIDs))
			if len(tt.want) == 0 {
				assert.Empty(t, scoped)
			} else {
				assert.Equal(t, tt.want, scoped)
			}
		})
	}
}
