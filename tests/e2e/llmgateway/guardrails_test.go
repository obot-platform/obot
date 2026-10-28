//go:build e2e

package llmgateway

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/tests/e2e/harness"
	"github.com/tidwall/gjson"
)

// guardrailPolicy is the policy API's shape.
type guardrailPolicy struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	Target       string   `json:"target,omitempty"`
	ApplyTo      string   `json:"applyTo"`
	Action       string   `json:"action"`
	Condition    string   `json:"condition,omitempty"`
	FailClosed   bool     `json:"failClosed,omitempty"`
	BlockMessage string   `json:"blockMessage,omitempty"`
	InputScope   string   `json:"inputScope,omitempty"`
	GuardrailIDs []string `json:"guardrailIDs"`
	ProfileID    string   `json:"profileID"`
}

// guardrailFinding is what the LLM audit log records about one guardrail policy's outcome.
type guardrailFinding struct {
	Stage         string   `json:"stage"`
	PolicyID      string   `json:"policyID"`
	PolicyName    string   `json:"policyName"`
	GuardrailType string   `json:"guardrailType"`
	Outcome       string   `json:"outcome"`
	Categories    []string `json:"categories"`
	Error         string   `json:"error"`
}

// githubToken returns a random, well-formed GitHub token for the secrets guardrail.
func githubToken() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var sb strings.Builder
	sb.WriteString("ghp_")
	for range 36 {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		sb.WriteByte(alphabet[n.Int64()])
	}
	return sb.String()
}

// createGuardrail creates a guardrail and deletes it when the test ends.
func createGuardrail(t *testing.T, typ string, config map[string]any) string {
	t.Helper()
	var p struct {
		ID string `json:"id"`
	}
	env.admin.Post(t, "/api/llm-gateway/guardrails", map[string]any{
		"name":   "e2e " + typ + " " + harness.RandomSuffix(),
		"type":   typ,
		"config": config,
	}, &p)
	if p.ID == "" {
		t.Fatal("guardrail created without an ID")
	}
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/llm-gateway/guardrails/"+p.ID, nil)
	})
	return p.ID
}

// createPolicy creates an LLM guardrail policy and deletes it when the test ends. Cleanups run
// in reverse order, so the policy goes before the guardrails it uses.
func createPolicy(t *testing.T, r guardrailPolicy) guardrailPolicy {
	t.Helper()
	r.Name = r.Name + " " + harness.RandomSuffix()
	r.Enabled = true
	var created guardrailPolicy
	env.admin.Post(t, "/api/llm-gateway/guardrail-policies", r, &created)
	if created.ID == "" {
		t.Fatal("policy created without an ID")
	}
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/llm-gateway/guardrail-policies/"+created.ID, nil)
	})
	return created
}

// waitForFindings polls a user's LLM audit log until it records at least atLeast findings of a
// policy, with the given outcome unless outcome is empty. It returns them and their JSON.
func waitForFindings(t *testing.T, userID, policyID, outcome string, atLeast int) ([]guardrailFinding, string) {
	t.Helper()
	var (
		findings []guardrailFinding
		raw      string
	)
	harness.Eventually(t, 60*time.Second, fmt.Sprintf("%d %s findings of %s for %s", atLeast, outcome, policyID, userID), func() error {
		resp := env.admin.Do(t, http.MethodGet, "/api/llm-audit-logs?hide_models_requests=true&user_id="+url.QueryEscape(userID)+"&limit=100", nil)
		if resp.Status != http.StatusOK {
			return fmt.Errorf("audit logs: %s", resp)
		}
		var list struct {
			Items []struct {
				GuardrailFindings []guardrailFinding `json:"guardrailFindings"`
			} `json:"items"`
		}
		resp.JSON(t, &list)
		findings = nil
		for _, item := range list.Items {
			for _, f := range item.GuardrailFindings {
				if f.PolicyID == policyID && (outcome == "" || f.Outcome == outcome) {
					findings = append(findings, f)
				}
			}
		}
		raw = mustJSON(findings)
		if len(findings) < atLeast {
			return fmt.Errorf("got %d findings", len(findings))
		}
		return nil
	})
	return findings, raw
}

// isGuardrailError reports whether a response body is a guardrail's error: type
// guardrail_intervention in the OpenAI formats, or Anthropic's error envelope, whose error
// types are limited to Anthropic's documented ones, with invalid_request_error.
func isGuardrailError(body string) bool {
	if strings.Contains(body, "guardrail_intervention") {
		return true
	}
	for _, event := range strings.Split(body, "\n") {
		event = strings.TrimPrefix(strings.TrimSpace(event), "data: ")
		if gjson.Get(event, "type").String() == "error" && gjson.Get(event, "error.type").String() == "invalid_request_error" {
			return true
		}
	}
	return false
}

// requireGuardrailBlock checks a blocked call: HTTP 400 with a guardrail error, or a stream
// that ends with one and delivered nothing, and the secret appears nowhere.
func requireGuardrailBlock(t *testing.T, res *llmResult, secret string) {
	t.Helper()
	if strings.Contains(res.Body, secret) {
		t.Fatalf("a blocked response contains the secret: %s", truncate(res.Body))
	}
	if res.Status == http.StatusBadRequest {
		if !isGuardrailError(res.Body) {
			t.Fatalf("want a guardrail error, got: %s", truncate(res.Body))
		}
		return
	}
	if res.Status == http.StatusOK && isGuardrailError(res.Body) && res.Text == "" && res.ToolArgs == "" {
		return
	}
	t.Fatalf("want HTTP 400 with a guardrail error, got %d (text %q): %s", res.Status, res.Text, truncate(res.Body))
}

func TestGuardrails(t *testing.T) {
	secrets := createGuardrail(t, "secrets", map[string]any{"generic": false})
	pii := createGuardrail(t, "regex", map[string]any{"piiTemplate": []string{"EMAIL"}})

	t.Run("input redaction", func(t *testing.T) {
		u := env.newUser(t, "gr-input")
		policy := createPolicy(t, guardrailPolicy{Name: "e2e input redact", ApplyTo: "input", Action: "redact",
			GuardrailIDs: []string{secrets, pii}, ProfileID: profileOf(t, u)})
		for _, api := range allAPIs {
			for _, model := range []string{"openai/gpt-e2e-a", "anthropic/claude-e2e-a"} {
				secret, email := githubToken(), "jane."+harness.RandomSuffix()+"@example.org"
				n := nonce("in")
				callLLM(t, u.Gateway(env.A), api, model, fmt.Sprintf("%s my token is %s and mail %s", n, secret, email), false, nil).requireOK(t)
				reqs := env.mock.InferenceRequests(n)
				if len(reqs) != 1 {
					t.Fatalf("%s %s: want 1 upstream request, got %d", api, model, len(reqs))
				}
				body := reqs[0].Body
				if strings.Contains(body, secret) || strings.Contains(body, email) {
					t.Fatalf("%s %s: the provider received unredacted input: %s", api, model, body)
				}
				if !strings.Contains(body, "[REDACTED:GITHUB_TOKEN]") || !strings.Contains(body, "[REDACTED:EMAIL]") {
					t.Fatalf("%s %s: want redaction labels upstream: %s", api, model, body)
				}
			}
		}
		findings, raw := waitForFindings(t, u.ID, policy.ID, "redacted", 6)
		for _, f := range findings {
			if f.Stage != "input" {
				t.Fatalf("unexpected finding %+v", f)
			}
		}
		if strings.Contains(raw, "ghp_") || strings.Contains(raw, "@example.org") {
			t.Fatalf("guardrail findings contain the offending content: %s", raw)
		}
	})

	t.Run("output redaction", func(t *testing.T) {
		u := env.newUser(t, "gr-output")
		createPolicy(t, guardrailPolicy{Name: "e2e output redact", ApplyTo: "output", Action: "redact",
			GuardrailIDs: []string{secrets}, ProfileID: profileOf(t, u)})
		for _, api := range allAPIs {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t", api, stream), func(t *testing.T) {
					secret := githubToken()
					n := nonce("out")
					reply := "here it is: " + secret + " done " + n
					// Five-byte chunks split the secret across many stream events.
					res := callLLM(t, u.Gateway(env.A), api, "openai/gpt-e2e-a", n+" "+harness.Reply(reply)+" @@chunk:5@@", stream, nil).requireOK(t)
					if strings.Contains(res.Body, secret) || strings.Contains(res.Text, secret) {
						t.Fatalf("the client received the secret: %s", truncate(res.Body))
					}
					want := "here it is: [REDACTED:GITHUB_TOKEN] done " + n
					if res.Text != want {
						t.Fatalf("want redacted text %q, got %q", want, res.Text)
					}
				})
			}
		}
		t.Run("tool call arguments", func(t *testing.T) {
			for _, api := range allAPIs {
				for _, stream := range []bool{false, true} {
					secret := githubToken()
					n := nonce("tool")
					res := callLLM(t, u.Gateway(env.A), api, "anthropic/claude-e2e-a",
						n+" "+harness.ToolCall("deploy", map[string]any{"token": secret, "note": n})+" @@chunk:7@@", stream, map[string]any{
							"tools": toolsFor(api),
						}).requireOK(t)
					if strings.Contains(res.Body, secret) {
						t.Fatalf("%s stream=%t: the client received the secret in a tool call: %s", api, stream, truncate(res.Body))
					}
					if !strings.Contains(res.ToolArgs, "REDACTED") {
						t.Fatalf("%s stream=%t: want redacted tool call arguments, got %q: %s", api, stream, res.ToolArgs, truncate(res.Body))
					}
				}
			}
		})
	})

	t.Run("block", func(t *testing.T) {
		u := env.newUser(t, "gr-block")
		message := "Blocked by the e2e policy " + harness.RandomSuffix()
		createPolicy(t, guardrailPolicy{Name: "e2e block", ApplyTo: "both", Action: "block", BlockMessage: message,
			GuardrailIDs: []string{secrets}, ProfileID: profileOf(t, u)})
		for _, api := range allAPIs {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("input/%s/stream=%t", api, stream), func(t *testing.T) {
					secret := githubToken()
					n := nonce("blockin")
					res := callLLM(t, u.Gateway(env.A), api, "openai/gpt-e2e-a", n+" "+secret, stream, nil)
					requireGuardrailBlock(t, res, secret)
					if res.Status != http.StatusBadRequest {
						t.Fatalf("a blocked request must get HTTP 400, got %d", res.Status)
					}
					if !strings.Contains(res.Body, message) {
						t.Fatalf("want the custom block message: %s", res.Body)
					}
					if len(env.mock.InferenceRequests(n)) != 0 {
						t.Fatal("a blocked request reached the provider")
					}
				})
				t.Run(fmt.Sprintf("output/%s/stream=%t", api, stream), func(t *testing.T) {
					secret := githubToken()
					n := nonce("blockout")
					res := callLLM(t, u.Gateway(env.A), api, "anthropic/claude-e2e-a", n+" "+harness.Reply("leaking "+secret+" now")+" @@chunk:5@@", stream, nil)
					requireGuardrailBlock(t, res, secret)
					if !strings.Contains(res.Body, message) {
						t.Fatalf("want the custom block message: %s", truncate(res.Body))
					}
				})
			}
		}
		// The provider produced and billed every response the output policy blocked, so each
		// one's usage (10 input and 5 output tokens from the mock) is recorded, and counts
		// against budgets, although the caller only got an error.
		t.Run("blocked output is still recorded", func(t *testing.T) {
			harness.Eventually(t, 60*time.Second, "token usage of the blocked responses", func() error {
				resp := env.admin.Do(t, http.MethodGet, "/api/users/"+u.ID+"/token-usage", nil)
				if resp.Status != http.StatusOK {
					return fmt.Errorf("usage: %s", resp)
				}
				var in, out int64
				for _, item := range gjson.GetBytes(resp.Body, "items").Array() {
					if item.Get("model").String() == "anthropic/claude-e2e-a" {
						in += item.Get("inputTokens").Int()
						out += item.Get("outputTokens").Int()
					}
				}
				if want := int64(len(allAPIs) * 2); in != 10*want || out != 5*want {
					return fmt.Errorf("blocked responses used %d in / %d out, want %d / %d: %s", in, out, 10*want, 5*want, resp.Body)
				}
				return nil
			})
		})
	})

	t.Run("log only", func(t *testing.T) {
		u := env.newUser(t, "gr-log")
		policy := createPolicy(t, guardrailPolicy{Name: "e2e log", ApplyTo: "both", Action: "log",
			GuardrailIDs: []string{secrets}, ProfileID: profileOf(t, u)})
		secret := githubToken()
		n := nonce("log")
		res := callLLM(t, u.Gateway(env.A), apiChat, "openai/gpt-e2e-a", n+" "+secret+" "+harness.Reply("echo "+secret), true, nil).requireOK(t)
		if res.Text != "echo "+secret {
			t.Fatalf("a log-only policy changed the response: %q", res.Text)
		}
		if reqs := env.mock.InferenceRequests(n); len(reqs) != 1 || !strings.Contains(reqs[0].Body, secret) {
			t.Fatal("a log-only policy changed the request")
		}
		findings, raw := waitForFindings(t, u.ID, policy.ID, "", 2)
		for _, f := range findings {
			if f.Outcome != "logged" {
				t.Fatalf("want logged findings, got %+v", f)
			}
		}
		if strings.Contains(raw, secret) {
			t.Fatalf("findings contain the secret: %s", raw)
		}
	})

	t.Run("subjects", func(t *testing.T) {
		member, target, bystander := env.newUser(t, "gr-member"), env.newUser(t, "gr-target"), env.newUser(t, "gr-bystander")
		group := "e2e-group-" + harness.RandomSuffix()
		// Local auth has no groups; add one as an auth provider would.
		if _, err := env.cluster.PG.Exec(t.Context(), env.cluster.DB, fmt.Sprintf(
			"insert into groups (id, auth_provider_name, auth_provider_namespace, name) values ('%s', 'local-auth-provider', 'default', '%s');"+
				"insert into group_memberships (user_id, group_id, created_at) values (%s, '%s', now())",
			group, group, member.ID, group)); err != nil {
			t.Fatalf("add group: %v", err)
		}
		createPolicy(t, guardrailPolicy{Name: "e2e group policy", ApplyTo: "input", Action: "redact",
			GuardrailIDs: []string{secrets}, ProfileID: profileOfSubjects(t, []types.Subject{{Type: types.SubjectTypeGroup, ID: group}}, member)})
		createPolicy(t, guardrailPolicy{Name: "e2e user policy", ApplyTo: "input", Action: "redact",
			GuardrailIDs: []string{secrets}, ProfileID: profileOf(t, target)})
		for _, tc := range []struct {
			who      *testUser
			redacted bool
		}{
			{who: member, redacted: true},
			{who: target, redacted: true},
			{who: bystander, redacted: false},
		} {
			secret := githubToken()
			n := nonce("subj")
			callLLM(t, tc.who.Gateway(env.A), apiChat, "openai/gpt-e2e-a", n+" "+secret, false, nil).requireOK(t)
			reqs := env.mock.InferenceRequests(n)
			if len(reqs) != 1 {
				t.Fatalf("%s: want 1 upstream request", tc.who.Email)
			}
			if got := !strings.Contains(reqs[0].Body, secret); got != tc.redacted {
				t.Fatalf("%s: redacted=%t, want %t", tc.who.Email, got, tc.redacted)
			}
		}
	})

	t.Run("condition", func(t *testing.T) {
		u := env.newUser(t, "gr-cond")
		profile := profileOf(t, u)
		createPolicy(t, guardrailPolicy{Name: "e2e condition", ApplyTo: "input", Action: "redact",
			Condition:    `model == "openai/gpt-e2e-b"`,
			GuardrailIDs: []string{secrets}, ProfileID: profile})
		for model, redacted := range map[string]bool{"openai/gpt-e2e-b": true, "gpt-e2e-b": true, "openai/gpt-e2e-a": false} {
			secret := githubToken()
			n := nonce("cond")
			callLLM(t, u.Gateway(env.A), apiChat, model, n+" "+secret, false, nil).requireOK(t)
			reqs := env.mock.InferenceRequests(n)
			if len(reqs) != 1 {
				t.Fatalf("%s: want 1 upstream request", model)
			}
			if got := !strings.Contains(reqs[0].Body, secret); got != redacted {
				t.Fatalf("%s: redacted=%t, want %t", model, got, redacted)
			}
		}

		// A condition that fails at runtime (a missing header) skips the policy, and blocks when
		// the policy fails closed.
		createPolicy(t, guardrailPolicy{Name: "e2e condition error", ApplyTo: "input", Action: "redact",
			Condition: `headers["x-e2e-missing"] == "1"`, GuardrailIDs: []string{secrets}, ProfileID: profile})
		secret := githubToken()
		n := nonce("conderr")
		callLLM(t, u.Gateway(env.A), apiChat, "openai/gpt-e2e-a", n+" "+secret, false, nil).requireOK(t)
		if reqs := env.mock.InferenceRequests(n); len(reqs) != 1 || !strings.Contains(reqs[0].Body, secret) {
			t.Fatal("a policy whose condition failed was applied")
		}

		u2 := env.newUser(t, "gr-cond-closed")
		createPolicy(t, guardrailPolicy{Name: "e2e condition error closed", ApplyTo: "input", Action: "redact", FailClosed: true,
			Condition: `headers["x-e2e-missing"] == "1"`, GuardrailIDs: []string{secrets}, ProfileID: profileOf(t, u2)})
		requireGuardrailBlock(t, callLLM(t, u2.Gateway(env.A), apiChat, "openai/gpt-e2e-a", "hi "+nonce("c"), false, nil), "ghp_")
	})

	t.Run("fail closed with an erroring guardrail", func(t *testing.T) {
		// Guardrails can't use loopback addresses, so this one is a name that never resolves.
		dead := createGuardrail(t, "presidio", map[string]any{"analyzerURL": "http://presidio.invalid:1"})
		closedUser, openUser := env.newUser(t, "gr-closed"), env.newUser(t, "gr-open")
		createPolicy(t, guardrailPolicy{Name: "e2e fail closed", ApplyTo: "input", Action: "redact", FailClosed: true,
			GuardrailIDs: []string{dead}, ProfileID: profileOf(t, closedUser)})
		openPolicy := createPolicy(t, guardrailPolicy{Name: "e2e fail open", ApplyTo: "input", Action: "redact",
			GuardrailIDs: []string{dead}, ProfileID: profileOf(t, openUser)})

		n := nonce("closed")
		res := callLLM(t, closedUser.Gateway(env.A), apiChat, "openai/gpt-e2e-a", "hello "+n, false, nil)
		requireGuardrailBlock(t, res, "never")
		if res.Status != http.StatusBadRequest || len(env.mock.InferenceRequests(n)) != 0 {
			t.Fatalf("a fail-closed policy with a broken guardrail let the request through: %d", res.Status)
		}
		n = nonce("open")
		callLLM(t, openUser.Gateway(env.A), apiChat, "openai/gpt-e2e-a", "hello "+n, false, nil).requireOK(t)
		if len(env.mock.InferenceRequests(n)) != 1 {
			t.Fatal("a fail-open policy with a broken guardrail blocked the request")
		}
		findings, _ := waitForFindings(t, openUser.ID, openPolicy.ID, "error", 1)
		if findings[0].Error == "" {
			t.Fatalf("an error finding has no error: %+v", findings[0])
		}
	})

	t.Run("LLM audit log records what guardrails did", func(t *testing.T) {
		u := env.newUser(t, "gr-auditlog")
		createPolicy(t, guardrailPolicy{Name: "e2e auditlog", ApplyTo: "both", Action: "redact",
			GuardrailIDs: []string{secrets}, ProfileID: profileOf(t, u)})
		inSecret, outSecret := githubToken(), githubToken()
		for _, stream := range []bool{false, true} {
			n := nonce("auditlog")
			callLLM(t, u.Gateway(env.A), apiChat, "openai/gpt-e2e-a",
				n+" "+inSecret+" "+harness.Reply("out "+outSecret)+" @@chunk:5@@", stream, nil).requireOK(t)
		}
		var items []gjson.Result
		harness.Eventually(t, 60*time.Second, "LLM audit log entries for "+u.ID, func() error {
			resp := env.admin.Do(t, http.MethodGet, "/api/llm-audit-logs?hide_models_requests=true&user_id="+url.QueryEscape(u.ID)+"&limit=10", nil)
			if resp.Status != http.StatusOK {
				return fmt.Errorf("audit logs: %s", resp)
			}
			items = gjson.GetBytes(resp.Body, "items").Array()
			if len(items) < 2 {
				return fmt.Errorf("%d entries", len(items))
			}
			return nil
		})
		auditor := newAuditor(t)
		for _, item := range items {
			if !strings.Contains(item.Get("guardrailFindings").Raw, "redacted") {
				t.Fatalf("want the entry to record the redaction: %s", truncate(item.Raw))
			}
			// The entry records the request as the client sent it, and the response as the
			// client got it: redacted.
			detail := auditor.Session.Do(t, http.MethodGet, "/api/llm-audit-logs/detail/"+url.PathEscape(item.Get("id").String()), nil)
			if detail.Status != http.StatusOK {
				t.Fatalf("audit detail: %s", detail)
			}
			if response := gjson.GetBytes(detail.Body, "responseBody").Raw; response == "" || strings.Contains(response, outSecret) {
				t.Fatalf("the audit log's response contains the redacted secret or is missing: %s", truncate(string(detail.Body)))
			}
		}
	})

	t.Run("Claude Code passthrough is checked", func(t *testing.T) {
		u := env.newUser(t, "gr-claude")
		createPolicy(t, guardrailPolicy{Name: "e2e claude code", ApplyTo: "both", Action: "redact",
			GuardrailIDs: []string{secrets}, ProfileID: profileOf(t, u)})
		inSecret, outSecret := githubToken(), githubToken()
		ua := map[string]string{"User-Agent": "claude-cli/2.0.0 (external, cli)", "anthropic-beta": "claude-code-20250219"}
		for _, stream := range []bool{false, true} {
			n := nonce("claude")
			res := callLLM(t, u.Gateway(env.A), apiMessages, "claude-e2e-a",
				n+" "+inSecret+" "+harness.Reply("out "+outSecret)+" @@chunk:5@@", stream, nil, ua).requireOK(t)
			reqs := env.mock.InferenceRequests(n)
			if len(reqs) != 1 {
				t.Fatalf("stream=%t: want 1 upstream request, got %d", stream, len(reqs))
			}
			if strings.Contains(reqs[0].Body, inSecret) || !strings.Contains(reqs[0].Body, "[REDACTED:GITHUB_TOKEN]") {
				t.Fatalf("stream=%t: the provider received unredacted Claude Code input: %s", stream, reqs[0].Body)
			}
			if strings.Contains(res.Body, outSecret) || res.Text != "out [REDACTED:GITHUB_TOKEN]" {
				t.Fatalf("stream=%t: Claude Code received unredacted output %q: %s", stream, res.Text, truncate(res.Body))
			}
		}
	})
}

// toolsFor returns a tool definition in the API format, so tool calls are expected.
func toolsFor(api string) []any {
	schema := map[string]any{"type": "object", "properties": map[string]any{"token": map[string]any{"type": "string"}, "note": map[string]any{"type": "string"}}}
	switch api {
	case apiChat:
		return []any{map[string]any{"type": "function", "function": map[string]any{"name": "deploy", "parameters": schema}}}
	case apiResponses:
		return []any{map[string]any{"type": "function", "name": "deploy", "parameters": schema}}
	default:
		return []any{map[string]any{"name": "deploy", "input_schema": schema}}
	}
}
