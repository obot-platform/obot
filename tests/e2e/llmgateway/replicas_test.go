//go:build e2e

package llmgateway

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/tests/e2e/harness"
)

// reloadTimeout bounds how long a replica may take to pick up a configuration change made
// on another replica ("within seconds").
const (
	reloadTimeout = 30 * time.Second
)

// TestMultiReplicaConfigReload changes gateway configuration on one replica and checks the
// other follows: providers and keys, model sync, routing rules, and key deletion.
func TestMultiReplicaConfigReload(t *testing.T) {
	tag := "e2ecustom" + harness.RandomSuffix()
	provider := tag
	model := tag + "-model"
	key1 := "e2e-upstream-" + tag + "-k1"
	env.mock.SetModels(key1, model)
	adminA, adminB := env.admin, env.admin.On(env.B.BaseURL)
	user := env.newUser(t, "replicas")

	parent := t
	t.Run("provider added on A routes on B", func(t *testing.T) {
		start := time.Now()
		addProvider(t, adminA, provider, env.mock.URL, key1, "openai")
		parent.Cleanup(func() {
			adminA.Do(parent, http.MethodDelete, "/api/llm-gateway/admin/providers/"+provider, nil)
		})

		// A model sync requested on B (run by whichever replica leads) finds the new
		// provider's model.
		env.waitForModels(t, adminB, provider+"/"+model)
		adminA.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
			DisplayName: "e2e " + tag,
			Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: user.ID}},
			Models:      []types.ModelResource{{ID: env.modelID(t, provider+"/"+model)}},
		}, nil)

		gwB := user.Gateway(env.B)
		harness.Eventually(t, reloadTimeout, "B routes to the provider added on A", func() error {
			n := nonce("reload")
			res := callLLM(t, gwB, apiChat, provider+"/"+model, "hi "+n, false, nil)
			if res.Status != http.StatusOK {
				return fmt.Errorf("HTTP %d: %s", res.Status, truncate(res.Body))
			}
			reqs := env.mock.InferenceRequests(n)
			if len(reqs) != 1 || reqs[0].Key != key1 || reqs[0].Model != model {
				return fmt.Errorf("upstream requests %+v", reqs)
			}
			return nil
		})
		t.Logf("B served the new provider %s after adding it on A", time.Since(start).Round(time.Millisecond))
	})

	t.Run("routing rule changed on B is followed by A", func(t *testing.T) {
		header := nonce("route-hdr")
		var created struct {
			Rule struct {
				ID string `json:"id"`
			} `json:"rule"`
		}
		adminA.Post(t, "/api/llm-gateway/admin/governance/routing-rules", map[string]any{
			"name":           "e2e-" + header,
			"enabled":        true,
			"cel_expression": fmt.Sprintf(`"x-e2e-route" in headers && headers["x-e2e-route"] == %q`, header),
			"targets":        []map[string]any{{"provider": "openai", "model": "gpt-e2e-b", "weight": 1}},
			"scope":          "global",
			"priority":       1,
		}, &created)
		ruleID := created.Rule.ID
		if ruleID == "" {
			t.Fatalf("routing rule was created without an ID")
		}
		t.Cleanup(func() {
			adminA.Do(t, http.MethodDelete, "/api/llm-gateway/admin/governance/routing-rules/"+ruleID, nil)
		})

		routedModel := func(r *harness.Replica) (string, error) {
			n := nonce("routed")
			res := callLLM(t, user.Gateway(r), apiChat, "openai/gpt-e2e-a", "hi "+n, false, nil, map[string]string{"X-E2E-Route": header})
			if res.Status != http.StatusOK {
				return "", fmt.Errorf("HTTP %d: %s", res.Status, truncate(res.Body))
			}
			reqs := env.mock.InferenceRequests(n)
			if len(reqs) != 1 {
				return "", fmt.Errorf("want 1 upstream request, got %d", len(reqs))
			}
			return reqs[0].Model, nil
		}
		waitRoutedTo := func(r *harness.Replica, want string) {
			t.Helper()
			harness.Eventually(t, reloadTimeout, r.Config.Name+" routes to "+want, func() error {
				got, err := routedModel(r)
				if err != nil {
					return err
				}
				if got != want {
					return fmt.Errorf("routed to %s", got)
				}
				return nil
			})
		}
		waitRoutedTo(env.A, "gpt-e2e-b")
		waitRoutedTo(env.B, "gpt-e2e-b")

		// Change the rule's target on B; A must follow.
		var rule map[string]any
		adminB.Get(t, "/api/llm-gateway/admin/governance/routing-rules/"+ruleID, &rule)
		if r, ok := rule["rule"].(map[string]any); ok {
			rule = r
		}
		rule["targets"] = []map[string]any{{"provider": "anthropic", "model": "claude-e2e-b", "weight": 1}}
		adminB.Put(t, "/api/llm-gateway/admin/governance/routing-rules/"+ruleID, rule, nil)
		waitRoutedTo(env.B, "claude-e2e-b")
		waitRoutedTo(env.A, "claude-e2e-b")
	})

	t.Run("key added on A and deleted on B stays deleted", func(t *testing.T) {
		key2 := "e2e-upstream-" + tag + "-k2"
		env.mock.SetModels(key2, model)
		key2Name := "k2-" + tag
		key2ID := addKey(t, adminA, provider, key2Name, key2)
		if key2ID == "" {
			t.Fatal("key was not created")
		}
		harness.Eventually(t, reloadTimeout, "B sees the key added on A", func() error {
			if findKeyID(t, adminB, provider, key2Name) == "" {
				return fmt.Errorf("not listed on B")
			}
			return nil
		})
		adminB.Delete(t, "/api/llm-gateway/admin/providers/"+provider+"/keys/"+key2ID)

		gone := func(r *harness.Replica) error {
			if findKeyID(t, env.admin.On(r.BaseURL), provider, key2Name) != "" {
				return fmt.Errorf("deleted key still listed on %s", r.Config.Name)
			}
			return nil
		}
		harness.Eventually(t, reloadTimeout, "A drops the key deleted on B", func() error { return gone(env.A) })

		// Another change on A makes every replica reload again; the deleted key must not be
		// resurrected from A's earlier in-memory configuration.
		key3Name := "k3-" + tag
		key3 := "e2e-upstream-" + tag + "-k3"
		env.mock.SetModels(key3, model)
		addKey(t, adminA, provider, key3Name, key3)
		harness.Eventually(t, reloadTimeout, "B sees the key added on A", func() error {
			if findKeyID(t, adminB, provider, key3Name) == "" {
				return fmt.Errorf("not listed on B")
			}
			return nil
		})
		for _, r := range []*harness.Replica{env.A, env.B} {
			if err := gone(r); err != nil {
				t.Fatal(err)
			}
		}

		// Requests through either replica never use the deleted key.
		since := len(env.mock.Requests(""))
		for i := range 10 {
			r := []*harness.Replica{env.A, env.B}[i%2]
			callLLM(t, user.Gateway(r), apiChat, provider+"/"+model, "hi "+nonce("keys"), false, nil).requireOK(t)
		}
		for _, req := range env.mock.Requests("")[since:] {
			if req.Key == key2 {
				t.Fatalf("a request used the deleted key: %+v", req.Path)
			}
		}

		// Both replicas agree on the provider's keys.
		keys := func(r *harness.Replica) []string {
			var list struct {
				Keys []struct {
					Name string `json:"name"`
				} `json:"keys"`
			}
			env.admin.On(r.BaseURL).Get(t, "/api/llm-gateway/admin/providers/"+provider+"/keys", &list)
			var names []string
			for _, k := range list.Keys {
				names = append(names, k.Name)
			}
			slices.Sort(names)
			return names
		}
		if a, b := keys(env.A), keys(env.B); !slices.Equal(a, b) {
			t.Fatalf("replicas disagree on keys: A=%v B=%v", a, b)
		}
	})

	t.Run("encryption key is shared", func(t *testing.T) {
		// Keys saved through A were encrypted with the generated key, and B decrypted them to
		// call the provider above. The key is stored once, in Obot's database.
		out, err := env.cluster.PG.Exec(t.Context(), env.cluster.DB, "select count(*) from credentials where context = 'llm-gateway' and name = 'encryption-key'")
		if err != nil {
			t.Fatalf("inspect the stored gateway key: %v", err)
		}
		if out != "1" {
			t.Fatalf("want one stored gateway encryption key, got %s", out)
		}
		for _, r := range []*harness.Replica{env.A, env.B} {
			if strings.Contains(r.Log(), "encryption key differs from the shared one") {
				t.Fatalf("%s has its own gateway encryption key", r.Config.Name)
			}
		}
	})
}
