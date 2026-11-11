package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// gatewayTestModel returns an active LLM model of the LLM gateway, named name, with target.
func gatewayTestModel(name, target string) *v1.Model {
	return &v1.Model{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelSpec{
			Manifest: types.ModelManifest{
				Active:        true,
				TargetModel:   target,
				ModelProvider: system.LLMGatewayModelProvider,
				Usage:         types.ModelUsageLLM,
			},
		},
	}
}

// gatewayTestProfile returns a gateway profile (model access policy) that grants subject models.
func gatewayTestProfile(name string, subject types.Subject, models ...string) *v1.ModelAccessPolicy {
	resources := make([]types.ModelResource, 0, len(models))
	for _, m := range models {
		resources = append(resources, types.ModelResource{ID: m})
	}
	return &v1.ModelAccessPolicy{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelAccessPolicySpec{
			Manifest: types.ModelAccessPolicyManifest{
				Subjects: []types.Subject{subject},
				Models:   resources,
			},
		},
	}
}

// llmAgent returns the principal of agent id as the API key authenticator builds it for a key with the LLM scope only.
func llmAgent(id uint) kuser.Info {
	return client.AgentUserInfo(&gatewaytypes.Agent{ID: id}, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true}.Groups(nil), nil)
}

// proxyAs runs a POST of body to the gateway endpoint as caller, and returns the recorded response and the error the
// proxy returned.
func proxyAs(t *testing.T, proxy *llmProviderProxy, req api.Context, caller kuser.Info, endpoint, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req.User = caller
	req.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/"+endpoint, strings.NewReader(body))
	req.SetPathValue("path", "v1/"+endpoint)
	w := httptest.NewRecorder()
	req.ResponseWriter = w
	return w, proxy.proxy(req)
}

// requireStatus requires err to be an HTTP error with status.
func requireStatus(t *testing.T, err error, status int) {
	t.Helper()
	httpErr, ok := errors.AsType[*types.ErrHTTP](err)
	require.True(t, ok, "got %v, want an HTTP %d", err, status)
	require.Equal(t, status, httpErr.Code, httpErr.Message)
}

// listedModels runs a model listing response that lists targets through caller's model filter, and returns the
// targets the caller is shown.
func listedModels(t *testing.T, proxy *llmProviderProxy, caller kuser.Info, targets ...string) []string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/v1/models", nil)
	req.SetPathValue("path", "v1/models")
	data := make([]string, 0, len(targets))
	for _, target := range targets {
		data = append(data, `{"id":"`+target+`","object":"model"}`)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"object":"list","data":[` + strings.Join(data, ",") + `]}`)),
		Request:    req,
	}
	w := &llmResponseWriter{
		user:          caller,
		modelProvider: system.LLMGatewayModelProvider,
		mapHelper:     proxy.mapHelper,
	}
	require.NoError(t, w.prepareUnary(resp))
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var listed []string
	for _, id := range gjson.GetBytes(body, "data.#.id").Array() {
		listed = append(listed, id.String())
	}
	return listed
}

// TestAgentWithoutAGrantIsRefusedOnEveryModelPath checks each path that enforces model access in the LLM gateway with
// an agent that no profile grants the model, while the user with the agent's numeric ID is granted it: the agent is
// listed no models, and every way of reaching the model is refused.
func TestAgentWithoutAGrantIsRefusedOnEveryModelPath(t *testing.T) {
	proxy, req := directProxy(t, &directFixture{})
	user := req.User
	agent := llmAgent(7)
	backend := proxy.backend.(*bifrostProviderBackend)
	asAgent := req
	asAgent.User = agent
	asAgent.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/chat/completions", nil)

	t.Run("list", func(t *testing.T) {
		assert.Empty(t, listedModels(t, proxy, agent, "openai/test"))
		assert.Equal(t, []string{"openai/test"}, listedModels(t, proxy, user, "openai/test"))
	})

	t.Run("direct", func(t *testing.T) {
		const body = `{"model":"m1-test","messages":[{"role":"user","content":"hi"}]}`
		_, err := proxyAs(t, proxy, req, agent, "chat/completions", body)
		requireStatus(t, err, http.StatusForbidden)

		w, err := proxyAs(t, proxy, req, user, "chat/completions", body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("direct by target model", func(t *testing.T) {
		_, err := proxyAs(t, proxy, req, agent, "messages", `{"model":"openai/test","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
		requireStatus(t, err, http.StatusForbidden)
	})

	t.Run("resolve", func(t *testing.T) {
		// A bare provider-native name names no gateway model exactly, so the gateway resolves it. The resolved model is
		// still checked: resolving prefers a model the caller may use, but finds one they may not when that is all.
		model, err := backend.resolveModel(proxy, asAgent, "test")
		require.NoError(t, err)
		require.Equal(t, "m1-test", model.Name)
		_, err = proxyAs(t, proxy, req, agent, "chat/completions", `{"model":"test","messages":[{"role":"user","content":"hi"}]}`)
		requireStatus(t, err, http.StatusForbidden)
	})

	t.Run("authorize", func(t *testing.T) {
		for _, reference := range []string{"m1-test", "openai/test", "test"} {
			_, err := backend.authorizeModel(proxy, asAgent, reference, "model")
			requireStatus(t, err, http.StatusForbidden)
		}
	})

	t.Run("per attempt", func(t *testing.T) {
		// A routing rule or fallback can make the gateway try a model the request didn't name. Each attempt is checked.
		identity, isAdmin, err := proxy.gatewayIdentity(agent)
		require.NoError(t, err)
		attempts := &gatewayAttempts{budgets: map[string][]string{}, identity: identity, isAdmin: isAdmin}
		err = attempts.check(backend, proxy, asAgent, nil)(t.Context(), guardrails.Attempt{Model: "openai/test"})
		refused, ok := errors.AsType[*guardrails.AttemptError](err)
		require.True(t, ok, "got %v, want a refused attempt", err)
		require.Equal(t, http.StatusForbidden, refused.Status)
		served, _ := attempts.served()
		require.Empty(t, served)
		require.True(t, attempts.lastRefused())
	})

	t.Run("fallbacks", func(t *testing.T) {
		const body = `{"model":"m1-test","fallbacks":["openai/test",{"provider":"openai","model":"test"}]}`
		out, err := backend.authorizeExtraModels(proxy, asAgent, []byte(body))
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, gjson.GetBytes(out, "fallbacks").Raw, "an agent's fallbacks it may not use are dropped")

		asUser := asAgent
		asUser.User = user
		out, err = backend.authorizeExtraModels(proxy, asUser, []byte(body))
		require.NoError(t, err)
		assert.JSONEq(t, `["openai/test","openai/test"]`, gjson.GetBytes(out, "fallbacks").Raw)
	})
}

// TestAgentGetsTheModelsAndBudgetsOfEveryProfileThatMatchesIt checks an agent matched by an every-agent profile and by
// a profile that names it: it may use the union of their models and no model of a user profile, and a model both
// grant brings the budgets of both, including one that exempts administrators.
func TestAgentGetsTheModelsAndBudgetsOfEveryProfileThatMatchesIt(t *testing.T) {
	everyAgent := types.Subject{
		Type: types.SubjectTypeAgent,
		ID:   "*",
	}
	agentSeven := types.Subject{
		Type: types.SubjectTypeAgent,
		ID:   "7",
	}
	everyUser := types.Subject{
		Type: types.SubjectTypeUser,
		ID:   "*",
	}
	userSeven := types.Subject{
		Type: types.SubjectTypeUser,
		ID:   "7",
	}
	proxy, req := directProxyWith(t, &directFixture{},
		[]*v1.Model{
			gatewayTestModel("m1-a", "openai/a"),
			gatewayTestModel("m1-b", "openai/b"),
			gatewayTestModel("m1-shared", "openai/shared"),
			gatewayTestModel("m1-users", "openai/users"),
		},
		[]*v1.ModelAccessPolicy{
			gatewayTestProfile("agents", everyAgent, "m1-a", "m1-shared"),
			gatewayTestProfile("agent-7", agentSeven, "m1-b", "m1-shared"),
			gatewayTestProfile("users", everyUser, "m1-users"),
			gatewayTestProfile("user-7", userSeven, "m1-users"),
		},
	)
	agent := llmAgent(7)
	backend := proxy.backend.(*bifrostProviderBackend)
	asAgent := req
	asAgent.User = agent
	asAgent.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/chat/completions", nil)

	assert.ElementsMatch(t, []string{"openai/a", "openai/b", "openai/shared"},
		listedModels(t, proxy, agent, "openai/a", "openai/b", "openai/shared", "openai/users"))
	for _, model := range []string{"m1-a", "m1-b", "m1-shared"} {
		w, err := proxyAs(t, proxy, req, agent, "chat/completions", `{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`)
		require.NoError(t, err, model)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	_, err := proxyAs(t, proxy, req, agent, "chat/completions", `{"model":"m1-users","messages":[{"role":"user","content":"hi"}]}`)
	requireStatus(t, err, http.StatusForbidden)

	// The agent's identity has no groups, is never an administrator, and is in both of its profiles.
	identity, isAdmin, err := proxy.gatewayIdentity(agent)
	require.NoError(t, err)
	assert.False(t, isAdmin)
	assert.Equal(t, "agent:7", identity.UserID)
	assert.Empty(t, identity.Groups)
	assert.ElementsMatch(t, []string{"agents", "agent-7"}, identity.ProfileIDs)

	ctx := t.Context()
	budget := func(name, profile string, exemptAdmins bool) *gatewaytypes.LLMGatewayBudget {
		b := &gatewaytypes.LLMGatewayBudget{
			Name:         name,
			Enabled:      true,
			PerUser:      true,
			ExemptAdmins: exemptAdmins,
			Limits: []gatewaytypes.LLMGatewayBudgetLimit{
				{
					Metric: gatewaytypes.LLMGatewayBudgetMetricRequests,
					Amount: 1,
					Window: "1d",
				},
			},
			ProfileID: profile,
		}
		require.NoError(t, req.GatewayClient.CreateLLMGatewayBudget(ctx, b))
		return b
	}
	agentsBudget := budget("Agents", "agents", false)
	agentSevenBudget := budget("Agent 7", "agent-7", false)
	exemptBudget := budget("Exempts admins", "agent-7", true)
	usersBudget := budget("Users", "users", false)

	// An agent is never an administrator, so it is not exempt from a budget that exempts administrators.
	attempts := &gatewayAttempts{budgets: map[string][]string{}, identity: identity, isAdmin: isAdmin}
	check := attempts.check(backend, proxy, asAgent, nil)
	require.NoError(t, check(ctx, guardrails.Attempt{Model: "openai/shared"}))
	served, budgetIDs := attempts.served()
	assert.Equal(t, "openai/shared", served)
	assert.ElementsMatch(t, []string{agentsBudget.ID, agentSevenBudget.ID, exemptBudget.ID}, budgetIDs)
	assert.NotContains(t, budgetIDs, usersBudget.ID)

	require.NoError(t, check(ctx, guardrails.Attempt{Model: "openai/a"}))
	_, budgetIDs = attempts.served()
	assert.Equal(t, []string{agentsBudget.ID}, budgetIDs, "a model only the every-agent profile grants brings only its budgets")
}

// TestAgentBudgetUsageIsItsOwn checks that a request an agent's budgets admit is counted against the agent, so the
// user with the agent's numeric ID keeps their own counter.
func TestAgentBudgetUsageIsItsOwn(t *testing.T) {
	proxy, req := directProxyWith(t, &directFixture{},
		[]*v1.Model{
			gatewayTestModel("m1-a", "openai/a"),
		},
		[]*v1.ModelAccessPolicy{
			gatewayTestProfile("everyone", types.Subject{Type: types.SubjectTypeSelector, ID: "*"}, "m1-a"),
		},
	)
	ctx, c := t.Context(), req.GatewayClient
	backend := proxy.backend.(*bifrostProviderBackend)
	budget := &gatewaytypes.LLMGatewayBudget{
		Name:    "One request each",
		Enabled: true,
		PerUser: true,
		Limits: []gatewaytypes.LLMGatewayBudgetLimit{
			{
				Metric: gatewaytypes.LLMGatewayBudgetMetricRequests,
				Amount: 1,
				Window: "1d",
			},
		},
		ProfileID: "everyone",
	}
	require.NoError(t, c.CreateLLMGatewayBudget(ctx, budget))

	// serve admits one attempt of caller on model A and records its usage, as a served request does.
	serve := func(caller kuser.Info) error {
		identity, isAdmin, err := proxy.gatewayIdentity(caller)
		require.NoError(t, err)
		attempts := &gatewayAttempts{budgets: map[string][]string{}, identity: identity, isAdmin: isAdmin}
		apiReq := req
		apiReq.User = caller
		apiReq.Request = httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/chat/completions", nil)
		if err := attempts.check(backend, proxy, apiReq, nil)(ctx, guardrails.Attempt{Model: "openai/a"}); err != nil {
			return err
		}
		w := &llmResponseWriter{user: caller, client: c, gateway: attempts, modelID: "m1-a"}
		w.recordUsage(gatewaytypes.TokenUsage{InputTokens: 1, TotalTokens: 1})
		return nil
	}

	agent, user := llmAgent(7), req.User
	require.NoError(t, serve(agent))
	err := serve(agent)
	refused, ok := errors.AsType[*guardrails.AttemptError](err)
	require.True(t, ok, "the agent used its one request, got %v", err)
	require.Equal(t, http.StatusTooManyRequests, refused.Status)
	require.NoError(t, serve(user), "the agent's usage must not count against the user with its numeric ID")

	report, err := c.LLMGatewayBudgetUsageReport(ctx, budget)
	require.NoError(t, err)
	var owners []string
	for _, entry := range report {
		owners = append(owners, entry.UserID)
	}
	slices.Sort(owners)
	assert.Equal(t, []string{"7", "agent:7"}, owners, "a user's usage is keyed by their bare ID as before, an agent's by its principal")
}
