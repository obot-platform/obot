//go:build e2e

package llmgateway

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/tests/e2e/harness"
)

const (
	// agentKeyCacheWindow is how long a replica caches a validated API key. A change to an agent or its key must take
	// effect on every replica well inside it, because each cache hit re-reads the key's and the agent's state.
	agentKeyCacheWindow = 15 * time.Second

	// agentVIPModel is granted to no agent by the setup policies: "e2e everyone" names subjects:*, which agents
	// match, but leaves it out, and "e2e vip" names one user. Tests grant it to an agent to see a grant change take
	// effect.
	agentVIPModel = "openai/gpt-e2e-vip"
)

// createAgent creates an agent through replica A and deletes it when the test ends.
func createAgent(t *testing.T, name string) types.AgentIdentity {
	t.Helper()
	var agent types.AgentIdentity
	env.admin.Post(t, "/api/agents", types.AgentIdentityManifest{DisplayName: name + " " + harness.RandomSuffix()}, &agent)
	if agent.ID == "" {
		t.Fatal("agent created without an ID")
	}
	t.Cleanup(func() {
		// The test may have deleted it already.
		env.admin.Do(t, http.MethodDelete, "/api/agents/"+agent.ID, nil)
	})
	return agent
}

// createAgentKey creates an API key for an agent through replica A.
func createAgentKey(t *testing.T, agentID string, req types.AgentAPIKeyCreateRequest) types.AgentAPIKeyCreateResponse {
	t.Helper()
	if req.Name == "" {
		req.Name = "e2e-" + harness.RandomSuffix()
	}
	var key types.AgentAPIKeyCreateResponse
	env.admin.Post(t, "/api/agents/"+agentID+"/api-keys", req, &key)
	if !strings.HasPrefix(key.Key, "oak1-"+agentID+"-") {
		t.Fatalf("agent key has an unexpected form: %q", key.Key)
	}
	return key
}

// createLLMAgentKey creates an agent's API key with the LLM proxy scope only.
func createLLMAgentKey(t *testing.T, agentID string) types.AgentAPIKeyCreateResponse {
	t.Helper()
	return createAgentKey(t, agentID, types.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
}

// agentGateways returns clients for an agent's key on replicas A and B.
func agentGateways(key string) (*harness.Client, *harness.Client) {
	return harness.NewAPIKeyClient(env.A.BaseURL, key), harness.NewAPIKeyClient(env.B.BaseURL, key)
}

// requireModelsStatus lists models once, with no retry, and fails unless the response has the wanted status.
func requireModelsStatus(t *testing.T, c *harness.Client, want int, why string) {
	t.Helper()
	resp := c.Do(t, http.MethodGet, "/api/llm-proxy/v1/models", nil)
	if resp.Status != want {
		t.Fatalf("%s: GET %s/api/llm-proxy/v1/models: want HTTP %d, got %s", why, c.BaseURL, want, resp)
	}
}

// requireChatStatus sends one chat request, with no retry, and fails unless it gets the wanted status. A refused
// request must not reach the provider, and an admitted one must reach it exactly once.
func requireChatStatus(t *testing.T, c *harness.Client, model string, want int, why string) {
	t.Helper()
	n := nonce("agent")
	res := callLLM(t, c, apiChat, model, "hi "+n, false, nil)
	if res.Status != want {
		t.Fatalf("%s: chat with %s on %s: want HTTP %d, got %d: %s", why, model, c.BaseURL, want, res.Status, truncate(res.Body))
	}
	reqs := env.mock.InferenceRequests(n)
	switch {
	case want == http.StatusOK && len(reqs) != 1:
		t.Fatalf("%s: want 1 upstream request, got %d", why, len(reqs))
	case want != http.StatusOK && len(reqs) != 0:
		t.Fatalf("%s: a refused request reached the provider", why)
	}
}

// TestAgentLifecycleAcrossReplicas checks that disabling, enabling and deleting an agent, and revoking its key, through
// replica A take effect on replica B at once, while B holds the key in its cache of validated keys.
func TestAgentLifecycleAcrossReplicas(t *testing.T) {
	agent := createAgent(t, "e2e lifecycle")
	key := createLLMAgentKey(t, agent.ID)
	gwA, gwB := agentGateways(key.Key)

	// Warm B's cache of validated keys, and check the agent is admitted on both replicas.
	requireModelsStatus(t, gwB, http.StatusOK, "warm on B")
	requireChatStatus(t, gwB, "openai/gpt-e2e-a", http.StatusOK, "warm on B")
	requireChatStatus(t, gwA, "openai/gpt-e2e-a", http.StatusOK, "active on A")

	// Each check is a single request sent right after A answered, so it is well inside B's cache window: B must not
	// serve a stale validation.
	changed := func(what string, change func()) time.Time {
		t.Helper()
		change()
		t.Logf("%s through A", what)
		return time.Now()
	}
	inWindow := func(since time.Time, what string) {
		t.Helper()
		if took := time.Since(since); took >= agentKeyCacheWindow {
			t.Fatalf("checking B after %s took %s, longer than the %s key cache window; the check proves nothing", what, took, agentKeyCacheWindow)
		}
	}

	t.Run("disable on A refuses on B", func(t *testing.T) {
		at := changed("disabled", func() {
			var disabled types.AgentIdentity
			env.admin.Post(t, "/api/agents/"+agent.ID+"/disable", types.AgentIdentityDisableRequest{Reason: "e2e cross-replica"}, &disabled)
			if disabled.Status != types.UserStatusDisabled {
				t.Fatalf("agent status after disable = %q", disabled.Status)
			}
		})
		requireModelsStatus(t, gwB, http.StatusForbidden, "disabled agent on B")
		requireChatStatus(t, gwB, "openai/gpt-e2e-a", http.StatusForbidden, "disabled agent on B")
		inWindow(at, "disable")
		requireModelsStatus(t, gwA, http.StatusForbidden, "disabled agent on A")
	})

	t.Run("enable on A admits on B", func(t *testing.T) {
		at := changed("enabled", func() {
			env.admin.Post(t, "/api/agents/"+agent.ID+"/enable", nil, nil)
		})
		requireModelsStatus(t, gwB, http.StatusOK, "enabled agent on B")
		requireChatStatus(t, gwB, "openai/gpt-e2e-a", http.StatusOK, "enabled agent on B")
		inWindow(at, "enable")
		requireChatStatus(t, gwA, "openai/gpt-e2e-a", http.StatusOK, "enabled agent on A")
	})

	t.Run("revoke on A refuses on B", func(t *testing.T) {
		// The key was just used on B, so B's cache holds it.
		at := changed("revoked the key", func() {
			env.admin.Delete(t, fmt.Sprintf("/api/agents/%s/api-keys/%d", agent.ID, key.ID))
		})
		requireModelsStatus(t, gwB, http.StatusUnauthorized, "revoked key on B")
		requireChatStatus(t, gwB, "openai/gpt-e2e-a", http.StatusUnauthorized, "revoked key on B")
		inWindow(at, "revoke")
		requireModelsStatus(t, gwA, http.StatusUnauthorized, "revoked key on A")
	})

	t.Run("delete on A refuses another key on B", func(t *testing.T) {
		other := createLLMAgentKey(t, agent.ID)
		otherA, otherB := agentGateways(other.Key)
		requireChatStatus(t, otherB, "openai/gpt-e2e-a", http.StatusOK, "warm the other key on B")
		at := changed("deleted the agent", func() {
			env.admin.Delete(t, "/api/agents/"+agent.ID)
		})
		requireModelsStatus(t, otherB, http.StatusUnauthorized, "deleted agent's key on B")
		requireChatStatus(t, otherB, "openai/gpt-e2e-a", http.StatusUnauthorized, "deleted agent's key on B")
		inWindow(at, "delete")
		requireModelsStatus(t, otherA, http.StatusUnauthorized, "deleted agent's key on A")
		// The revoked key stays refused.
		requireModelsStatus(t, gwB, http.StatusUnauthorized, "revoked key of a deleted agent on B")
	})
}

// waitForAgentModels waits until the agent's key lists the model on both replicas when listed is true, or on neither
// when it is false. Replicas apply model access policies from their own caches of them.
func waitForAgentModels(t *testing.T, key, target string, listed bool) {
	t.Helper()
	gwA, gwB := agentGateways(key)
	for _, c := range []*harness.Client{gwA, gwB} {
		harness.Eventually(t, time.Minute, fmt.Sprintf("%s listed=%t for the agent on %s", target, listed, c.BaseURL), func() error {
			if ids := listedModels(t, c); slices.Contains(ids, target) != listed {
				return fmt.Errorf("listed %v", ids)
			}
			return nil
		})
	}
}

// createAgentPolicy creates a model access policy for subjects that grants models, through replica A, and returns its
// ID. It is deleted when the test ends if the test has not deleted it.
func createAgentPolicy(t *testing.T, subjects []types.Subject, models ...string) string {
	t.Helper()
	granted := make([]types.ModelResource, 0, len(models))
	for _, m := range models {
		granted = append(granted, types.ModelResource{ID: env.modelID(t, m)})
	}
	var created types.ModelAccessPolicy
	env.admin.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
		DisplayName: "e2e agent " + harness.RandomSuffix(),
		Subjects:    subjects,
		Models:      granted,
	}, &created)
	if created.ID == "" {
		t.Fatal("model access policy created without an ID")
	}
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/model-access-policies/"+created.ID, nil)
	})
	return created.ID
}

// TestAgentGrantChanges checks that adding and removing a model access policy that names an agent changes what an
// existing key of the agent can do, on both replicas, without a new key.
func TestAgentGrantChanges(t *testing.T) {
	agent := createAgent(t, "e2e grants")
	key := createLLMAgentKey(t, agent.ID)
	gwA, gwB := agentGateways(key.Key)
	agentSubject := []types.Subject{{Type: types.SubjectTypeAgent, ID: agent.ID}}

	// Before: subjects:* grants the agent the everyday models, and nothing grants it the vip model.
	for _, c := range []*harness.Client{gwA, gwB} {
		if ids := listedModels(t, c); slices.Contains(ids, agentVIPModel) || !slices.Contains(ids, "openai/gpt-e2e-a") {
			t.Fatalf("before the grant, the agent's models on %s are %v", c.BaseURL, ids)
		}
		requireChatStatus(t, c, agentVIPModel, http.StatusForbidden, "before the grant")
	}

	policyID := createAgentPolicy(t, agentSubject, agentVIPModel)
	waitForAgentModels(t, key.Key, agentVIPModel, true)
	for _, c := range []*harness.Client{gwA, gwB} {
		requireChatStatus(t, c, agentVIPModel, http.StatusOK, "after the policy naming the agent was added")
	}

	// Changing the policy to name another agent takes the grant away as well.
	t.Run("policy edited to name someone else", func(t *testing.T) {
		other := createAgent(t, "e2e grants other")
		var policy types.ModelAccessPolicy
		env.admin.Get(t, "/api/model-access-policies/"+policyID, &policy)
		policy.Subjects = []types.Subject{{Type: types.SubjectTypeAgent, ID: other.ID}}
		env.admin.Put(t, "/api/model-access-policies/"+policyID, policy.ModelAccessPolicyManifest, nil)
		waitForAgentModels(t, key.Key, agentVIPModel, false)
		for _, c := range []*harness.Client{gwA, gwB} {
			requireChatStatus(t, c, agentVIPModel, http.StatusForbidden, "after the policy was changed to name another agent")
		}

		policy.Subjects = agentSubject
		env.admin.Put(t, "/api/model-access-policies/"+policyID, policy.ModelAccessPolicyManifest, nil)
		waitForAgentModels(t, key.Key, agentVIPModel, true)
		requireChatStatus(t, gwB, agentVIPModel, http.StatusOK, "after the policy named the agent again")
	})

	t.Run("policy deleted", func(t *testing.T) {
		env.admin.Delete(t, "/api/model-access-policies/"+policyID)
		waitForAgentModels(t, key.Key, agentVIPModel, false)
		for _, c := range []*harness.Client{gwA, gwB} {
			requireChatStatus(t, c, agentVIPModel, http.StatusForbidden, "after the policy naming the agent was deleted")
			// The grants subjects:* gives are untouched.
			requireChatStatus(t, c, "openai/gpt-e2e-a", http.StatusOK, "the everyday model after the policy was deleted")
		}
	})

	// agents:* grants every agent, and only agents: the vip model's only user is unaffected, and a user without it
	// stays without it.
	t.Run("agents wildcard", func(t *testing.T) {
		wildcardID := createAgentPolicy(t, []types.Subject{{Type: types.SubjectTypeAgent, ID: "*"}}, agentVIPModel)
		waitForAgentModels(t, key.Key, agentVIPModel, true)
		requireChatStatus(t, gwB, agentVIPModel, http.StatusOK, "granted by agents:*")
		u := env.newUser(t, "agent-wildcard")
		requireChatStatus(t, u.Gateway(env.B), agentVIPModel, http.StatusForbidden, "a user is not granted by agents:*")

		// It grants every agent, so it must be gone from both replicas before another test grants an agent the vip
		// model and waits for that.
		env.admin.Delete(t, "/api/model-access-policies/"+wildcardID)
		waitForAgentModels(t, key.Key, agentVIPModel, false)
	})
}

// TestAgentGuardrail checks that an LLM guardrail policy applied to a model access policy (profile) that names an
// agent blocks that agent's requests, and not those of someone the profile does not name.
func TestAgentGuardrail(t *testing.T) {
	agent := createAgent(t, "e2e guardrail")
	key := createLLMAgentKey(t, agent.ID)
	gwA, gwB := agentGateways(key.Key)

	// The profile grants the vip model, which nothing else grants the agent, so the test can tell when both replicas
	// apply it.
	profileID := createAgentPolicy(t, []types.Subject{{Type: types.SubjectTypeAgent, ID: agent.ID}}, append([]string{agentVIPModel}, profileModels...)...)
	waitForAgentModels(t, key.Key, agentVIPModel, true)

	secrets := createGuardrail(t, "secrets", map[string]any{"generic": false})
	message := "Blocked for agents by the e2e policy " + harness.RandomSuffix()
	policy := createPolicy(t, guardrailPolicy{Name: "e2e agent block", ApplyTo: "input", Action: "block", BlockMessage: message,
		GuardrailIDs: []string{secrets}, ProfileID: profileID})

	blocked := func(c *harness.Client) error {
		secret := githubToken()
		n := nonce("agentblock")
		res := callLLM(t, c, apiChat, "openai/gpt-e2e-a", n+" "+secret, false, nil)
		if res.Status != http.StatusBadRequest || !isGuardrailError(res.Body) || !strings.Contains(res.Body, message) {
			return fmt.Errorf("want HTTP 400 with the block message, got %d: %s", res.Status, truncate(res.Body))
		}
		if strings.Contains(res.Body, secret) {
			return fmt.Errorf("a blocked response contains the secret: %s", truncate(res.Body))
		}
		if len(env.mock.InferenceRequests(n)) != 0 {
			return fmt.Errorf("a blocked request reached the provider")
		}
		return nil
	}
	if err := blocked(gwA); err != nil {
		t.Fatalf("agent on A: %v", err)
	}
	harness.Eventually(t, reloadTimeout, "the guardrail blocks the agent on B", func() error { return blocked(gwB) })

	// The same request without the secret passes, so it was the guardrail that blocked it.
	requireChatStatus(t, gwA, "openai/gpt-e2e-a", http.StatusOK, "agent request without a secret")

	// A user whom the profile does not name is not blocked.
	u := env.newUser(t, "agent-guardrail-other")
	n := nonce("notblocked")
	callLLM(t, u.Gateway(env.A), apiChat, "openai/gpt-e2e-a", n+" "+githubToken(), false, nil).requireOK(t)
	if len(env.mock.InferenceRequests(n)) != 1 {
		t.Fatal("a user outside the agent's profile did not reach the provider")
	}

	// The audit log records the block against the agent.
	harness.Eventually(t, time.Minute, "a blocked finding of the agent's guardrail policy", func() error {
		resp := env.admin.Do(t, http.MethodGet, "/api/llm-audit-logs?hide_models_requests=true&agent_id="+url.QueryEscape(agent.ID)+"&limit=100", nil)
		if resp.Status != http.StatusOK {
			return fmt.Errorf("audit logs: %s", resp)
		}
		var list struct {
			Items []struct {
				GuardrailFindings []guardrailFinding `json:"guardrailFindings"`
			} `json:"items"`
		}
		resp.JSON(t, &list)
		for _, item := range list.Items {
			for _, f := range item.GuardrailFindings {
				if f.PolicyID == policy.ID && f.Outcome == "blocked" {
					return nil
				}
			}
		}
		return fmt.Errorf("no blocked finding of %s in %d entries", policy.ID, len(list.Items))
	})
}
