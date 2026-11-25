package skillaccessrule

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// testOnBehalfOf returns the principal of agent 3 acting on behalf of user 1, who is in the group "eng".
func testOnBehalfOf(t *testing.T) kuser.Info {
	t.Helper()

	info := testAgent(3, nil)
	principal.RecordOnBehalfOf(info.GetExtra(), 1)
	require.NoError(t, agentprofile.RecordDelegation(info.GetExtra(), agentprofile.Delegation{
		ID:   "adl1test",
		User: subjects.Caller{Principal: principal.User(1), Groups: subjects.Groups{AuthProvider: map[string]struct{}{"eng": {}}}},
	}))
	return info
}

// TestSkillAccessOnBehalfOfAUser checks that an agent acting on behalf of a user is granted skills through every skill
// access rule of the agent's, and never through the user's.
func TestSkillAccessOnBehalfOfAUser(t *testing.T) {
	agent := []types.Subject{{Type: types.SubjectTypeAgent, ID: "3"}}
	helper := newTestHelper(t,
		newRule("agent-repo", agent, []types.SkillResource{{Type: types.SkillResourceTypeSkillRepository, ID: "repo1"}}),
		newRule("agent-skill", agent, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk2"}}),
		newRule("users", []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}, []types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}}),
		newRule("eng", []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}, []types.SkillResource{{Type: types.SkillResourceTypeSkill, ID: "sk3"}}),
	)

	caller := testOnBehalfOf(t)
	all, repos, skills, err := helper.GetUserSkillAccessScope(caller)
	require.NoError(t, err)
	assert.False(t, all, "the user's rule for every skill never applies to the agent")
	assert.Equal(t, map[string]struct{}{"repo1": {}}, repos)
	assert.Equal(t, map[string]struct{}{"sk2": {}}, skills)
	access := map[string]bool{"sk1": true, "sk2": true}
	for skill, repo := range map[string]string{"sk1": "repo1", "sk2": "repo2", "sk3": "repo3"} {
		ok, err := helper.UserHasAccessToSkillID(caller, skill, repo)
		require.NoError(t, err)
		assert.Equal(t, access[skill], ok, skill)
	}
}
