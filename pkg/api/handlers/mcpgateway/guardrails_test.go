package mcpgateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/mcp"
)

const (
	testSecret = "AKIAIOSFODNN7EXAMPLE"
)

type guardrailStore struct{}

// guardrailChain is a server with two filters, in resource name order: a webhook filter
// (mwv1a-policy) and a guardrail filter (mwv1b-secrets) that checks for secrets.
type guardrailChain struct {
	runner *scriptedMCPHookRunner
	hooks  mcp.Hooks
	// servers are the filter servers the hook runner calls.
	servers    mcp.HookServerConfigs
	guardrails *mcpGuardrails
}

func (s *guardrailStore) ListGuardrailPolicies(context.Context) ([]types.GuardrailPolicy, error) {
	return nil, nil
}

func (s *guardrailStore) ListGuardrails(context.Context) ([]types.Guardrail, error) {
	return []types.Guardrail{{ID: "secrets", Name: "secrets", Type: "secrets", Config: json.RawMessage(`{}`)}}, nil
}

func (s *guardrailStore) GuardrailSecrets(context.Context, string) (map[string]string, error) {
	return nil, nil
}

func newGuardrailChain(t *testing.T, action string, selectors apitypes.MCPSelectors, run func(mcp.SessionMessageHook, string) (mcp.SessionMessageHook, bool, error)) guardrailChain {
	t.Helper()
	return newGuardrailChainWithConfig(t, apitypes.MCPGuardrailFilterConfig{GuardrailIDs: []string{"secrets"}, Action: action, SamplingRate: 100}, selectors, run)
}

func newGuardrailChainWithConfig(t *testing.T, config apitypes.MCPGuardrailFilterConfig, selectors apitypes.MCPSelectors, run func(mcp.SessionMessageHook, string) (mcp.SessionMessageHook, bool, error)) guardrailChain {
	t.Helper()
	serverConfig := mcp.ServerConfig{
		MCPServerName: "ms1files", MCPServerDisplayName: "files", UserID: "alice",
		Webhooks: []mcp.Webhook{
			{
				Name: "mwv1a-policy", DisplayName: "Policy", URL: "http://policy.example/mcp", ToolName: "check",
				Definitions: selectors, MutateAllowed: true,
			},
			{
				Name: "mwv1b-secrets", DisplayName: "No secrets", Definitions: selectors,
				Guardrail: &config,
			},
		},
	}
	hooks, servers := mcp.ServerHookConfig(serverConfig)
	if _, ok := servers["mwv1b-secrets"]; ok || len(servers) != 1 {
		t.Fatalf("a guardrail filter must not have a filter server: %#v", servers)
	}
	engine := guardrails.New(&guardrailStore{}, nil)
	return guardrailChain{
		runner:     &scriptedMCPHookRunner{run: run},
		hooks:      hooks,
		servers:    servers,
		guardrails: newMCPGuardrails(engine, guardrails.Identity{UserID: "alice"}, serverConfig, http.Header{"Authorization": []string{"Bearer x"}}),
	}
}

func (c guardrailChain) process(t *testing.T, request *http.Request, audit *proxyAudit, store *hookCorrelationStore) *hookProcessor {
	t.Helper()
	processor, err := newHookProcessor(request, c.runner, c.hooks, c.servers, audit, store, nil, c.guardrails)
	if err != nil {
		t.Fatal(err)
	}
	return processor
}

func readBody(t *testing.T, body io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGuardrailFilterRedactsToolCallsInTheChain(t *testing.T) {
	// The webhook filter runs first and adds a secret, so the guardrail filter, which runs
	// second, must see the webhook's mutation to redact it. On the way back, the webhook
	// sees the server's result before the guardrail filter redacts it.
	var webhookSawResult string
	chain := newGuardrailChain(t, apitypes.MCPGuardrailActionRedact, apitypes.MCPSelectors{{Method: "tools/call"}}, func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		message := *input.Message
		if len(message.Result) > 0 {
			webhookSawResult = string(message.Result)
			return mcp.SessionMessageHook{Accept: true, Message: &message}, true, nil
		}
		message.Params = json.RawMessage(`{"name":"write","arguments":{"text":"key ` + testSecret + `"}}`)
		return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "policy rewrote"}, true, nil
	})
	collector := new(recordingProxyAuditCollector)
	request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write","arguments":{"text":"hello"}}}`)
	auditor, err := newProxyAudit(request, map[string]string{"mcpID": "ms1files", "userID": "alice"}, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	processor := chain.process(t, request, auditor, nil)
	auditor.recordRequest()
	if _, blocked, err := processor.blockedRequest(); blocked {
		t.Fatalf("redaction must not block: %v", err)
	}
	if body := readBody(t, request.Body); strings.Contains(body, testSecret) || !strings.Contains(body, "[REDACTED:AWS_ACCESS_KEY_ID]") {
		t.Fatalf("arguments not redacted before the server: %s", body)
	}

	response := mcpHookResponse(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"wrote ` + testSecret + `"}]}}`)
	if err := processor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	if err := auditor.wrapResponse(response); err != nil {
		t.Fatal(err)
	}
	body := readBody(t, response.Body)
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, testSecret) || !strings.Contains(body, "[REDACTED:AWS_ACCESS_KEY_ID]") || !strings.Contains(body, mcp.HookMutationsMetaKey) {
		t.Fatalf("result not redacted before the client: %s", body)
	}
	if !strings.Contains(webhookSawResult, testSecret) {
		t.Fatalf("the webhook filter should run before the guardrail filter on responses, saw %s", webhookSawResult)
	}

	// Audit statuses name each filter, in chain order.
	if len(collector.entries) != 2 {
		t.Fatalf("got %d audit entries, want request and response", len(collector.entries))
	}
	for i, entry := range collector.entries {
		statuses := entry.WebhookStatuses
		if len(statuses) != 2 ||
			statuses[0].Name != "Policy" || statuses[0].Tool != "mwv1a-policy/check" ||
			statuses[1].Name != "No secrets" || statuses[1].Tool != guardrailHookTool || statuses[1].Status != "mutated" ||
			statuses[1].Message != "redacted by guardrail filter No secrets" {
			t.Fatalf("entry %d: unexpected hook statuses %#v", i, statuses)
		}
	}
}

// A fail-open guardrail filter whose guardrail fails lets the call through, and the MCP audit
// log's status for the filter says which guardrail failed and why.
func TestGuardrailFilterFailOpenErrorIsInTheAuditLog(t *testing.T) {
	chain := newGuardrailChainWithConfig(t, apitypes.MCPGuardrailFilterConfig{
		GuardrailIDs: []string{"gone"},
		Action:       apitypes.MCPGuardrailActionBlock,
		SamplingRate: 100,
	}, nil, nil)
	collector := new(recordingProxyAuditCollector)
	request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write","arguments":{"text":"hello"}}}`)
	auditor, err := newProxyAudit(request, map[string]string{"mcpID": "ms1files", "userID": "alice"}, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	processor := chain.process(t, request, auditor, nil)
	auditor.recordRequest()
	if _, blocked, err := processor.blockedRequest(); blocked {
		t.Fatalf("a fail-open filter must not block: %v", err)
	}
	if len(collector.entries) != 1 {
		t.Fatalf("got %d audit entries, want the request", len(collector.entries))
	}
	statuses := collector.entries[0].WebhookStatuses
	want := `guardrail "gone" failed: guardrail gone does not exist; allowed because the filter fails open`
	if len(statuses) != 2 || statuses[1].Name != "No secrets" || statuses[1].Status != "ok" || statuses[1].Message != want {
		t.Fatalf("unexpected hook statuses %#v, want the guardrail filter's message %q", statuses, want)
	}
}

func TestGuardrailFilterBlocksToolCalls(t *testing.T) {
	chain := newGuardrailChain(t, apitypes.MCPGuardrailActionBlock, nil, nil)

	processor := chain.process(t, mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write","arguments":{"text":"key `+testSecret+`"}}}`), nil, nil)
	body, blocked, _ := processor.blockedRequest()
	if !blocked || !strings.Contains(string(body), "No secrets") || strings.Contains(string(body), testSecret) {
		t.Fatalf("expected the tool call to be blocked, got %v %s", blocked, body)
	}
	if chain.runner.callCount() != 1 {
		t.Fatalf("the webhook filter should still run in the chain, got %d calls", chain.runner.callCount())
	}

	// Clean calls pass, and a result containing a secret is replaced by an error.
	processor = chain.process(t, mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read","arguments":{"path":"a.txt"}}}`), nil, nil)
	if _, blocked, err := processor.blockedRequest(); blocked {
		t.Fatalf("a clean call must not be blocked: %v", err)
	}
	response := mcpHookResponse(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"` + testSecret + `"}]}}`)
	if err := processor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	if body := readBody(t, response.Body); strings.Contains(body, testSecret) || !strings.Contains(body, `"error"`) {
		t.Fatalf("expected an error response, got %s", body)
	}

	// Methods without content to check pass.
	processor = chain.process(t, mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`), nil, nil)
	if _, blocked, err := processor.blockedRequest(); blocked {
		t.Fatalf("tools/list must not be blocked: %v", err)
	}
}

func TestGuardrailFilterChecksResourcesAndPrompts(t *testing.T) {
	chain := newGuardrailChain(t, apitypes.MCPGuardrailActionRedact, apitypes.MCPSelectors{{Method: "resources/read"}, {Method: "prompts/get"}}, nil)

	// resources/read: the text contents of the result are redacted.
	processor := chain.process(t, mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"file:///keys"}}`), nil, nil)
	response := mcpHookResponse(`{"jsonrpc":"2.0","id":1,"result":{"contents":[{"uri":"file:///keys","text":"key ` + testSecret + `"}]}}`)
	if err := processor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	if body := readBody(t, response.Body); strings.Contains(body, testSecret) || !strings.Contains(body, "[REDACTED:AWS_ACCESS_KEY_ID]") {
		t.Fatalf("resource contents not redacted: %s", body)
	}

	// prompts/get: arguments on the way in and messages on the way out are redacted.
	request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"review","arguments":{"code":"`+testSecret+`"}}}`)
	processor = chain.process(t, request, nil, nil)
	if body := readBody(t, request.Body); strings.Contains(body, testSecret) {
		t.Fatalf("prompt arguments not redacted: %s", body)
	}
	response = mcpHookResponse(`{"jsonrpc":"2.0","id":2,"result":{"messages":[{"role":"user","content":{"type":"text","text":"review ` + testSecret + `"}}]}}`)
	if err := processor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	if body := readBody(t, response.Body); strings.Contains(body, testSecret) || !strings.Contains(body, "[REDACTED:AWS_ACCESS_KEY_ID]") {
		t.Fatalf("prompt messages not redacted: %s", body)
	}

	// tools/call isn't selected, so it passes unchanged.
	request = mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write","arguments":{"text":"`+testSecret+`"}}}`)
	chain.process(t, request, nil, nil)
	if body := readBody(t, request.Body); !strings.Contains(body, testSecret) {
		t.Fatalf("an unselected method must not be filtered: %s", body)
	}

	// Blocking resources/read and prompts/get works the same way.
	chain = newGuardrailChain(t, apitypes.MCPGuardrailActionBlock, apitypes.MCPSelectors{{Method: "resources/read"}, {Method: "prompts/get"}}, nil)
	processor = chain.process(t, mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"file:///keys"}}`), nil, nil)
	response = mcpHookResponse(`{"jsonrpc":"2.0","id":4,"result":{"contents":[{"uri":"file:///keys","text":"` + testSecret + `"}]}}`)
	if err := processor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	if body := readBody(t, response.Body); strings.Contains(body, testSecret) || !strings.Contains(body, `"error"`) {
		t.Fatalf("expected the resource to be blocked, got %s", body)
	}
	processor = chain.process(t, mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":5,"method":"prompts/get","params":{"name":"review","arguments":{"code":"`+testSecret+`"}}}`), nil, nil)
	if _, blocked, _ := processor.blockedRequest(); !blocked {
		t.Fatal("expected the prompt request to be blocked")
	}
}

func TestGuardrailFilterRejectsBatchesAndUncheckableCalls(t *testing.T) {
	chain := newGuardrailChain(t, apitypes.MCPGuardrailActionBlock, nil, nil)

	processor := chain.process(t, mustMCPHookRequest(t, `[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write","arguments":{"text":"key `+testSecret+`"}}}]`), nil, nil)
	if body, blocked, _ := processor.blockedRequest(); !blocked || !strings.Contains(string(body), "batches") {
		t.Fatalf("a batch must be rejected while filters apply, got %v %s", blocked, body)
	}

	processor = chain.process(t, mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":["write"]}`), nil, nil)
	if _, blocked, _ := processor.blockedRequest(); !blocked {
		t.Fatal("a tool call whose params can't be checked must be blocked")
	}

	// Two JSON values in one body can't be decoded as one message; a lenient server could
	// still act on the first.
	processor = chain.process(t, mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write","arguments":{"text":"key `+testSecret+`"}}} {"jsonrpc":"2.0","method":"notifications/cancelled"}`), nil, nil)
	if body, blocked, _ := processor.blockedRequest(); !blocked || strings.Contains(string(body), testSecret) {
		t.Fatalf("an unparseable message must be rejected while filters apply, got %v %s", blocked, body)
	}
}

func TestUnparseableMessagesPassWithoutFilters(t *testing.T) {
	processor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1} {}`), nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, blocked, _ := processor.blockedRequest(); blocked {
		t.Fatal("without filters, the gateway must leave unparseable messages to the server")
	}
}

func TestGuardrailFilterChecksResultsOnGETStreams(t *testing.T) {
	chain := newGuardrailChain(t, apitypes.MCPGuardrailActionRedact, nil, nil)
	store := newHookCorrelationStore(newMCPProxyTestStorage(), map[string]string{"mcpID": "mcp-1", "userID": "user-1"})

	// The call is made with POST, and its result arrives on a separate GET stream.
	post := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"read","arguments":{}}}`)
	post.Header.Set(mcpSessionHeader, "s1")
	chain.process(t, post, nil, store)
	get := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	get.Header.Set(mcpSessionHeader, "s1")
	processor := chain.process(t, get, nil, store)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"" + testSecret + "\"}]}}\n\n")),
	}
	if err := processor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	if body := readBody(t, response.Body); strings.Contains(body, testSecret) {
		t.Fatalf("a result on the GET stream must be checked: %s", body)
	}
}

func TestGuardrailFiltersRunWithoutAHookRunner(t *testing.T) {
	chain := newGuardrailChain(t, apitypes.MCPGuardrailActionBlock, nil, nil)
	processor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write","arguments":{"text":"`+testSecret+`"}}}`), nil, chain.hooks, nil, nil, nil, nil, chain.guardrails)
	if err != nil {
		t.Fatal(err)
	}
	if _, blocked, _ := processor.blockedRequest(); !blocked {
		t.Fatal("guardrail filters must run without a hook runner")
	}
}
