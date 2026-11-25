package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gocache "k8s.io/client-go/tools/cache"
)

// TestSkillHandlerAgentGrantChangesApplyToExistingKeys checks that adding, changing and removing a skill access rule
// that names an agent changes which skills the agent lists and may get, for the same caller: an agent's key carries no
// grant, so a request made with a key issued before the change sees the change. The rules are in the indexer that the
// skill access rule informer keeps in the server, and are changed in place, as the informer changes them.
func TestSkillHandlerAgentGrantChangesApplyToExistingKeys(t *testing.T) {
	skills := []*v1.Skill{
		{
			Name: "sk-repo-1", Namespace: system.DefaultNamespace,
			Spec: v1.SkillSpec{
				SkillManifest: types.SkillManifest{Name: "repo-1-skill"},
				RepoID:        "repo-1",
			},
			Status: v1.SkillStatus{Valid: true},
		},
		{
			Name: "sk-repo-2", Namespace: system.DefaultNamespace,
			Spec: v1.SkillSpec{
				SkillManifest: types.SkillManifest{Name: "repo-2-skill"},
				RepoID:        "repo-2",
			},
			Status: v1.SkillStatus{Valid: true},
		},
	}
	storage := newFakeStorage(t, skills[0].DeepCopy(), skills[1].DeepCopy())
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, skillaccessrule.Indexers())
	helper := skillaccessrule.NewHelper(indexer)
	handler := NewSkillHandler(100, helper)

	// The caller is what the API key authenticator builds for a key of agent 7 with the Skills scope. It does not
	// change from one request to the next.
	agent := gclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, []string{types.GroupSkills, types.GroupAuthenticated}, nil)

	list := func(t *testing.T) []string {
		t.Helper()
		rec := httptest.NewRecorder()
		require.NoError(t, handler.List(api.Context{
			ResponseWriter: rec,
			Request:        httptest.NewRequest(http.MethodGet, "/api/skills", nil),
			Storage:        storage,
			User:           agent,
		}))
		var got types.SkillList
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		ids := make([]string, 0, len(got.Items))
		for _, skill := range got.Items {
			ids = append(ids, skill.ID)
		}
		return ids
	}
	// GET /api/skills/{id} is authorized with the same helper (authz checkSkill).
	canGet := func(t *testing.T, skill *v1.Skill) bool {
		t.Helper()
		ok, err := helper.UserHasAccessToSkill(agent, skill)
		require.NoError(t, err)
		return ok
	}
	requireAccess := func(t *testing.T, want ...string) {
		t.Helper()
		assert.ElementsMatch(t, want, list(t))
		for _, skill := range skills {
			assert.Equal(t, slices.Contains(want, skill.Name), canGet(t, skill), "access to %s", skill.Name)
		}
	}

	// No rule names the agent, and a rule for the user with its numeric ID grants it nothing.
	require.NoError(t, indexer.Add(newSkillRule("rule-user-7", []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
		[]types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-1"}})))
	requireAccess(t)

	rule := newSkillRule("rule-agent-7", []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
		[]types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-1"}})
	require.NoError(t, indexer.Add(rule))
	requireAccess(t, "sk-repo-1")

	// The rule now grants another repository.
	rule = rule.DeepCopy()
	rule.Spec.Manifest.Resources = []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo-2"}}
	require.NoError(t, indexer.Update(rule))
	requireAccess(t, "sk-repo-2")

	// The rule now names another agent.
	rule = rule.DeepCopy()
	rule.Spec.Manifest.Subjects = []types.Subject{{Type: types.SubjectTypeAgent, ID: "8"}}
	require.NoError(t, indexer.Update(rule))
	requireAccess(t)

	// A rule for every user grants an agent nothing.
	everyone := newSkillRule("rule-everyone", []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
		[]types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}})
	require.NoError(t, indexer.Add(everyone))
	requireAccess(t)

	// Naming the agent again grants it; removing that rule takes the grant away.
	named := newSkillRule("rule-every-skill-agent-7", []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
		[]types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}})
	require.NoError(t, indexer.Add(named))
	requireAccess(t, "sk-repo-1", "sk-repo-2")
	require.NoError(t, indexer.Delete(named))
	requireAccess(t)
}
