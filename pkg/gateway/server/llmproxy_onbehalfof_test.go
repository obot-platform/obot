package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// llmAgentOnBehalfOf returns the principal of the agent with id, with a key with the LLM scope only, acting on behalf
// of the user with userID, in groups.
func llmAgentOnBehalfOf(t *testing.T, id, userID uint, groups []string) kuser.Info {
	t.Helper()

	info := client.AgentUserInfo(&gatewaytypes.Agent{ID: id}, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true}.Groups(nil), nil)
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

// TestLLMGatewayOnBehalfOfAUser checks that an agent acting on behalf of a user reaches the models of the agent's LLM
// gateway policies, and no other, that budgets apply through those policies only and count against the agent for all
// users together, that the user's own budgets never apply, and that guardrails apply through the policies that cover
// the user as well.
func TestLLMGatewayOnBehalfOfAUser(t *testing.T) {
	agentThree := types.Subject{Type: types.SubjectTypeAgent, ID: "3"}
	proxy, req := directProxyWith(t, &directFixture{},
		[]*v1.Model{
			gatewayTestModel("m1-a", "openai/a"),
			gatewayTestModel("m1-b", "openai/b"),
			gatewayTestModel("m1-users", "openai/users"),
		},
		[]*v1.ModelAccessPolicy{
			gatewayTestProfile("agent-a", agentThree, "m1-a"),
			gatewayTestProfile("agent-b", agentThree, "m1-b"),
			gatewayTestProfile("user-9", types.Subject{Type: types.SubjectTypeUser, ID: "9"}, "m1-users", "m1-a"),
			gatewayTestProfile("eng", types.Subject{Type: types.SubjectTypeGroup, ID: "eng"}, "m1-users"),
		},
	)
	ctx, c := t.Context(), req.GatewayClient
	backend := proxy.backend.(*bifrostProviderBackend)
	forNine := llmAgentOnBehalfOf(t, 3, 9, []string{"eng"})

	// Model access is the agent's, on every path.
	assert.Equal(t, []string{"openai/a", "openai/b"}, listedModels(t, proxy, forNine, "openai/a", "openai/b", "openai/users"))
	for _, model := range []string{"m1-a", "m1-b"} {
		w, err := proxyAs(t, proxy, req, forNine, "chat/completions", `{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	_, err := proxyAs(t, proxy, req, forNine, "chat/completions", `{"model":"m1-users","messages":[{"role":"user","content":"hi"}]}`)
	requireStatus(t, err, http.StatusForbidden)

	// The caller is the agent, never an administrator, with no groups. Guardrails apply through the agent's policies and
	// through those that cover the user; budgets do not.
	identity, isAdmin, err := proxy.gatewayIdentity(forNine)
	require.NoError(t, err)
	assert.False(t, isAdmin)
	assert.Equal(t, "agent:3", identity.UserID)
	assert.Empty(t, identity.Groups)
	assert.ElementsMatch(t, []string{"agent-a", "agent-b", "user-9", "eng"}, identity.ProfileIDs)

	budget := func(name, profile string) *gatewaytypes.LLMGatewayBudget {
		b := &gatewaytypes.LLMGatewayBudget{
			Name:    name,
			Enabled: true,
			PerUser: true,
			Limits: []gatewaytypes.LLMGatewayBudgetLimit{
				{
					Metric: gatewaytypes.LLMGatewayBudgetMetricRequests,
					Amount: 1,
					Window: "1d",
				},
			},
			ProfileID: profile,
		}
		require.NoError(t, c.CreateLLMGatewayBudget(ctx, b))
		return b
	}
	agentBudget := budget("Agent A", "agent-a")
	budget("Agent B", "agent-b")
	userBudget := budget("User 9", "user-9")

	serve := func(caller kuser.Info) ([]string, error) {
		identity, isAdmin, err := proxy.gatewayIdentity(caller)
		require.NoError(t, err)
		attempts := &gatewayAttempts{budgets: map[string][]string{}, identity: identity, isAdmin: isAdmin}
		apiReq := req
		apiReq.User = caller
		apiReq.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/chat/completions", nil)
		if err := attempts.check(backend, proxy, apiReq, nil)(ctx, guardrails.Attempt{Model: "openai/a"}); err != nil {
			return nil, err
		}
		_, budgetIDs := attempts.served()
		w := &llmResponseWriter{user: caller, client: c, gateway: attempts, modelID: "m1-a"}
		w.recordUsage(gatewaytypes.TokenUsage{InputTokens: 1, TotalTokens: 1})
		return budgetIDs, nil
	}

	budgetIDs, err := serve(forNine)
	require.NoError(t, err)
	assert.Equal(t, []string{agentBudget.ID}, budgetIDs, "only the budget of the agent's policy that grants the model applies, never the user's")

	// The agent's budget counts the requests it makes for every user together.
	_, err = serve(llmAgentOnBehalfOf(t, 3, 10, nil))
	refused, ok := errors.AsType[*guardrails.AttemptError](err)
	require.True(t, ok, "the agent used its one request, got %v", err)
	require.Equal(t, http.StatusTooManyRequests, refused.Status)

	// The user's own budget is untouched by what the agent did for them.
	report, err := c.LLMGatewayBudgetUsageReport(ctx, agentBudget)
	require.NoError(t, err)
	require.Len(t, report, 1)
	assert.Equal(t, "agent:3", report[0].UserID)
	report, err = c.LLMGatewayBudgetUsageReport(ctx, userBudget)
	require.NoError(t, err)
	assert.Empty(t, report)

	// Usage is attributed to the agent, on behalf of the user, and never to the user.
	activity := newRunTokenActivity(forNine, "m1-a", gatewaytypes.TokenUsage{TotalTokens: 1})
	assert.Empty(t, activity.UserID)
	assert.Equal(t, "agent", activity.PrincipalKind)
	assert.Equal(t, "3", activity.PrincipalID)
	assert.Equal(t, "9", activity.OnBehalfOf)
}
