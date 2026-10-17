//go:build e2e

// Package llmgatewayupgrade checks an upgrade from the main branch to this branch's LLM
// gateway: an installation with message policies and daily token limits is run on main,
// then two replicas of the new binary start on the same database at once.
//
// Run it with `make test-e2e-llm-gateway-upgrade`. It builds main in a temporary git
// worktree unless OBOT_E2E_MAIN_BINARY points at a main build.
package llmgatewayupgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/tests/e2e/harness"
	"github.com/tidwall/gjson"
)

const (
	dailyInput   = 5000
	dailyOutput  = 2000
	legacyAPIKey = "e2e-legacy-openai-key"
	// legacyAnthropicKey and legacyGenericKey are the legacy Anthropic and Generic Responses
	// providers' keys.
	legacyAnthropicKey = "e2e-legacy-anthropic-key"
	legacyGenericKey   = "e2e-legacy-generic-key"

	// fakeProviderDaemon is a provider daemon that serves one model: claude-haiku-4-5, an
	// undated alias, for the Anthropic provider, gpt-generic for the Generic Responses
	// provider, and gpt-legacy otherwise.
	fakeProviderDaemon = `package main

import (
	"net/http"
	"os"
)

func main() {
	model := "gpt-legacy"
	if os.Getenv("OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY") != "" {
		model = "claude-haiku-4-5"
	} else if os.Getenv("OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY") != "" {
		model = "gpt-generic"
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(` + "`" + `{"object":"list","data":[{"id":"` + "`" + `+model+` + "`" + `","object":"model","metadata":{"usage":"llm"}}]}` + "`" + `))
	})
	_ = http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), nil)
}
`
)

// budget is an LLM gateway budget as the API returns it.
type budget struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	PerUser         bool   `json:"perUser"`
	ExemptAdmins    bool   `json:"exemptAdmins"`
	UserTokenLimits bool   `json:"userTokenLimits"`
	Limits          []struct {
		Metric string  `json:"metric"`
		Amount float64 `json:"amount"`
		Window string  `json:"window"`
	} `json:"limits"`
	ProfileID string `json:"profileID"`
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// buildMain builds the main branch's obot binary in a temporary worktree.
func buildMain(t *testing.T, dir string) string {
	t.Helper()
	if p := os.Getenv("OBOT_E2E_MAIN_BINARY"); p != "" {
		return p
	}
	root := repoRoot()
	ref := os.Getenv("OBOT_E2E_MAIN_REF")
	if ref == "" {
		ref = "main"
	}
	src := filepath.Join(dir, "main-src")
	run := func(cwd string, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = cwd
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
	}
	run(root, "git", "worktree", "add", "--detach", src, ref)
	t.Cleanup(func() {
		cmd := exec.Command("git", "worktree", "remove", "--force", src)
		cmd.Dir = root
		_ = cmd.Run()
	})
	out := filepath.Join(dir, "obot-main")
	start := time.Now()
	run(src, "go", "build", "-tags", "kustomize_disable_go_plugin_support", "-o", out, ".")
	t.Logf("built %s in %s", ref, time.Since(start).Round(time.Second))
	return out
}

func TestUpgradeFromMain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	dir := os.Getenv("OBOT_E2E_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	t.Logf("replica logs and data in %s", dir)
	branch := os.Getenv("OBOT_E2E_BINARY")
	if branch == "" {
		branch = filepath.Join(repoRoot(), "bin", "obot")
	}
	branchBinary, err := harness.PrepareBinary(branch, filepath.Join(dir, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	mainBinary, err := harness.PrepareBinary(buildMain(t, dir), filepath.Join(dir, "bin-main"))
	if err != nil {
		t.Fatal(err)
	}

	mock, err := harness.StartMockLLM()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	cluster, err := harness.NewCluster(ctx, harness.ClusterOptions{
		Name:             "obot-e2e-upgrade-" + harness.RandomSuffix(),
		Dir:              dir,
		PretendContainer: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cluster.Close(); err != nil {
			t.Logf("cleanup: %v", err)
		}
		// Logs and data stay for inspection; the binary copies are large.
		for _, sub := range []string{"bin", "bin-main", "obot-main"} {
			_ = os.RemoveAll(filepath.Join(dir, sub))
		}
	}()

	// Obot images ship a provider registry; this one has only the model providers whose
	// credentials the gateway imports, with a daemon that lists one model and does nothing else,
	// so legacy credentials can be configured.
	registry := writeProviderRegistry(t, filepath.Join(dir, "provider-registry"))
	limits := []string{
		"--daily-user-input-token-limit", fmt.Sprint(dailyInput),
		"--daily-user-output-token-limit", fmt.Sprint(dailyOutput),
		"--provider-registries", registry,
	}
	// main only serves the message policy API with the feature on. The upgraded version has no
	// such setting.
	policiesOn := map[string]string{"OBOT_SERVER_ENABLE_MESSAGE_POLICIES": "true"}

	// --- The old installation, on main. ---
	// main syncs model prices from models.dev unless told not to; the upgrade removes it.
	old, err := cluster.StartReplica(ctx, "main", mainBinary, append(slices.Clone(limits), "--model-info-source-url", ""), policiesOn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := cluster.ActivateAdmin(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	userEmail, userPassword := "legacy-user@e2e.example.com", "E2e-Legacy-Passw0rd!"
	admin.Post(t, "/api/local-auth/users", map[string]any{"email": userEmail, "password": userPassword, "requirePasswordChange": false}, nil)
	user, err := harness.Login(ctx, old.BaseURL, userEmail, userPassword)
	if err != nil {
		t.Fatal(err)
	}
	var me types.User
	user.Get(t, "/api/me", &me)
	// Obot has no API for personal token limits; they are set on the user record.
	if _, err := cluster.PG.Exec(ctx, cluster.DB, fmt.Sprintf("update users set daily_input_tokens_limit = 15 where id = %s", me.ID)); err != nil {
		t.Fatal(err)
	}

	type policy struct {
		manifest types.MessagePolicyManifest
		id       string
		applyTo  string
	}
	policies := []*policy{
		{manifest: types.MessagePolicyManifest{DisplayName: "No travel booking", Definition: "Do not help book travel above economy class.",
			Direction: types.PolicyDirectionUserMessage, Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}},
			applyTo: "input"},
		{manifest: types.MessagePolicyManifest{DisplayName: "No destructive tools", Definition: "Never call tools that delete data.",
			Direction: types.PolicyDirectionToolCalls, Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: me.ID}}},
			applyTo: "output"},
		{manifest: types.MessagePolicyManifest{DisplayName: "No secrets either way", Definition: "Do not reveal credentials.",
			Direction: types.PolicyDirectionBoth, Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "e2e-group"}}},
			applyTo: "both"},
	}
	for _, p := range policies {
		var created types.MessagePolicy
		admin.Post(t, "/api/message-policies", p.manifest, &created)
		p.id = created.ID
	}

	// Legacy model provider credentials, which the gateway imports on its first start. The
	// test controls main's binary and the registry, so failing to set them up fails the test
	// rather than skipping the import and migration checks.
	legacyModelID := configureLegacyProvider(t, admin, system.OpenAIModelProvider, "gpt-legacy", map[string]string{
		"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": legacyAPIKey,
	})
	// Anthropic's provider listed undated aliases, which the gateway's lists only in their
	// dated form.
	legacyHaikuID := configureLegacyProvider(t, admin, system.AnthropicModelProvider, "claude-haiku-4-5", map[string]string{
		"OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY": legacyAnthropicKey,
	})
	// The Generic Responses provider's base URL included the API version.
	legacyGenericID := configureLegacyProvider(t, admin, system.GenericResponsesModelProvider, "gpt-generic", map[string]string{
		"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": mock.URL + "/v1",
		"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY":  legacyGenericKey,
	})

	// The default model aliases and access policies refer to the legacy providers' models,
	// which the gateway must move them off.
	admin.Put(t, "/api/default-model-aliases/llm", types.DefaultModelAliasManifest{Alias: "llm", Model: legacyModelID}, nil)
	admin.Put(t, "/api/default-model-aliases/llm-mini", types.DefaultModelAliasManifest{Alias: "llm-mini", Model: legacyHaikuID}, nil)
	admin.Put(t, "/api/default-model-aliases/vision", types.DefaultModelAliasManifest{Alias: "vision", Model: legacyGenericID}, nil)
	admin.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
		DisplayName: "legacy model",
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
		Models:      []types.ModelResource{{ID: legacyModelID}},
	}, nil)
	admin.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
		DisplayName: "legacy aliases",
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
		Models:      []types.ModelResource{{ID: legacyHaikuID}, {ID: legacyGenericID}},
	}, nil)

	// The profiles that exist at the upgrade, each of which gets a copy of the daily token
	// limit budget.
	var oldProfiles struct {
		Items []types.ModelAccessPolicy `json:"items"`
	}
	admin.Get(t, "/api/model-access-policies", &oldProfiles)
	var legacyProfileID string
	for _, p := range oldProfiles.Items {
		if p.DisplayName == "legacy model" {
			legacyProfileID = p.ID
		}
	}
	if len(oldProfiles.Items) < 2 || legacyProfileID == "" {
		t.Fatalf("want the Default Policy and the legacy model policy on main, got %+v", oldProfiles.Items)
	}

	// A request a message policy changed, as main recorded it in the LLM audit log.
	if _, err := cluster.PG.Exec(ctx, cluster.DB, fmt.Sprintf(`insert into llm_audit_logs (id, created_at, user_id, model_provider, target_model, request_path, request_method, message_policy_triggered, policy_modified_request_body, encrypted)
		values ('e2e-violation', now(), '%s', '%s', 'gpt-legacy', '/api/llm-proxy/openai/v1/chat/completions', 'POST', true, convert_to('{"prompt":"blocked"}', 'UTF8'), false)`, me.ID, system.OpenAIModelProvider)); err != nil {
		t.Fatal(err)
	}

	if err := old.Stop(); err != nil {
		t.Fatal(err)
	}
	if lines := harness.PanicLines(old.Log()); len(lines) > 0 {
		t.Fatalf("main crashed:\n%s", strings.Join(lines, "\n"))
	}

	// --- The upgrade: two replicas of the new binary at once. ---
	var (
		wg       sync.WaitGroup
		replicas = make([]*harness.Replica, 2)
		errs     = make([]error, 2)
	)
	for i := range replicas {
		wg.Go(func() {
			replicas[i], errs[i] = cluster.StartReplica(ctx, fmt.Sprintf("upgraded-%d", i), branchBinary, limits, nil)
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	admin, err = harness.Login(ctx, replicas[0].BaseURL, harness.AdminEmail, harness.AdminPassword)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range replicas {
		a := admin.On(r.BaseURL)
		harness.Eventually(t, 2*time.Minute, "gateway ready on "+r.Config.Name, func() error {
			resp := a.Do(t, http.MethodGet, "/api/llm-gateway/status", nil)
			if !gjson.GetBytes(resp.Body, "ready").Bool() {
				return fmt.Errorf("status %s", resp)
			}
			return nil
		})
	}

	t.Run("daily token limit budget on every profile", func(t *testing.T) {
		var list struct {
			Items []budget `json:"items"`
		}
		admin.Get(t, "/api/llm-gateway/budgets", &list)
		// One copy on each profile that existed at the upgrade, and none on the profiles the
		// message policy conversion created.
		if len(list.Items) != len(oldProfiles.Items) {
			t.Fatalf("want one budget per pre-existing profile (%d), got %d: %+v", len(oldProfiles.Items), len(list.Items), list.Items)
		}
		for _, p := range oldProfiles.Items {
			var copies []budget
			for _, b := range list.Items {
				if b.ProfileID == p.ID {
					copies = append(copies, b)
				}
			}
			if len(copies) != 1 {
				t.Fatalf("profile %q (%s): want one daily token limit budget, got %+v", p.DisplayName, p.ID, copies)
			}
			b := copies[0]
			if b.Name != "Daily token limit" || !b.Enabled || !b.PerUser || !b.ExemptAdmins || !b.UserTokenLimits {
				t.Fatalf("profile %q: unexpected budget: %+v", p.DisplayName, b)
			}
			got := map[string]float64{}
			for _, l := range b.Limits {
				if l.Window != "1d" {
					t.Errorf("profile %q: limit %s has window %s, want 1d", p.DisplayName, l.Metric, l.Window)
				}
				got[l.Metric] = l.Amount
			}
			if got["input_tokens"] != dailyInput || got["output_tokens"] != dailyOutput || len(got) != 2 {
				t.Fatalf("profile %q: budget limits %v, want input %d and output %d", p.DisplayName, got, dailyInput, dailyOutput)
			}
		}
	})

	t.Run("message policies became LLM judge guardrails", func(t *testing.T) {
		var guardrailList struct {
			Items []struct {
				ID     string          `json:"id"`
				Name   string          `json:"name"`
				Type   string          `json:"type"`
				Config json.RawMessage `json:"config"`
			} `json:"items"`
		}
		var guardrailPolicies struct {
			Items []struct {
				ID           string   `json:"id"`
				Name         string   `json:"name"`
				Enabled      bool     `json:"enabled"`
				FailClosed   bool     `json:"failClosed"`
				Target       string   `json:"target"`
				ApplyTo      string   `json:"applyTo"`
				Action       string   `json:"action"`
				GuardrailIDs []string `json:"guardrailIDs"`
				ProfileID    string   `json:"profileID"`
			} `json:"items"`
		}
		admin.Get(t, "/api/llm-gateway/guardrails", &guardrailList)
		admin.Get(t, "/api/llm-gateway/guardrail-policies", &guardrailPolicies)
		if len(guardrailPolicies.Items) != len(policies) || len(guardrailList.Items) != len(policies) {
			t.Fatalf("want %d guardrail policies and guardrails, got %d guardrail policies and %d guardrails", len(policies), len(guardrailPolicies.Items), len(guardrailList.Items))
		}
		for _, p := range policies {
			pi := slices.IndexFunc(guardrailList.Items, func(x struct {
				ID     string          `json:"id"`
				Name   string          `json:"name"`
				Type   string          `json:"type"`
				Config json.RawMessage `json:"config"`
			}) bool {
				return x.Name == p.manifest.DisplayName
			})
			if pi < 0 {
				t.Fatalf("no guardrail for policy %q", p.manifest.DisplayName)
			}
			guardrail := guardrailList.Items[pi]
			if guardrail.Type != "llm-judge" || gjson.GetBytes(guardrail.Config, "definition").String() != p.manifest.Definition {
				t.Fatalf("guardrail for %q: %+v %s", p.manifest.DisplayName, guardrail, guardrail.Config)
			}
			found := false
			for _, r := range guardrailPolicies.Items {
				if r.Name != p.manifest.DisplayName {
					continue
				}
				found = true
				// Message policies blocked when the judge failed, so the guardrail policies fail closed. They
				// start disabled, for an administrator to review and enable.
				if r.Enabled || !r.FailClosed || r.Target != "llm" || r.Action != "block" || r.ApplyTo != p.applyTo ||
					len(r.GuardrailIDs) != 1 || r.GuardrailIDs[0] != guardrail.ID {
					t.Fatalf("guardrail policy for %q: %+v", p.manifest.DisplayName, r)
				}
				// The guardrail policy applies to the policy's subjects, through a profile.
				subjects := profileSubjects(t, admin, r.ProfileID)
				everyone := types.Subject{Type: types.SubjectTypeSelector, ID: "*"}
				if slices.Contains(p.manifest.Subjects, everyone) {
					if !slices.Contains(subjects, everyone) {
						t.Fatalf("guardrail policy for %q: its profile %q doesn't apply to everyone: %+v", p.manifest.DisplayName, r.ProfileID, subjects)
					}
				} else if mustJSON(subjects) != mustJSON(p.manifest.Subjects) {
					t.Fatalf("guardrail policy for %q: its profile %q has subjects %+v, want %+v", p.manifest.DisplayName, r.ProfileID, subjects, p.manifest.Subjects)
				}
			}
			if !found {
				t.Fatalf("no guardrail policy for message policy %q", p.manifest.DisplayName)
			}
		}
		// The message policy API is gone; LLM judge guardrails replace it.
		if resp := admin.Do(t, http.MethodGet, "/api/message-policies", nil); resp.Status != http.StatusNotFound && resp.Status != http.StatusForbidden {
			t.Errorf("listing message policies: want the API gone, got %s", resp)
		}
	})

	t.Run("legacy provider credentials were imported", func(t *testing.T) {
		imports := map[string]string{
			"openai":            system.OpenAIModelProvider,
			"anthropic":         system.AnthropicModelProvider,
			"generic-responses": system.GenericResponsesModelProvider,
		}
		for _, r := range replicas {
			a := admin.On(r.BaseURL)
			for provider, legacy := range imports {
				harness.Eventually(t, time.Minute, "imported provider "+provider+" on "+r.Config.Name, func() error {
					var keys struct {
						Keys []struct {
							Name  string          `json:"name"`
							Value json.RawMessage `json:"value"`
						} `json:"keys"`
					}
					resp := a.Do(t, http.MethodGet, "/api/llm-gateway/admin/providers/"+provider+"/keys", nil)
					if resp.Status != http.StatusOK {
						return fmt.Errorf("keys: %s", resp)
					}
					resp.JSON(t, &keys)
					var names []string
					for _, k := range keys.Keys {
						names = append(names, k.Name)
					}
					if len(names) != 1 || names[0] != "imported-"+legacy {
						return fmt.Errorf("want one imported key, got %v", names)
					}
					return nil
				})
			}
		}
		// The gateway adds the API version to a provider's base URL itself, so the version the
		// legacy provider's base URL included is left out.
		resp := admin.Do(t, http.MethodGet, "/api/llm-gateway/admin/providers/generic-responses", nil)
		if got := gjson.GetBytes(resp.Body, "network_config.base_url").String(); got != mock.URL {
			t.Errorf("imported Generic Responses base URL %q, want %q: %s", got, mock.URL, resp)
		}
	})

	t.Run("references to the legacy model moved to the gateway", func(t *testing.T) {
		// The imported provider still talks to OpenAI with a fake key, so the gateway can't
		// list gpt-legacy yet. Until it does, the legacy model the alias and policy name must
		// survive, so the references don't break. The import has finished (the subtest above),
		// so a model sync now runs the migration with deletion enabled.
		admin.Post(t, "/api/llm-gateway/sync-models", map[string]any{}, nil)
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
			if resp := admin.Do(t, http.MethodGet, "/api/models/"+legacyModelID, nil); resp.Status != http.StatusOK {
				t.Fatalf("the referenced legacy model was deleted before the gateway served its model: %s", resp)
			}
			var alias types.DefaultModelAlias
			admin.Get(t, "/api/default-model-aliases/llm", &alias)
			if alias.Model != legacyModelID {
				t.Fatalf("llm alias %q before the gateway served the model, want the legacy model %s", alias.Model, legacyModelID)
			}
		}

		// Now point the imported provider at the mock, which serves the same model for the
		// imported key. Moving a provider removes its keys, so the key is entered again.
		mock.SetModels(legacyAPIKey, "gpt-legacy")
		updateProvider(t, admin, "openai", map[string]any{"base_url": mock.URL, "allow_private_network": true}, legacyAPIKey)
		admin.Post(t, "/api/llm-gateway/sync-models", map[string]any{}, nil)

		var gatewayModelID string
		harness.Eventually(t, 2*time.Minute, "the llm alias on the gateway model", func() error {
			var list types.ModelList
			admin.Get(t, "/api/models?all=true", &list)
			for _, m := range list.Items {
				if m.TargetModel == "openai/gpt-legacy" {
					gatewayModelID = m.ID
				}
			}
			var alias types.DefaultModelAlias
			admin.Get(t, "/api/default-model-aliases/llm", &alias)
			if gatewayModelID == "" || alias.Model != gatewayModelID {
				return fmt.Errorf("gateway model %q, llm alias %q", gatewayModelID, alias.Model)
			}
			return nil
		})
		var policies struct {
			Items []types.ModelAccessPolicy `json:"items"`
		}
		admin.Get(t, "/api/model-access-policies", &policies)
		found := false
		for _, p := range policies.Items {
			if p.DisplayName != "legacy model" {
				continue
			}
			found = true
			if len(p.Models) != 1 || p.Models[0].ID != gatewayModelID {
				t.Errorf("access policy models %v, want the gateway model %s", p.Models, gatewayModelID)
			}
		}
		if !found {
			t.Error("the access policy referring to the legacy model is gone")
		}
		harness.Eventually(t, time.Minute, "the legacy model and provider to be deleted", func() error {
			if resp := admin.Do(t, http.MethodGet, "/api/models/"+legacyModelID, nil); resp.Status != http.StatusNotFound {
				return fmt.Errorf("legacy model: %s", resp)
			}
			if resp := admin.Do(t, http.MethodGet, "/api/model-providers/"+system.OpenAIModelProvider, nil); resp.Status != http.StatusNotFound {
				return fmt.Errorf("legacy provider: %s", resp)
			}
			return nil
		})
	})

	t.Run("undated Anthropic aliases and Generic Responses models moved to the gateway", func(t *testing.T) {
		// Point the imported providers at the mock, which lists Anthropic's models only in
		// their dated form, as Anthropic does. The Generic Responses provider keeps its base URL.
		mock.SetModels(legacyAnthropicKey, "claude-haiku-4-5-20251001", "claude-upgrade")
		mock.SetModels(legacyGenericKey, "gpt-generic")
		updateProvider(t, admin, "anthropic", map[string]any{"base_url": mock.URL, "allow_private_network": true}, legacyAnthropicKey)
		updateProvider(t, admin, "generic-responses", map[string]any{"allow_private_network": true}, "")
		admin.Post(t, "/api/llm-gateway/sync-models", map[string]any{}, nil)

		gatewayModels := map[string]string{}
		harness.Eventually(t, 2*time.Minute, "the aliases on the gateway models", func() error {
			var list types.ModelList
			admin.Get(t, "/api/models?all=true", &list)
			for _, m := range list.Items {
				if m.ModelProvider == system.LLMGatewayModelProvider {
					gatewayModels[m.TargetModel] = m.ID
				}
			}
			for alias, target := range map[string]string{
				"llm-mini": "anthropic/claude-haiku-4-5-20251001",
				"vision":   "generic-responses/gpt-generic",
			} {
				var a types.DefaultModelAlias
				admin.Get(t, "/api/default-model-aliases/"+alias, &a)
				if gatewayModels[target] == "" || a.Model != gatewayModels[target] {
					return fmt.Errorf("%s alias %q, want the gateway model for %s (%q)", alias, a.Model, target, gatewayModels[target])
				}
			}
			return nil
		})
		var policies struct {
			Items []types.ModelAccessPolicy `json:"items"`
		}
		admin.Get(t, "/api/model-access-policies", &policies)
		i := slices.IndexFunc(policies.Items, func(p types.ModelAccessPolicy) bool { return p.DisplayName == "legacy aliases" })
		if i < 0 {
			t.Fatal("the access policy referring to the legacy aliases is gone")
		}
		want := []string{gatewayModels["anthropic/claude-haiku-4-5-20251001"], gatewayModels["generic-responses/gpt-generic"]}
		var got []string
		for _, m := range policies.Items[i].Models {
			got = append(got, m.ID)
		}
		if !slices.Equal(got, want) {
			t.Errorf("access policy models %v, want the gateway models %v", got, want)
		}
		harness.Eventually(t, time.Minute, "the legacy models and providers to be deleted", func() error {
			for _, id := range []string{legacyHaikuID, legacyGenericID} {
				if resp := admin.Do(t, http.MethodGet, "/api/models/"+id, nil); resp.Status != http.StatusNotFound {
					return fmt.Errorf("legacy model %s: %s", id, resp)
				}
			}
			for _, provider := range []string{system.AnthropicModelProvider, system.GenericResponsesModelProvider} {
				if resp := admin.Do(t, http.MethodGet, "/api/model-providers/"+provider, nil); resp.Status != http.StatusNotFound {
					return fmt.Errorf("legacy provider %s: %s", provider, resp)
				}
			}
			return nil
		})

		// Clients set up before the upgrade keep sending the undated alias, and the Generic
		// Responses provider's requests reach its upstream's /v1 API, not /v1/v1.
		var key struct {
			Key string `json:"key"`
		}
		admin.Post(t, "/api/api-keys", map[string]any{"name": "aliases", "canAccessLLMProxy": true}, &key)
		gw := harness.NewAPIKeyClient(replicas[1].BaseURL, key.Key)
		for _, tc := range []struct {
			path, model, upstreamModel, upstreamPath string
		}{
			{path: "/api/llm-proxy/v1/messages", model: "claude-haiku-4-5", upstreamModel: "claude-haiku-4-5-20251001", upstreamPath: "/v1/messages"},
			{path: "/api/llm-proxy/anthropic/v1/messages", model: "anthropic/claude-haiku-4-5", upstreamModel: "claude-haiku-4-5-20251001", upstreamPath: "/v1/messages"},
			{path: "/api/llm-proxy/v1/chat/completions", model: "generic-responses/gpt-generic", upstreamModel: "gpt-generic", upstreamPath: "/v1/chat/completions"},
		} {
			n := "alias-" + harness.RandomSuffix()
			body := map[string]any{"model": tc.model, "max_tokens": 10, "messages": []any{map[string]any{"role": "user", "content": "hi " + n}}}
			harness.Eventually(t, time.Minute, tc.model+" on "+tc.path, func() error {
				if resp := gw.Do(t, http.MethodPost, tc.path, body, map[string]string{"anthropic-version": "2023-06-01"}); resp.Status != http.StatusOK {
					return fmt.Errorf("%s", resp)
				}
				return nil
			})
			reqs := mock.InferenceRequests(n)
			if len(reqs) == 0 || reqs[0].Model != tc.upstreamModel || reqs[0].Path != tc.upstreamPath {
				t.Errorf("%s on %s: want upstream %s %s, got %+v", tc.model, tc.path, tc.upstreamPath, tc.upstreamModel, reqs)
			}
		}
	})

	t.Run("personal token limit applies through a profile's carried-over budget", func(t *testing.T) {
		// The imported Anthropic provider serves the mock's models (the subtest above).
		a := admin
		a.Post(t, "/api/llm-gateway/sync-models", map[string]any{}, nil)
		var modelID string
		harness.Eventually(t, 2*time.Minute, "the mock model in Obot", func() error {
			var list types.ModelList
			a.Get(t, "/api/models?all=true", &list)
			for _, m := range list.Items {
				if m.TargetModel == "anthropic/claude-upgrade" {
					modelID = m.ID
					return nil
				}
			}
			return fmt.Errorf("not synced")
		})
		// The user reaches the model through a profile that existed at the upgrade, and so
		// has the carried-over budget, and through one created since, which has none.
		var legacyProfile types.ModelAccessPolicy
		a.Get(t, "/api/model-access-policies/"+legacyProfileID, &legacyProfile)
		legacyProfile.Models = append(legacyProfile.Models, types.ModelResource{ID: modelID})
		a.Put(t, "/api/model-access-policies/"+legacyProfileID, legacyProfile.ModelAccessPolicyManifest, nil)
		var newProfile types.ModelAccessPolicy
		a.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
			DisplayName: "upgrade",
			Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			Models:      []types.ModelResource{{ID: modelID}},
		}, &newProfile)
		var newBudgets struct {
			Items []budget `json:"items"`
		}
		a.Get(t, "/api/llm-gateway/budgets?profileID="+newProfile.ID, &newBudgets)
		for _, b := range newBudgets.Items {
			if b.ProfileID == newProfile.ID {
				t.Fatalf("a profile created after the upgrade got a budget: %+v", b)
			}
		}
		var key struct {
			Key string `json:"key"`
		}
		session, err := harness.Login(ctx, replicas[1].BaseURL, userEmail, userPassword)
		if err != nil {
			t.Fatal(err)
		}
		session.Post(t, "/api/api-keys", map[string]any{"name": "upgrade", "canAccessLLMProxy": true}, &key)
		call := func(r *harness.Replica) *harness.Response {
			return harness.NewAPIKeyClient(r.BaseURL, key.Key).Do(t, http.MethodPost, "/api/llm-proxy/v1/chat/completions", map[string]any{
				"model": "anthropic/claude-upgrade", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
			})
		}
		// Converted guardrail policies start disabled. Once an administrator enables the one converted for
		// everyone, it fails closed, as message policies did, and no judge model is configured
		// here, so it blocks.
		var guardrailPolicies struct {
			Items []map[string]any `json:"items"`
		}
		a.Get(t, "/api/llm-gateway/guardrail-policies", &guardrailPolicies)
		for _, r := range guardrailPolicies.Items {
			if r["name"] == "No travel booking" {
				r["enabled"] = true
				a.Put(t, "/api/llm-gateway/guardrail-policies/"+fmt.Sprint(r["id"]), r, nil)
			}
		}
		harness.Eventually(t, time.Minute, "the converted message policy to block", func() error {
			resp := call(replicas[0])
			if resp.Status != http.StatusBadRequest || !strings.Contains(string(resp.Body), "guardrail_intervention") ||
				!strings.Contains(string(resp.Body), "No travel booking") {
				return fmt.Errorf("%s", resp)
			}
			return nil
		})
		// Disable the converted guardrail policies to reach the budget.
		a.Get(t, "/api/llm-gateway/guardrail-policies", &guardrailPolicies)
		for _, r := range guardrailPolicies.Items {
			r["enabled"] = false
			a.Put(t, "/api/llm-gateway/guardrail-policies/"+fmt.Sprint(r["id"]), r, nil)
		}
		for _, r := range replicas {
			harness.Eventually(t, time.Minute, "a gateway request on "+r.Config.Name, func() error {
				if resp := call(r); resp.Status != http.StatusOK {
					return fmt.Errorf("%s", resp)
				}
				return nil
			})
		}
		// Two requests of 10 input tokens use up the personal limit of 15.
		resp := call(replicas[0])
		if resp.Status != http.StatusTooManyRequests || !strings.Contains(string(resp.Body), "Daily token limit") || !strings.Contains(string(resp.Body), "of 15") {
			t.Fatalf("want the legacy model profile's carried-over budget to apply the personal limit of 15, got %s", resp)
		}
	})

	t.Run("past message policy violations aren't shown", func(t *testing.T) {
		// Message policies are gone; what they did to past requests stays in the database but
		// isn't shown.
		resp := admin.Do(t, http.MethodGet, "/api/llm-audit-logs?limit=1000&user_id="+me.ID, nil)
		entry := gjson.GetBytes(resp.Body, `items.#(id=="e2e-violation")`)
		if resp.Status != http.StatusOK || !entry.Exists() {
			t.Fatalf("the audit entry made before the upgrade: %s", resp)
		}
		detail := admin.Do(t, http.MethodGet, "/api/llm-audit-logs/detail/e2e-violation", nil)
		if detail.Status != http.StatusOK {
			t.Fatalf("audit entry detail: %s", detail)
		}
		for name, body := range map[string]string{"list": entry.Raw, "detail": string(detail.Body)} {
			for _, field := range []string{"messagePolicyTriggered", "policyModifiedRequestBody"} {
				if gjson.Get(body, field).Exists() {
					t.Errorf("the audit entry's %s shows the message policy's %s: %s", name, field, body)
				}
			}
		}
	})

	t.Run("no replica crashed", func(t *testing.T) {
		for _, r := range replicas {
			if r.Exited() {
				t.Errorf("%s exited; log %s", r.Config.Name, r.LogPath)
			}
			if lines := harness.PanicLines(r.Log()); len(lines) > 0 {
				t.Errorf("%s crashed:\n%s", r.Config.Name, strings.Join(lines, "\n"))
			}
		}
		out, err := cluster.PG.Exec(ctx, cluster.DB, "select count(*) from migrations where name in ('llm_gateway_profile_budgets', 'guardrails_message_policies')")
		if err != nil {
			t.Logf("could not read migration markers: %v", err)
		} else if out != "2" {
			t.Errorf("want both migration markers recorded once, got %s", out)
		}
	})
}

// updateProvider changes a gateway provider's network settings, keeping the rest of its
// configuration. Moving a provider to another base URL removes its keys, so rekey, when set,
// is the key value to enter again afterwards.
func updateProvider(t *testing.T, admin *harness.Client, name string, network map[string]any, rekey string) {
	t.Helper()
	var provider map[string]any
	admin.Get(t, "/api/llm-gateway/admin/providers/"+name, &provider)
	nc, _ := provider["network_config"].(map[string]any)
	if nc == nil {
		nc = map[string]any{}
	}
	maps.Copy(nc, network)
	update := map[string]any{"network_config": nc}
	for _, k := range []string{"keys", "concurrency_and_buffer_size", "proxy_config", "custom_provider_config"} {
		if v, ok := provider[k]; ok {
			update[k] = v
		}
	}
	path := "/api/llm-gateway/admin/providers/" + name
	if rekey != "" {
		path += "?removeKeys=true"
	}
	admin.Put(t, path, update, nil)
	if rekey != "" {
		admin.Post(t, "/api/llm-gateway/admin/providers/"+name+"/keys", map[string]any{
			"name": name + "-rekeyed", "value": rekey, "models": []string{"*"}, "weight": 1,
		}, nil)
	}
}

// configureLegacyProvider configures a legacy model provider on main and returns the ID of
// the model it lists, target.
func configureLegacyProvider(t *testing.T, admin *harness.Client, provider, target string, credential map[string]string) string {
	t.Helper()
	harness.Eventually(t, time.Minute, "the legacy provider "+provider+" from the registry", func() error {
		if resp := admin.Do(t, http.MethodGet, "/api/model-providers/"+provider, nil); resp.Status != http.StatusOK {
			return fmt.Errorf("%s", resp)
		}
		return nil
	})
	if resp := admin.Do(t, http.MethodPost, "/api/model-providers/"+provider+"/configure", credential); resp.Status >= 300 {
		t.Fatalf("could not configure the legacy provider %s on main: %s", provider, resp)
	}
	harness.Eventually(t, 90*time.Second, "the legacy provider "+provider+" to be configured", func() error {
		var mp struct {
			Configured bool `json:"configured"`
		}
		admin.Get(t, "/api/model-providers/"+provider, &mp)
		if !mp.Configured {
			return fmt.Errorf("not configured")
		}
		return nil
	})
	var id string
	harness.Eventually(t, time.Minute, "the legacy model "+target+" on main", func() error {
		var list types.ModelList
		admin.Get(t, "/api/models?all=true", &list)
		for _, m := range list.Items {
			if m.ModelProvider == provider && m.TargetModel == target {
				id = m.ID
				return nil
			}
		}
		return fmt.Errorf("not synced")
	})
	return id
}

// writeProviderRegistry writes a provider registry with the OpenAI, Anthropic and Generic
// Responses model providers and an auth provider, all run by fakeProviderDaemon.
func writeProviderRegistry(t *testing.T, dir string) string {
	t.Helper()
	for _, sub := range []string{"model-providers", "auth-providers", "daemon-src"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(dir, "daemon-src", "main.go")
	if err := os.WriteFile(src, []byte(fakeProviderDaemon), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "fake-provider"), src)
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GO111MODULE=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the fake provider daemon: %v\n%s", err, out)
	}
	files := map[string]string{
		"model-providers/openai-model-provider.yaml": `name: OpenAI
command: fake-provider
description: e2e stand-in for the OpenAI model provider
requiredConfigurationParameters:
  - name: OBOT_OPENAI_MODEL_PROVIDER_API_KEY
    friendlyName: API Key
    sensitive: true
`,
		"model-providers/anthropic-model-provider.yaml": `name: Anthropic
command: fake-provider
description: e2e stand-in for the Anthropic model provider
requiredConfigurationParameters:
  - name: OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY
    friendlyName: API Key
    sensitive: true
`,
		"model-providers/generic-responses-model-provider.yaml": `name: Generic Responses
command: fake-provider
description: e2e stand-in for the Generic Responses model provider
requiredConfigurationParameters:
  - name: OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL
    friendlyName: Base URL
  - name: OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY
    friendlyName: API Key
    sensitive: true
`,
		"auth-providers/e2e-auth-provider.yaml": `name: E2E Auth
command: fake-provider
description: e2e stand-in auth provider, never configured
groupIDPrefix: "e2e/"
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// profileSubjects returns the subjects of a gateway profile (a model access policy).
func profileSubjects(t *testing.T, admin *harness.Client, id string) []types.Subject {
	t.Helper()
	if id == "" {
		t.Fatal("no profile")
	}
	var profile types.ModelAccessPolicy
	admin.Get(t, "/api/model-access-policies/"+id, &profile)
	return profile.Subjects
}
