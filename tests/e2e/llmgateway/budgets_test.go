//go:build e2e

package llmgateway

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	"github.com/obot-platform/obot/tests/e2e/harness"
)

// profileProbeModel is granted only by test profiles, to tell when replicas apply them.
const profileProbeModel = "openai/gpt-e2e-secret"

// profileModels are the models test profiles grant besides the probe: the ones everyone may
// use. A budget covers only its own profile's models, so test profiles grant the models their
// budgets are tested with.
var (
	profileModels = []string{
		"openai/gpt-e2e-a", "openai/gpt-e2e-b", "openai/gpt-e2e-broken",
		"anthropic/claude-e2e-a", "anthropic/claude-e2e-b", "anthropic/claude-e2e-broken",
	}
	// defaultBudgetID is the daily token limit budget a new installation carries over to its
	// Default Policy.
	defaultBudgetID = "daily-token-limit-" + modelaccesspolicy.DefaultPolicyName
)

type budgetLimit struct {
	Metric   string  `json:"metric"`
	Amount   float64 `json:"amount"`
	Window   string  `json:"window"`
	Calendar bool    `json:"calendar,omitempty"`
}

type budget struct {
	ID              string        `json:"id,omitempty"`
	Name            string        `json:"name"`
	Enabled         bool          `json:"enabled"`
	PerUser         bool          `json:"perUser"`
	ExemptAdmins    bool          `json:"exemptAdmins"`
	UserTokenLimits bool          `json:"userTokenLimits,omitempty"`
	Limits          []budgetLimit `json:"limits"`
	Models          []string      `json:"models,omitempty"`
	ProfileID       string        `json:"profileID"`
}

// createBudget creates a budget through the admin API and deletes it when the test ends.
func createBudget(t *testing.T, b budget) budget {
	t.Helper()
	b.Name = b.Name + " " + harness.RandomSuffix()
	b.Enabled = true
	var created budget
	env.admin.Post(t, "/api/llm-gateway/budgets", b, &created)
	if created.ID == "" {
		t.Fatalf("budget created without an ID")
	}
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/llm-gateway/budgets/"+created.ID, nil)
	})
	return created
}

func users(us ...*testUser) []types.Subject {
	out := make([]types.Subject, 0, len(us))
	for _, u := range us {
		out = append(out, types.Subject{Type: types.SubjectTypeUser, ID: u.ID})
	}
	return out
}

// profileOf creates a gateway profile for the users and deletes it when the test ends.
// Budgets and guardrail policies apply to users through their profiles.
func profileOf(t *testing.T, us ...*testUser) string {
	t.Helper()
	return profileOfSubjects(t, users(us...), us...)
}

// profileOfSubjects creates a gateway profile for the subjects that grants the models everyone
// may use, and waits until every replica applies it to members.
func profileOfSubjects(t *testing.T, subjects []types.Subject, members ...*testUser) string {
	t.Helper()
	return profileGranting(t, subjects, profileModels, members...)
}

// profileGranting creates a gateway profile for the subjects that grants models, and waits
// until every replica applies it to members. Replicas match profiles from their own cache of
// them, so the profile also grants a model no one else has, and members must see it listed on
// each replica.
func profileGranting(t *testing.T, subjects []types.Subject, models []string, members ...*testUser) string {
	t.Helper()
	granted := []types.ModelResource{{ID: env.modelID(t, profileProbeModel)}}
	for _, m := range models {
		granted = append(granted, types.ModelResource{ID: env.modelID(t, m)})
	}
	for _, u := range members {
		if slices.Contains(listedModels(t, u.Gateway(env.A)), profileProbeModel) {
			t.Fatalf("%s is already in a test profile, so a new one can't be waited for; reuse that profile", u.ID)
		}
	}
	var created types.ModelAccessPolicy
	env.admin.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
		DisplayName: "e2e profile " + harness.RandomSuffix(),
		Subjects:    subjects,
		Models:      granted,
	}, &created)
	if created.ID == "" {
		t.Fatal("profile created without an ID")
	}
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/model-access-policies/"+created.ID, nil)
	})
	for _, u := range members {
		for _, r := range []*harness.Replica{env.A, env.B} {
			env.waitForModelAccess(t, u.Gateway(r), profileProbeModel)
		}
	}
	return created.ID
}

// expectAllowed makes a request that must succeed.
func expectAllowed(t *testing.T, u *testUser, r *harness.Replica, model, why string) {
	t.Helper()
	res := callLLM(t, u.Gateway(r), apiChat, model, "budget "+nonce("b"), false, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("%s: want 200 on %s, got %d: %s", why, r.Config.Name, res.Status, truncate(res.Body))
	}
}

// expectExhausted makes a request that must be refused by budget b.
func expectExhausted(t *testing.T, u *testUser, r *harness.Replica, model string, b budget, why string) *llmResult {
	t.Helper()
	n := nonce("over")
	res := callLLM(t, u.Gateway(r), apiChat, model, "budget "+n, false, nil)
	if res.Status != http.StatusTooManyRequests {
		t.Fatalf("%s: want 429 on %s, got %d: %s", why, r.Config.Name, res.Status, truncate(res.Body))
	}
	if !strings.Contains(res.Body, b.Name) {
		t.Fatalf("%s: the 429 doesn't name budget %q: %s", why, b.Name, res.Body)
	}
	if len(env.mock.InferenceRequests(n)) != 0 {
		t.Fatalf("%s: a request refused by a budget reached the provider", why)
	}
	return res
}

func TestBudgets(t *testing.T) {
	replicas := []*harness.Replica{env.A, env.B}

	t.Run("per-user request limit is exact across replicas", func(t *testing.T) {
		u, other := env.newUser(t, "budget-req"), env.newUser(t, "budget-req-other")
		const n = 6
		b := createBudget(t, budget{
			Name:    "e2e requests",
			PerUser: true,
			Limits:  []budgetLimit{{Metric: "requests", Amount: n, Window: "1h"}},
			// The other user is assigned too: per-user budgets give each their own allowance.
			ProfileID: profileOf(t, u, other),
		})
		for i := range n {
			expectAllowed(t, u, replicas[i%2], "openai/gpt-e2e-a", fmt.Sprintf("request %d of %d", i+1, n))
		}
		res := expectExhausted(t, u, replicas[n%2], "openai/gpt-e2e-a", b, "request over the limit")
		if !strings.Contains(res.Body, "6 of 6") {
			t.Errorf("want the 429 to report 6 of 6 requests used: %s", res.Body)
		}
		if res.Header.Get("Retry-After") == "" {
			t.Errorf("the 429 has no Retry-After header")
		}
		// Clients such as Claude Code would otherwise retry it until they give up.
		if got := res.Header.Get("X-Should-Retry"); got != "false" {
			t.Errorf("the 429 has X-Should-Retry %q, want false", got)
		}
		expectExhausted(t, u, replicas[(n+1)%2], "openai/gpt-e2e-a", b, "request over the limit on the other replica")
		// Anthropic clients get the error in their format.
		msg := callLLM(t, u.Gateway(env.A), apiMessages, "anthropic/claude-e2e-a", "hi", false, nil)
		if msg.Status != http.StatusTooManyRequests || msg.ErrorMessage == "" || !strings.Contains(msg.ErrorMessage, b.Name) {
			t.Errorf("want an Anthropic-format 429 naming the budget, got %d: %s", msg.Status, msg.Body)
		}
		if msg.Header.Get("Retry-After") == "" || msg.Header.Get("X-Should-Retry") != "false" {
			t.Errorf("the Anthropic-format 429 lacks Retry-After or X-Should-Retry: false: %v", msg.Header)
		}
		streamed := callLLM(t, u.Gateway(env.B), apiMessages, "anthropic/claude-e2e-a", "hi", true, nil)
		if streamed.Status != http.StatusTooManyRequests || streamed.Header.Get("X-Should-Retry") != "false" {
			t.Errorf("want a streaming Anthropic-format 429 with X-Should-Retry: false, got %d: %v", streamed.Status, streamed.Header)
		}
		// Obot's per-second API rate limit headers would say the user has requests left.
		for _, refusal := range []http.Header{res.Header, msg.Header, streamed.Header} {
			if refusal.Get("X-RateLimit-Remaining") != "" || refusal.Get("X-RateLimit-Limit") != "" || refusal.Get("X-RateLimit-Reset") != "" {
				t.Errorf("a budget 429 carries rate limit headers: %v", refusal)
			}
		}
		// The other user has their own allowance.
		expectAllowed(t, other, env.B, "openai/gpt-e2e-a", "another user's per-user allowance")

		var usage struct {
			Items []struct {
				UserID string    `json:"userID"`
				Used   []float64 `json:"used"`
			} `json:"items"`
		}
		env.admin.Get(t, "/api/llm-gateway/budgets/"+b.ID+"/usage", &usage)
		found := false
		for _, item := range usage.Items {
			if item.UserID == u.ID {
				found = true
				if len(item.Used) != 1 || item.Used[0] != n {
					t.Errorf("usage report for the user: %v, want [%d]", item.Used, n)
				}
			}
		}
		if !found {
			t.Errorf("usage report has no entry for the user: %+v", usage.Items)
		}
	})

	t.Run("shared budget across two users", func(t *testing.T) {
		u1, u2 := env.newUser(t, "budget-shared1"), env.newUser(t, "budget-shared2")
		b := createBudget(t, budget{
			Name:      "e2e shared",
			PerUser:   false,
			Limits:    []budgetLimit{{Metric: "requests", Amount: 4, Window: "1d"}},
			ProfileID: profileOf(t, u1, u2),
		})
		expectAllowed(t, u1, env.A, "openai/gpt-e2e-a", "u1 first")
		expectAllowed(t, u2, env.B, "openai/gpt-e2e-a", "u2 first")
		expectAllowed(t, u1, env.B, "anthropic/claude-e2e-a", "u1 second")
		expectAllowed(t, u2, env.A, "anthropic/claude-e2e-a", "u2 second")
		res := expectExhausted(t, u1, env.A, "openai/gpt-e2e-a", b, "u1 after the pool is used")
		if !strings.Contains(res.Body, "shared") {
			t.Errorf("want the 429 to say the budget is shared: %s", res.Body)
		}
		expectExhausted(t, u2, env.B, "openai/gpt-e2e-a", b, "u2 after the pool is used")
	})

	t.Run("token budget", func(t *testing.T) {
		u := env.newUser(t, "budget-tokens")
		// Each mock response reports 10 input and 5 output tokens.
		b := createBudget(t, budget{
			Name:      "e2e tokens",
			PerUser:   true,
			Limits:    []budgetLimit{{Metric: "tokens", Amount: 30, Window: "1d", Calendar: true}},
			ProfileID: profileOf(t, u),
		})
		expectAllowed(t, u, env.A, "openai/gpt-e2e-a", "0 of 30 tokens used")
		expectAllowed(t, u, env.B, "anthropic/claude-e2e-a", "15 of 30 tokens used")
		res := expectExhausted(t, u, env.A, "openai/gpt-e2e-a", b, "30 of 30 tokens used")
		if !strings.Contains(res.Body, "30 of 30") {
			t.Errorf("want the 429 to report 30 of 30 tokens: %s", res.Body)
		}
	})

	t.Run("output token budget counts streamed usage", func(t *testing.T) {
		u := env.newUser(t, "budget-output")
		b := createBudget(t, budget{
			Name:      "e2e output tokens",
			PerUser:   true,
			Limits:    []budgetLimit{{Metric: "output_tokens", Amount: 10, Window: "6h"}},
			ProfileID: profileOf(t, u),
		})
		for i, api := range []string{apiResponses, apiMessages} {
			callLLM(t, u.Gateway(replicas[i]), api, "openai/gpt-e2e-a", "stream "+nonce("s"), true, nil).requireOK(t)
		}
		expectExhausted(t, u, env.A, "openai/gpt-e2e-a", b, "10 of 10 output tokens used by streams")
	})

	t.Run("spend budget", func(t *testing.T) {
		u := env.newUser(t, "budget-spend")
		target := "anthropic/claude-e2e-b"
		id := env.modelID(t, target)
		// $0.10 per token: each request (10 in, 5 out) costs $1.50.
		var model types.Model
		env.admin.Get(t, "/api/models/"+id, &model)
		manifest := model.ModelManifest
		manifest.OverrideCost = &types.ModelCost{Input: 100000, Output: 100000}
		env.admin.Put(t, "/api/models/"+id, manifest, nil)
		t.Cleanup(func() {
			manifest.OverrideCost = nil
			env.admin.Put(t, "/api/models/"+id, manifest, nil)
		})
		for _, r := range replicas {
			harness.Eventually(t, 30*time.Second, "override cost on "+r.Config.Name, func() error {
				var m types.Model
				env.admin.On(r.BaseURL).Get(t, "/api/models/"+id, &m)
				if m.OverrideCost == nil || m.OverrideCost.Input != 100000 {
					return fmt.Errorf("override cost %+v", m.OverrideCost)
				}
				return nil
			})
		}

		b := createBudget(t, budget{
			Name:      "e2e spend",
			PerUser:   true,
			Limits:    []budgetLimit{{Metric: "spend", Amount: 3, Window: "1M", Calendar: true}},
			ProfileID: profileOf(t, u),
		})
		expectAllowed(t, u, env.A, target, "$0 of $3 spent")
		expectAllowed(t, u, env.B, target, "$1.50 of $3 spent")
		res := expectExhausted(t, u, env.A, target, b, "$3 of $3 spent")
		if !strings.Contains(res.Body, "$3.00 of $3.00") {
			t.Errorf("want the 429 to report $3.00 of $3.00: %s", res.Body)
		}
		var usage struct {
			Items []struct {
				Used []float64 `json:"used"`
			} `json:"items"`
		}
		env.admin.Get(t, "/api/llm-gateway/budgets/"+b.ID+"/usage", &usage)
		if len(usage.Items) != 1 || len(usage.Items[0].Used) != 1 || math.Abs(usage.Items[0].Used[0]-3) > 1e-9 {
			t.Errorf("spend usage report: %+v, want $3", usage.Items)
		}

		// The audit log shows what each request cost.
		harness.Eventually(t, 60*time.Second, "audit log entries with their spend", func() error {
			var logs struct {
				Items []struct {
					Spend          float64 `json:"spend"`
					ResponseStatus int     `json:"responseStatus"`
				} `json:"items"`
			}
			env.admin.Get(t, "/api/llm-audit-logs?hide_models_requests=true&user_id="+url.QueryEscape(u.ID)+"&limit=10", &logs)
			var spent []float64
			for _, item := range logs.Items {
				if item.ResponseStatus == http.StatusOK {
					spent = append(spent, item.Spend)
				}
			}
			if len(spent) != 2 || math.Abs(spent[0]-1.5) > 1e-9 || math.Abs(spent[1]-1.5) > 1e-9 {
				return fmt.Errorf("spend of the served requests %v, want $1.50 each", spent)
			}
			return nil
		})
	})

	t.Run("admin exemption", func(t *testing.T) {
		u := env.newUser(t, "budget-exempt")
		b := createBudget(t, budget{
			Name:         "e2e exempt admins",
			PerUser:      true,
			ExemptAdmins: true,
			Limits:       []budgetLimit{{Metric: "requests", Amount: 1, Window: "1h"}},
			ProfileID:    profileOf(t, env.adminUser, u),
		})
		for i := range 3 {
			expectAllowed(t, env.adminUser, replicas[i%2], "openai/gpt-e2e-a", "an exempt admin")
		}
		// A key that can only call the LLM gateway carries none of its user's role groups;
		// its admin is exempt all the same.
		var llmKey struct {
			Key string `json:"key"`
		}
		env.admin.Post(t, "/api/api-keys", map[string]any{
			"name":              "e2e-admin-llm-only-" + harness.RandomSuffix(),
			"canAccessLLMProxy": true,
		}, &llmKey)
		llmOnlyAdmin := *env.adminUser
		llmOnlyAdmin.Key = llmKey.Key
		for i := range 3 {
			expectAllowed(t, &llmOnlyAdmin, replicas[i%2], "openai/gpt-e2e-a", "an exempt admin with an LLM-only key")
		}
		expectAllowed(t, u, env.A, "openai/gpt-e2e-a", "a user's first request")
		expectExhausted(t, u, env.B, "openai/gpt-e2e-a", b, "a user's second request")

		// Without the exemption, the same admin is limited.
		nb := createBudget(t, budget{
			Name:      "e2e admins not exempt",
			PerUser:   true,
			Limits:    []budgetLimit{{Metric: "requests", Amount: 1, Window: "1h"}},
			Models:    []string{env.modelID(t, "openai/gpt-e2e-b")},
			ProfileID: b.ProfileID,
		})
		expectAllowed(t, env.adminUser, env.A, "openai/gpt-e2e-b", "an admin's first request")
		expectExhausted(t, env.adminUser, env.B, "openai/gpt-e2e-b", nb, "an admin who isn't exempt")
	})

	t.Run("a budget applies through its profile, and goes with it", func(t *testing.T) {
		u, other := env.newUser(t, "budget-profile"), env.newUser(t, "budget-profile-other")
		profileID := profileOf(t, u)
		b := createBudget(t, budget{
			Name:      "e2e profile",
			PerUser:   true,
			Limits:    []budgetLimit{{Metric: "requests", Amount: 1, Window: "1h"}},
			ProfileID: profileID,
		})
		expectAllowed(t, u, env.A, "openai/gpt-e2e-a", "first request in the profile")
		expectExhausted(t, u, env.B, "openai/gpt-e2e-a", b, "second request in the profile")
		expectAllowed(t, other, env.A, "openai/gpt-e2e-a", "a user outside the profile")

		env.admin.Do(t, http.MethodDelete, "/api/model-access-policies/"+profileID, nil)
		// The profile's finalizer deletes its budgets once the profile is marked for deletion.
		harness.Eventually(t, 30*time.Second, "the deleted profile's budget to be deleted", func() error {
			if res := env.admin.Do(t, http.MethodGet, "/api/llm-gateway/budgets/"+b.ID, nil); res.Status != http.StatusNotFound {
				return fmt.Errorf("deleting the profile should delete its budget, got %s", res)
			}
			return nil
		})
		deadline := time.Now().Add(30 * time.Second)
		for callLLM(t, u.Gateway(env.B), apiChat, "openai/gpt-e2e-a", "budget "+nonce("gone"), false, nil).Status != http.StatusOK {
			if time.Now().After(deadline) {
				t.Fatal("the deleted profile's budget still applies")
			}
			time.Sleep(200 * time.Millisecond)
		}
	})

	t.Run("a budget must name a profile that exists", func(t *testing.T) {
		for _, profileID := range []string{"", "no-such-profile"} {
			res := env.admin.Do(t, http.MethodPost, "/api/llm-gateway/budgets", budget{
				Name:      "e2e no profile",
				Limits:    []budgetLimit{{Metric: "requests", Amount: 1, Window: "1h"}},
				ProfileID: profileID,
			})
			if res.Status != http.StatusBadRequest {
				t.Fatalf("profile %q: want 400, got %s", profileID, res)
			}
		}
	})

	t.Run("a budget covers only its own profile's models", func(t *testing.T) {
		u := env.newUser(t, "budget-own-models")
		// The profile grants only the probe model; everyone may use the others through the
		// e2e everyone profile, which has no budget here.
		b := createBudget(t, budget{
			Name:      "e2e own models",
			PerUser:   true,
			Limits:    []budgetLimit{{Metric: "requests", Amount: 0, Window: "1h"}},
			ProfileID: profileGranting(t, users(u), nil, u),
		})
		for i := range 3 {
			expectAllowed(t, u, replicas[i%2], "openai/gpt-e2e-a", "a model the budget's profile doesn't grant")
		}
		expectExhausted(t, u, env.A, profileProbeModel, b, "the budget's profile's model")
	})

	t.Run("a budget can only name its profile's models", func(t *testing.T) {
		u := env.newUser(t, "budget-model-check")
		res := env.admin.Do(t, http.MethodPost, "/api/llm-gateway/budgets", budget{
			Name:      "e2e not granted",
			Limits:    []budgetLimit{{Metric: "requests", Amount: 1, Window: "1h"}},
			Models:    []string{env.modelID(t, "openai/gpt-e2e-vip")},
			ProfileID: profileOf(t, u),
		})
		if res.Status != http.StatusBadRequest {
			t.Fatalf("a model the profile doesn't grant: want 400, got %s", res)
		}
	})

	t.Run("model-scoped budget", func(t *testing.T) {
		u := env.newUser(t, "budget-model")
		b := createBudget(t, budget{
			Name:      "e2e model scoped",
			PerUser:   true,
			Limits:    []budgetLimit{{Metric: "requests", Amount: 1, Window: "1h"}},
			Models:    []string{env.modelID(t, "anthropic/claude-e2e-a")},
			ProfileID: profileOf(t, u),
		})
		expectAllowed(t, u, env.A, "anthropic/claude-e2e-a", "first request to the budgeted model")
		expectExhausted(t, u, env.B, "anthropic/claude-e2e-a", b, "second request to the budgeted model")
		// A bare model ID resolves to the same model and is limited too.
		expectExhausted(t, u, env.A, "claude-e2e-a", b, "the budgeted model by its bare ID")
		for i := range 3 {
			expectAllowed(t, u, replicas[i%2], "openai/gpt-e2e-a", "another model")
		}
	})

	t.Run("userTokenLimits uses personal daily limits", func(t *testing.T) {
		limited, unlimited, plain := env.newUser(t, "budget-personal"), env.newUser(t, "budget-personal-off"), env.newUser(t, "budget-personal-none")
		// Obot has no API for personal token limits; they are set on the user record.
		for id, limit := range map[string]int{limited.ID: 15, unlimited.ID: -1} {
			if _, err := env.cluster.PG.Exec(t.Context(), env.cluster.DB, fmt.Sprintf("update users set daily_input_tokens_limit = %d where id = %s", limit, id)); err != nil {
				t.Fatalf("set personal limit: %v", err)
			}
		}
		b := createBudget(t, budget{
			Name:            "e2e personal limits",
			PerUser:         true,
			UserTokenLimits: true,
			Limits:          []budgetLimit{{Metric: "input_tokens", Amount: 25, Window: "1d", Calendar: true}},
			ProfileID:       profileOf(t, limited, unlimited, plain),
		})
		// 15 personal input tokens: two requests of 10.
		expectAllowed(t, limited, env.A, "openai/gpt-e2e-a", "0 of 15 personal input tokens")
		expectAllowed(t, limited, env.B, "openai/gpt-e2e-a", "10 of 15 personal input tokens")
		// The carried-over "Daily token limit" budget on the Default Policy also uses personal
		// limits and may cover the model, so either budget may report it.
		res := callLLM(t, limited.Gateway(env.A), apiChat, "openai/gpt-e2e-a", "over "+nonce("p"), false, nil)
		if res.Status != http.StatusTooManyRequests || !strings.Contains(res.Body, "20 of 15 input tokens") ||
			!strings.Contains(res.Body, b.Name) && !strings.Contains(res.Body, "Daily token limit") {
			t.Fatalf("want a 429 for 20 of 15 personal input tokens, got %d: %s", res.Status, res.Body)
		}
		// A negative personal limit removes the budget's limit.
		for i := range 4 {
			expectAllowed(t, unlimited, replicas[i%2], "openai/gpt-e2e-a", "a user with no personal limit")
		}
		// Without a personal limit, the budget's own 25 applies: three requests of 10.
		for i := range 3 {
			expectAllowed(t, plain, replicas[i%2], "openai/gpt-e2e-a", "the budget's own limit")
		}
		expectExhausted(t, plain, env.B, "openai/gpt-e2e-a", b, "30 of 25 input tokens")
	})

	t.Run("migrated daily token limit budget exists", func(t *testing.T) {
		var list struct {
			Items []budget `json:"items"`
		}
		env.admin.Get(t, "/api/llm-gateway/budgets", &list)
		for _, b := range list.Items {
			if b.ID == defaultBudgetID {
				if b.Name != "Daily token limit" || b.ProfileID != modelaccesspolicy.DefaultPolicyName || !b.UserTokenLimits || !b.ExemptAdmins || !b.PerUser {
					t.Fatalf("default budget: %+v", b)
				}
				return
			}
		}
		t.Fatalf("no daily token limit budget on the Default Policy in %+v", list.Items)
	})
}
