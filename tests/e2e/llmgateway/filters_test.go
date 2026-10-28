//go:build e2e

package llmgateway

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/tests/e2e/harness"
	"github.com/tidwall/gjson"
)

// propagation bounds how long a new filter, access rule or server takes to reach the MCP
// gateway's caches. They are fed by storage watches, which normally deliver changes at once
// but fall back to polling every two minutes when a notification is missed.
const (
	propagation = 3 * time.Minute
)

// bearer adds an API key to every request.
type bearer struct {
	key  string
	next http.RoundTripper
}

// mcpTarget is a remote MCP server registered in Obot for one test.
type mcpTarget struct {
	entryID  string
	serverID string
}

type filterSpec struct {
	method       string
	action       string
	direction    string
	failClosed   bool
	condition    string
	blockMessage string
	subjects     []types.MCPGuardrailFilterSubject
	guardrails   []string
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.key)
	return b.next.RoundTrip(r)
}

// newMCPTarget registers the mock MCP server as a remote catalog entry, and publishes it to
// everyone as a vMCP, which is what MCP clients connect to.
func newMCPTarget(t *testing.T, owner *testUser) mcpTarget {
	t.Helper()
	var entry types.MCPServerCatalogEntry
	env.admin.Post(t, "/api/mcp-catalogs/"+system.DefaultCatalog+"/entries", types.MCPServerCatalogEntryManifest{
		Name:             "e2e mock " + harness.RandomSuffix(),
		ShortDescription: "e2e guardrail filter target",
		Runtime:          types.RuntimeRemote,
		RemoteConfig:     &types.RemoteCatalogConfig{FixedURL: env.mcp.URL},
	}, &entry)
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/mcp-catalogs/"+system.DefaultCatalog+"/entries/"+entry.ID, nil)
	})
	return mcpTarget{entryID: entry.ID, serverID: newMCPServer(t, owner, entry.ID)}
}

// mcpDiag describes the state of a vMCP for a failure message.
func mcpDiag(t *testing.T, u *testUser, vmcpID string) string {
	t.Helper()
	asUser := u.Session.Do(t, http.MethodGet, "/api/vmcps/"+vmcpID, nil)
	asAdmin := env.admin.Do(t, http.MethodGet, "/api/vmcps/"+vmcpID, nil)
	me := u.Session.Do(t, http.MethodGet, "/api/me", nil)
	return fmt.Sprintf("vMCP as user: %s; as admin: %s; me: %s", asUser, truncate(asAdmin.String()), me)
}

// newMCPServer publishes a catalog entry to everyone as a vMCP and returns the vMCP's ID,
// which is where MCP clients connect. Each user who connects gets their own server.
func newMCPServer(t *testing.T, owner *testUser, entryID string) string {
	t.Helper()
	var vmcp types.VMCP
	env.admin.Post(t, "/api/vmcps", types.VMCPManifest{
		DisplayName: "e2e mock " + harness.RandomSuffix(),
		Components: []types.VMCPComponent{
			{
				Name:                    "mock",
				MCPCatalogID:            system.DefaultCatalog,
				MCPServerCatalogEntryID: entryID,
			},
		},
		Profiles: []types.VMCPProfile{
			{
				Name:        "everyone",
				Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			},
		},
	}, &vmcp)
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/vmcps/"+vmcp.ID, nil)
	})
	owner.Session.Do(t, http.MethodPost, "/api/vmcps/"+vmcp.ID+"/launch", map[string]any{})
	return vmcp.ID
}

// connectMCP opens an MCP session through Obot's MCP gateway as the user.
func connectMCP(t *testing.T, u *testUser, r *harness.Replica, serverID string) *gomcp.ClientSession {
	t.Helper()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "obot-e2e", Version: "1.0.0"}, nil)
	var session *gomcp.ClientSession
	harness.Eventually(t, propagation, "connect to MCP server "+serverID, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		session, err = client.Connect(ctx, &gomcp.StreamableClientTransport{
			Endpoint:             r.BaseURL + "/mcp-connect/" + serverID,
			HTTPClient:           &http.Client{Transport: bearer{key: u.Key, next: http.DefaultTransport}, Timeout: 30 * time.Second},
			DisableStandaloneSSE: true,
		}, nil)
		if err != nil {
			// Say why the gateway refused.
			raw := harness.NewAPIKeyClient(r.BaseURL, u.Key).Do(t, http.MethodPost, "/mcp-connect/"+serverID, map[string]any{
				"jsonrpc": "2.0", "id": 1, "method": "initialize",
				"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "diag", "version": "1"}},
			}, map[string]string{"Accept": "application/json, text/event-stream"})
			return fmt.Errorf("%w (raw initialize: %s; %s)", err, raw, mcpDiag(t, u, serverID))
		}
		return nil
	})
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// createGuardrailFilter creates a guardrail filter on the target's vMCP.
func createGuardrailFilter(t *testing.T, target mcpTarget, f filterSpec) string {
	t.Helper()
	var selectors types.MCPSelectors
	if f.method != "" {
		selectors = types.MCPSelectors{{Method: f.method}}
	}
	var created types.MCPWebhookValidation
	env.admin.Post(t, "/api/mcp-webhook-validations", types.MCPWebhookValidationManifest{
		Name:      "e2e guardrail filter " + harness.RandomSuffix(),
		Resources: []types.Resource{{Type: types.ResourceTypeMCPServer, ID: target.serverID}},
		Selectors: selectors,
		Guardrail: &types.MCPGuardrailFilterConfig{
			GuardrailIDs: f.guardrails,
			Action:       f.action,
			Direction:    f.direction,
			FailClosed:   f.failClosed,
			Condition:    f.condition,
			BlockMessage: f.blockMessage,
			Subjects:     f.subjects,
		},
	}, &created)
	if created.ID == "" {
		t.Fatal("filter created without an ID")
	}
	t.Cleanup(func() {
		env.admin.Do(t, http.MethodDelete, "/api/mcp-webhook-validations/"+created.ID, nil)
	})
	return created.ID
}

func toolText(res *gomcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*gomcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestGuardrailFilters(t *testing.T) {
	secrets := createGuardrail(t, "secrets", map[string]any{"generic": false})
	ctx := context.Background()

	t.Run("tools/call arguments redacted", func(t *testing.T) {
		u := env.newUser(t, "mcp-redact-in")
		target := newMCPTarget(t, u)
		filter := createGuardrailFilter(t, target, filterSpec{method: "tools/call", action: "redact", direction: "request", guardrails: []string{secrets}})
		session := connectMCP(t, u, env.A, target.serverID)
		harness.Eventually(t, propagation, "redacted tool call arguments", func() error {
			secret, n := githubToken(), nonce("mcpin")
			res, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": n + " " + secret}})
			if err != nil {
				return err
			}
			if got := env.mcp.Received(n); len(got) != 1 || strings.Contains(got[0], secret) || !strings.Contains(got[0], "[REDACTED:GITHUB_TOKEN]") {
				return fmt.Errorf("the MCP server received %v", got)
			}
			if strings.Contains(toolText(res), secret) {
				return fmt.Errorf("result has the secret: %s", toolText(res))
			}
			return nil
		})
		// The MCP audit log records the redaction as the filter's status. The list has only
		// summaries; the statuses are in each entry's detail.
		harness.Eventually(t, 60*time.Second, "the guardrail filter's status in the MCP audit log", func() error {
			resp := env.admin.Do(t, http.MethodGet, "/api/mcp-audit-logs?user_id="+url.QueryEscape(u.ID)+"&limit=100", nil)
			if resp.Status != http.StatusOK {
				return fmt.Errorf("audit logs: %s", resp)
			}
			for _, item := range gjson.GetBytes(resp.Body, "items").Array() {
				detail := env.admin.Do(t, http.MethodGet, "/api/mcp-audit-logs/detail/"+item.Get("id").String(), nil)
				if detail.Status != http.StatusOK {
					return fmt.Errorf("audit log %s: %s", item.Get("id"), detail)
				}
				for _, status := range gjson.GetBytes(detail.Body, "details.webhookStatuses").Array() {
					if status.Get("tool").String() == "guardrail" && status.Get("status").String() == "mutated" {
						if strings.Contains(status.Raw, "ghp_") {
							return fmt.Errorf("the status contains the secret: %s", status.Raw)
						}
						return nil
					}
				}
			}
			return fmt.Errorf("no mutated guardrail filter status for %s (filter %s)", u.ID, filter)
		})
	})

	t.Run("tools/call blocked both ways", func(t *testing.T) {
		u := env.newUser(t, "mcp-block")
		target := newMCPTarget(t, u)
		msg := "MCP blocked by e2e " + harness.RandomSuffix()
		createGuardrailFilter(t, target, filterSpec{method: "tools/call", action: "block", direction: "both", blockMessage: msg, guardrails: []string{secrets}})
		session := connectMCP(t, u, env.A, target.serverID)
		harness.Eventually(t, propagation, "blocked tool call request", func() error {
			secret, n := githubToken(), nonce("mcpblock")
			res, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": n + " " + secret}})
			if err == nil && !res.IsError {
				return fmt.Errorf("not blocked: %s", toolText(res))
			}
			if len(env.mcp.Received(n)) != 0 || len(env.mcp.RawRequests(secret)) != 0 {
				return fmt.Errorf("a blocked request reached the MCP server")
			}
			text := fmt.Sprint(err)
			if res != nil {
				text += toolText(res)
			}
			if !strings.Contains(text, msg) {
				return fmt.Errorf("want the block message, got %s", text)
			}
			return nil
		})
		// A clean call still works, and a result with a secret is blocked.
		if res, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "clean"}}); err != nil || res.IsError {
			t.Fatalf("a clean tool call failed: %v %+v", err, res)
		}
		secret := githubToken()
		res, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "leak", Arguments: map[string]any{"payload": b64(secret)}})
		if err == nil && !res.IsError {
			t.Fatalf("a result with a secret was not blocked: %s", toolText(res))
		}
		if err == nil && strings.Contains(toolText(res), secret) || strings.Contains(fmt.Sprint(err), secret) {
			t.Fatal("a blocked result leaked the secret")
		}
	})

	t.Run("tools/call results, resources and prompts redacted", func(t *testing.T) {
		u := env.newUser(t, "mcp-redact-out")
		target := newMCPTarget(t, u)
		createGuardrailFilter(t, target, filterSpec{action: "redact", direction: "both", guardrails: []string{secrets}})
		session := connectMCP(t, u, env.A, target.serverID)

		harness.Eventually(t, propagation, "redacted tool result", func() error {
			secret := githubToken()
			res, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "leak", Arguments: map[string]any{"payload": b64(secret)}})
			if err != nil {
				return err
			}
			if text := toolText(res); strings.Contains(text, secret) || !strings.Contains(text, "[REDACTED:GITHUB_TOKEN]") {
				return fmt.Errorf("tool result %q", text)
			}
			return nil
		})

		secret := githubToken()
		read, err := session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "e2e://doc/" + base64.RawURLEncoding.EncodeToString([]byte("key "+secret))})
		if err != nil {
			t.Fatalf("read resource: %v", err)
		}
		if len(read.Contents) != 1 || strings.Contains(read.Contents[0].Text, secret) || !strings.Contains(read.Contents[0].Text, "[REDACTED:GITHUB_TOKEN]") {
			t.Fatalf("resource contents not redacted: %+v", read.Contents)
		}

		argSecret, outSecret, n := githubToken(), githubToken(), nonce("prompt")
		prompt, err := session.GetPrompt(ctx, &gomcp.GetPromptParams{Name: "brief", Arguments: map[string]string{
			"topic": n + " " + argSecret, "payload": b64(outSecret),
		}})
		if err != nil {
			t.Fatalf("get prompt: %v; %s", err, mcpDiag(t, u, target.serverID))
		}
		if got := env.mcp.Received(n); len(got) != 1 || strings.Contains(got[0], argSecret) {
			t.Fatalf("the MCP server received unredacted prompt arguments: %v", got)
		}
		text := prompt.Messages[0].Content.(*gomcp.TextContent).Text
		if strings.Contains(text, outSecret) || strings.Contains(text, argSecret) || strings.Count(text, "[REDACTED:GITHUB_TOKEN]") != 2 {
			t.Fatalf("prompt not redacted: %q", text)
		}
	})

	t.Run("resources and prompts blocked", func(t *testing.T) {
		u := env.newUser(t, "mcp-block-rp")
		target := newMCPTarget(t, u)
		createGuardrailFilter(t, target, filterSpec{action: "block", direction: "response", guardrails: []string{secrets}})
		session := connectMCP(t, u, env.A, target.serverID)
		harness.Eventually(t, propagation, "blocked resource read", func() error {
			secret := githubToken()
			read, err := session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "e2e://doc/" + base64.RawURLEncoding.EncodeToString([]byte(secret))})
			if err == nil {
				return fmt.Errorf("not blocked: %+v", read.Contents)
			}
			if strings.Contains(err.Error(), secret) {
				return fmt.Errorf("the error leaks the secret")
			}
			return nil
		})
		secret := githubToken()
		if p, err := session.GetPrompt(ctx, &gomcp.GetPromptParams{Name: "brief", Arguments: map[string]string{"topic": "x", "payload": b64(secret)}}); err == nil {
			t.Fatalf("a prompt with a secret was not blocked: %+v", p.Messages)
		}
		// Clean content passes.
		if _, err := session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "e2e://doc/" + base64.RawURLEncoding.EncodeToString([]byte("clean"))}); err != nil {
			t.Fatalf("a clean resource read failed: %v", err)
		}
	})

	t.Run("initialize and tools/list unaffected by a failing fail-closed filter", func(t *testing.T) {
		u := env.newUser(t, "mcp-failclosed")
		target := newMCPTarget(t, u)
		createGuardrailFilter(t, target, filterSpec{action: "redact", direction: "both", failClosed: true,
			condition: `mcp_arguments["no_such_argument"] == "x"`, guardrails: []string{secrets}})
		// Wait for the filter to apply: tool calls are blocked by the failing condition.
		probe := connectMCP(t, u, env.A, target.serverID)
		harness.Eventually(t, propagation, "the fail-closed filter to block tool calls", func() error {
			res, err := probe.CallTool(ctx, &gomcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hello"}})
			if err == nil && !res.IsError {
				return fmt.Errorf("tool call allowed: %s", toolText(res))
			}
			return nil
		})
		// A new session initializes and lists tools, resources templates and prompts.
		session := connectMCP(t, u, env.A, target.serverID)
		tools, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("tools/list failed under a fail-closed filter: %v", err)
		}
		if len(tools.Tools) != 2 {
			t.Fatalf("want 2 tools, got %d", len(tools.Tools))
		}
		if _, err := session.ListPrompts(ctx, nil); err != nil {
			t.Fatalf("prompts/list failed under a fail-closed filter: %v", err)
		}
	})

	t.Run("filter subjects", func(t *testing.T) {
		subject, other := env.newUser(t, "mcp-subject"), env.newUser(t, "mcp-other")
		target := newMCPTarget(t, subject)
		createGuardrailFilter(t, target, filterSpec{method: "tools/call", action: "redact", direction: "request", guardrails: []string{secrets},
			subjects: []types.MCPGuardrailFilterSubject{{Type: "user", ID: subject.ID}}})
		s1 := connectMCP(t, subject, env.A, target.serverID)
		harness.Eventually(t, propagation, "the filter applies to its subject", func() error {
			secret, n := githubToken(), nonce("subj")
			if _, err := s1.CallTool(ctx, &gomcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": n + " " + secret}}); err != nil {
				return err
			}
			if got := env.mcp.Received(n); len(got) != 1 || strings.Contains(got[0], secret) {
				return fmt.Errorf("received %v", got)
			}
			return nil
		})
		s2 := connectMCP(t, other, env.A, target.serverID)
		secret, n := githubToken(), nonce("nosubj")
		if _, err := s2.CallTool(ctx, &gomcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": n + " " + secret}}); err != nil {
			t.Fatal(err)
		}
		if got := env.mcp.Received(n); len(got) != 1 || !strings.Contains(got[0], secret) {
			t.Fatalf("the filter applied to a user it isn't assigned to: %v", got)
		}
	})

	t.Run("JSON-RPC batches and unparseable messages are refused", func(t *testing.T) {
		u := env.newUser(t, "mcp-batch")
		target := newMCPTarget(t, u)
		createGuardrailFilter(t, target, filterSpec{method: "tools/call", action: "redact", direction: "both", guardrails: []string{secrets}})
		// Wait until the filter applies.
		probe := connectMCP(t, u, env.A, target.serverID)
		harness.Eventually(t, propagation, "the filter to apply", func() error {
			secret, n := githubToken(), nonce("probe")
			if _, err := probe.CallTool(ctx, &gomcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": n + " " + secret}}); err != nil {
				return err
			}
			if got := env.mcp.Received(n); len(got) != 1 || strings.Contains(got[0], secret) {
				return fmt.Errorf("not redacted yet")
			}
			return nil
		})

		c := harness.NewAPIKeyClient(env.A.BaseURL, u.Key)
		headers := map[string]string{"Accept": "application/json, text/event-stream", "Mcp-Protocol-Version": "2025-06-18"}
		init := c.Do(t, http.MethodPost, "/mcp-connect/"+target.serverID, map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "raw", "version": "1"}},
		}, headers)
		if init.Status != http.StatusOK {
			t.Fatalf("initialize: %s", init)
		}
		if sid := init.Header.Get("Mcp-Session-Id"); sid != "" {
			headers["Mcp-Session-Id"] = sid
		}
		c.Do(t, http.MethodPost, "/mcp-connect/"+target.serverID, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}, headers)

		call := func(id int, secret string) map[string]any {
			return map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call",
				"params": map[string]any{"name": "echo", "arguments": map[string]any{"text": "batched " + secret}}}
		}
		secret := githubToken()
		batch := c.Do(t, http.MethodPost, "/mcp-connect/"+target.serverID, []any{call(2, secret), call(3, secret)}, headers)
		if batch.Status == http.StatusOK && !strings.Contains(string(batch.Body), "error") {
			t.Fatalf("a batch was accepted: %s", batch)
		}
		if strings.Contains(string(batch.Body), secret) {
			t.Fatalf("a batch response echoed the secret: %s", batch)
		}
		if raw := env.mcp.RawRequests(secret); len(raw) != 0 {
			t.Fatalf("a batch reached the MCP server unfiltered: %v", raw)
		}

		// Two JSON values in one body can't be parsed as one message.
		secret2 := githubToken()
		body := mustJSON(call(4, secret2)) + " " + mustJSON(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled"})
		odd := c.Do(t, http.MethodPost, "/mcp-connect/"+target.serverID, body, headers)
		if raw := env.mcp.RawRequests(secret2); len(raw) != 0 {
			t.Errorf("an unparseable message reached the MCP server unfiltered (HTTP %d): %v", odd.Status, raw)
		}
	})
}
