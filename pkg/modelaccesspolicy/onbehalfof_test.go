package modelaccesspolicy

import (
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// testOnBehalfOf returns the principal of the agent with agentID acting on behalf of the user with userID, in groups,
// as the authenticator of delegated tokens builds it.
func testOnBehalfOf(t *testing.T, agentID, userID uint, groups []string) *kuser.DefaultInfo {
	t.Helper()

	info := testAgent(agentID, nil)
	principal.RecordOnBehalfOf(info.Extra, userID)
	authProvider := map[string]struct{}{}
	for _, group := range groups {
		authProvider[group] = struct{}{}
	}
	require.NoError(t, agentprofile.RecordDelegation(info.Extra, agentprofile.Delegation{
		ID:   "adl1test",
		User: subjects.Caller{Principal: principal.User(userID), Groups: subjects.Groups{AuthProvider: authProvider}},
	}))
	return info
}

func testPolicy(name string, subjects []types2.Subject, models ...string) *v1.ModelAccessPolicy {
	policy := &v1.ModelAccessPolicy{
		Name:      name,
		Namespace: "default",
		Spec: v1.ModelAccessPolicySpec{Manifest: types2.ModelAccessPolicyManifest{
			Subjects: subjects,
		}},
	}
	for _, model := range models {
		policy.Spec.Manifest.Models = append(policy.Spec.Manifest.Models, types2.ModelResource{ID: model})
	}
	return policy
}

// TestUserPoliciesOnBehalfOfAUser checks that an agent acting on behalf of a user has every model access policy of the
// agent's, and none of the user's: model access and the profiles that budgets and guardrails apply through are the
// agent's own.
func TestUserPoliciesOnBehalfOfAUser(t *testing.T) {
	agent := []types2.Subject{{Type: types2.SubjectTypeAgent, ID: "3"}}
	h := newModelHelper(t,
		[]*v1.Model{
			newModel("m1-small", "llm-gateway-model-provider", "openai/small", true),
			newModel("m1-large", "llm-gateway-model-provider", "openai/large", true),
			newModel("m1-users", "llm-gateway-model-provider", "openai/users", true),
		},
		testPolicy("small", agent, "m1-small"),
		testPolicy("large", agent, "m1-large"),
		// The user's own policies, which the agent never gets through them.
		testPolicy("everyone", []types2.Subject{{Type: types2.SubjectTypeSelector, ID: types2.SubjectWildcardID}}, "m1-users"),
		testPolicy("ann", []types2.Subject{{Type: types2.SubjectTypeUser, ID: "7"}}, "m1-users"),
		testPolicy("eng", []types2.Subject{{Type: types2.SubjectTypeGroup, ID: "eng"}}, "m1-users"),
	)

	caller := testOnBehalfOf(t, 3, 7, []string{"eng"})
	ids, err := h.UserProfileIDs(caller)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"large", "small"}, ids)

	allowed, all, err := h.GetUserAllowedModels(caller)
	require.NoError(t, err)
	assert.False(t, all)
	models := map[string]bool{"m1-small": true, "m1-large": true}
	assert.Equal(t, models, allowed)
	for _, model := range []string{"m1-small", "m1-large", "m1-users"} {
		ok, err := h.UserHasAccessToModel(caller, model)
		require.NoError(t, err)
		assert.Equal(t, models[model], ok, model)

		// Budgets apply through the agent's policies that grant the model, only.
		budgetProfiles, err := h.UserProfileIDsForModel(caller, model)
		require.NoError(t, err)
		assert.Equal(t, models[model], len(budgetProfiles) > 0, model)
	}

	// The user's own policies are found for matching, such as for guardrails, apart from the agent's.
	policies, err := h.CallerPolicies(subjects.Caller{Principal: principal.User(7), Groups: subjects.Groups{AuthProvider: map[string]struct{}{"eng": {}}}})
	require.NoError(t, err)
	var names []string
	for _, policy := range policies {
		names = append(names, policy.Name)
	}
	assert.ElementsMatch(t, []string{"everyone", "ann", "eng"}, names)

	// The agent acting for itself has the same policies.
	ids, err = h.UserProfileIDs(testAgent(3, nil))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"small", "large"}, ids)
}
