package mcpgateway

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/obot-platform/obot/pkg/mcp"
)

type mcpHookCall struct {
	target  string
	message mcp.Message
}

type scriptedMCPHookRunner struct {
	mu    sync.Mutex
	calls []mcpHookCall
	run   func(mcp.SessionMessageHook, string) (mcp.SessionMessageHook, bool, error)
}

func (r *scriptedMCPHookRunner) RunHook(_ context.Context, _ mcp.HookServerConfigs, input mcp.SessionMessageHook, target string) (*mcp.SessionMessageHook, error) {
	r.mu.Lock()
	r.calls = append(r.calls, mcpHookCall{target: target, message: *input.Message})
	r.mu.Unlock()

	result := mcp.SessionMessageHook{Accept: true, Message: input.Message}
	hasOutput := true
	var err error
	if r.run != nil {
		result, hasOutput, err = r.run(input, target)
	}
	if !hasOutput {
		return nil, err
	}
	return &result, err
}

func (r *scriptedMCPHookRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func TestMCPProxyHooksScopeByMethodNameAndDirection(t *testing.T) {
	runner := new(scriptedMCPHookRunner)
	hooks := mcp.Hooks{
		{Name: "tools/list", Params: map[string]string{"direction": "request"}, Targets: []mcp.HookTarget{{Target: "policy/list"}}},
		{Name: "tools/call", Params: map[string]string{"name": "echo", "direction": "request"}, Targets: []mcp.HookTarget{{Target: "policy/echo"}}},
		{Name: "tools/call", Params: map[string]string{"name": "echo", "direction": "response"}, Targets: []mcp.HookTarget{{Target: "policy/echo-response"}}},
	}

	tests := []struct {
		name      string
		request   string
		wantCalls int
	}{
		{name: "method selector", request: `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, wantCalls: 1},
		{name: "method and name selector", request: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo"}}`, wantCalls: 1},
		{name: "different tool name", request: `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search"}}`},
		{name: "different method", request: `{"jsonrpc":"2.0","id":4,"method":"prompts/list","params":{}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := runner.callCount()
			if _, err := newHookProcessor(mustMCPHookRequest(t, tt.request), runner, hooks, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
			if got := runner.callCount() - before; got != tt.wantCalls {
				t.Fatalf("got %d request hook calls, want %d", got, tt.wantCalls)
			}
		})
	}

	request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"echo"}}`)
	hookProcessor, err := newHookProcessor(request, runner, hooks, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeResponse := runner.callCount()
	response := mcpHookResponse(`{"jsonrpc":"2.0","id":5,"result":{"content":[]}}`)
	if err := hookProcessor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	if got := runner.callCount() - beforeResponse; got != 1 {
		t.Fatalf("got %d response hook calls, want 1", got)
	}
}

func TestMCPProxyHooksChainAndContinueAfterErrors(t *testing.T) {
	t.Run("chains mutations", func(t *testing.T) {
		runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, target string) (mcp.SessionMessageHook, bool, error) {
			message := *input.Message
			switch target {
			case "policy/first":
				message.Params = json.RawMessage(`{"name":"echo","arguments":{"value":"first"}}`)
			case "policy/second":
				if !strings.Contains(string(message.Params), `"value":"first"`) {
					t.Fatalf("second hook did not receive first hook mutation: %s", message.Params)
				}
				message.Params = json.RawMessage(`{"name":"echo","arguments":{"value":"second"}}`)
			}
			return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: target}, true, nil
		}}
		hooks := mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/first"}, {Target: "policy/second"}}}}
		request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"value":"original"}}}`)

		if _, err := newHookProcessor(request, runner, hooks, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if runner.callCount() != 2 || !strings.Contains(string(body), `"value":"second"`) {
			t.Fatalf("hooks were not chained: calls=%d body=%s", runner.callCount(), body)
		}
	})

	t.Run("continues after runner error", func(t *testing.T) {
		runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, target string) (mcp.SessionMessageHook, bool, error) {
			if target == "policy/fail" {
				return mcp.SessionMessageHook{}, false, errors.New("policy unavailable")
			}
			return mcp.SessionMessageHook{Accept: true, Message: input.Message}, true, nil
		}}
		hooks := mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/fail"}, {Target: "policy/pass"}}}}
		hookProcessor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo"}}`), runner, hooks, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, blocked, hookErr := hookProcessor.blockedRequest()
		if runner.callCount() != 2 || !blocked || hookErr == nil || !strings.Contains(hookErr.Error(), "policy unavailable") {
			t.Fatalf("hook chain did not continue after error: calls=%d blocked=%v err=%v", runner.callCount(), blocked, hookErr)
		}
	})
}

func TestMCPProxyHooksMutateRequestAndResponseWithAudit(t *testing.T) {
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		message := *input.Message
		if len(message.Result) == 0 {
			message.Params = json.RawMessage(`{"name":"echo","arguments":{"value":"redacted request"}}`)
			return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "request redacted"}, true, nil
		}
		message.Result = json.RawMessage(`{"content":[{"type":"text","text":"redacted response"}]}`)
		return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "response redacted"}, true, nil
	}}
	hooks := mcp.Hooks{{Name: "tools/call", Params: map[string]string{"name": "echo"}, Targets: []mcp.HookTarget{{Target: "policy/redact"}}}}
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	collector := new(recordingProxyAuditCollector)
	request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo","arguments":{"value":"secret request"}}}`)
	request.Header.Set(mcpSessionHeader, "session-1")
	auditor, err := newProxyAudit(request, metadata, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	hookProcessor, err := newHookProcessor(request, runner, hooks, nil, auditor, nil)
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	requestBody, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(requestBody), "secret request") || !strings.Contains(string(requestBody), "redacted request") {
		t.Fatalf("request was not redacted: %s", requestBody)
	}

	response := mcpHookResponse(`{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"secret response"}]}}`)
	if err := hookProcessor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	if err := auditor.wrapResponse(response); err != nil {
		t.Fatal(err)
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(responseBody), "secret response") || !strings.Contains(string(responseBody), "redacted response") {
		t.Fatalf("response was not redacted: %s", responseBody)
	}
	if !strings.Contains(string(responseBody), mcp.HookMutationsMetaKey) {
		t.Fatalf("response omitted mutation metadata: %s", responseBody)
	}

	if len(collector.entries) != 2 {
		t.Fatalf("got %d audit entries, want request and response", len(collector.entries))
	}
	requestEntry, responseEntry := collector.entries[0], collector.entries[1]
	if !strings.Contains(string(requestEntry.MutatedRequestBody), "redacted request") {
		t.Fatalf("request audit omitted mutated request: %s", requestEntry.MutatedRequestBody)
	}
	if !strings.Contains(string(responseEntry.OriginalResponseBody), "secret response") || !strings.Contains(string(responseEntry.ResponseBody), "redacted response") {
		t.Fatalf("response audit did not retain both response bodies: original=%s response=%s", responseEntry.OriginalResponseBody, responseEntry.ResponseBody)
	}
	if len(requestEntry.WebhookStatuses) != 1 || requestEntry.WebhookStatuses[0].Type != "request" {
		t.Fatalf("unexpected request hook statuses: %#v", requestEntry.WebhookStatuses)
	}
	if len(responseEntry.WebhookStatuses) != 1 || responseEntry.WebhookStatuses[0].Type != "response" {
		t.Fatalf("unexpected response hook statuses: %#v", responseEntry.WebhookStatuses)
	}
}

func TestMCPProxyHookAuditSavesEachMutationCombination(t *testing.T) {
	for _, tt := range []struct {
		name           string
		mutateRequest  bool
		mutateResponse bool
	}{
		{name: "request only", mutateRequest: true},
		{name: "response only", mutateResponse: true},
		{name: "request and response", mutateRequest: true, mutateResponse: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
				message := *input.Message
				if len(message.Result) == 0 {
					if !tt.mutateRequest {
						return mcp.SessionMessageHook{Accept: true, Message: &message}, true, nil
					}
					message.Params = json.RawMessage(`{"name":"echo","arguments":{"value":"modified request"}}`)
					return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "request modified"}, true, nil
				}
				if !tt.mutateResponse {
					return mcp.SessionMessageHook{Accept: true, Message: &message}, true, nil
				}
				message.Result = json.RawMessage(`{"content":[{"type":"text","text":"modified response"}]}`)
				return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "response modified"}, true, nil
			}}
			hooks := mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/modify"}}}}
			metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
			collector := new(recordingProxyAuditCollector)
			request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"echo","arguments":{"value":"original request"}}}`)
			request.Header.Set(mcpSessionHeader, "session-1")
			auditor, err := newProxyAudit(request, metadata, collector, newMCPProxyTestStorage())
			if err != nil {
				t.Fatal(err)
			}
			hookProcessor, err := newHookProcessor(request, runner, hooks, nil, auditor, nil)
			if err != nil {
				t.Fatal(err)
			}
			auditor.recordRequest()
			response := mcpHookResponse(`{"jsonrpc":"2.0","id":11,"result":{"content":[{"type":"text","text":"original response"}]}}`)
			if err := hookProcessor.filterResponse(response); err != nil {
				t.Fatal(err)
			}
			if err := auditor.wrapResponse(response); err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Fatal(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}

			if len(collector.entries) != 2 {
				t.Fatalf("got %d audit entries, want request and response", len(collector.entries))
			}
			requestEntry, responseEntry := collector.entries[0], collector.entries[1]
			if !strings.Contains(string(requestEntry.RequestBody), "original request") {
				t.Fatalf("original request was not saved: %s", requestEntry.RequestBody)
			}
			if got := strings.Contains(string(requestEntry.MutatedRequestBody), "modified request"); got != tt.mutateRequest {
				t.Fatalf("mutated request saved=%v, want %v: %s", got, tt.mutateRequest, requestEntry.MutatedRequestBody)
			}
			if len(requestEntry.WebhookStatuses) != 1 || requestEntry.WebhookStatuses[0].Type != "request" {
				t.Fatalf("unexpected request hook statuses: %#v", requestEntry.WebhookStatuses)
			}
			if len(responseEntry.WebhookStatuses) != 1 || responseEntry.WebhookStatuses[0].Type != "response" {
				t.Fatalf("unexpected response hook statuses: %#v", responseEntry.WebhookStatuses)
			}
			if got := strings.Contains(string(responseEntry.OriginalResponseBody), "original response"); got != tt.mutateResponse {
				t.Fatalf("original response saved=%v, want %v: %s", got, tt.mutateResponse, responseEntry.OriginalResponseBody)
			}
			responseText := string(responseEntry.ResponseBody)
			if tt.mutateResponse {
				if !strings.Contains(responseText, "modified response") || strings.Contains(responseText, "original response") {
					t.Fatalf("modified response was not saved: %s", responseText)
				}
			} else if !strings.Contains(responseText, "original response") {
				t.Fatalf("unmodified response was not saved: %s", responseText)
			}
		})
	}
}

func TestMCPProxyHookAuditKeepsRequestMutationOnTransportError(t *testing.T) {
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		message := *input.Message
		message.Params = json.RawMessage(`{"name":"echo","arguments":{"value":"modified request"}}`)
		return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "request modified"}, true, nil
	}}
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	collector := new(recordingProxyAuditCollector)
	request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"echo","arguments":{"value":"original request"}}}`)
	auditor, err := newProxyAudit(request, metadata, collector, newMCPProxyTestStorage())
	if err != nil {
		t.Fatal(err)
	}
	_, err = newHookProcessor(request, runner, mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/modify"}}}}, nil, auditor, nil)
	if err != nil {
		t.Fatal(err)
	}
	auditor.recordRequest()
	auditor.recordTransportError(errors.New("upstream unavailable"), http.StatusBadGateway)

	if len(collector.entries) != 2 || !strings.Contains(string(collector.entries[0].MutatedRequestBody), "modified request") {
		t.Fatalf("request mutation was lost on transport error: %#v", collector.entries)
	}
}

func TestMCPProxyHooksRejectRequestAndDisallowedMutation(t *testing.T) {
	tests := []struct {
		name               string
		mutateDisallowed   bool
		response           mcp.SessionMessageHook
		wantErrorSubstring string
	}{
		{name: "explicit rejection", response: mcp.SessionMessageHook{Accept: false, Reason: "blocked by policy"}, wantErrorSubstring: "blocked by policy"},
		{name: "disallowed mutation", mutateDisallowed: true, response: mcp.SessionMessageHook{Accept: true, Mutated: true, Reason: "redaction"}, wantErrorSubstring: "mutation not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
				result := tt.response
				result.Message = input.Message
				return result, true, nil
			}}
			hooks := mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/check", MutateDisallowed: tt.mutateDisallowed}}}}
			hookProcessor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo"}}`), runner, hooks, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			body, blocked, hookErr := hookProcessor.blockedRequest()
			if !blocked || hookErr == nil || !strings.Contains(hookErr.Error(), tt.wantErrorSubstring) {
				t.Fatalf("request was not blocked as expected: blocked=%v err=%v", blocked, hookErr)
			}
			var message mcp.Message
			if err := json.Unmarshal(body, &message); err != nil {
				t.Fatal(err)
			}
			if message.Error == nil || mcp.MessageIDString(message.ID) != "1" {
				t.Fatalf("expected correlated JSON-RPC error, got %s", body)
			}
			if message.Error.Code != mcp.ErrRPCUnknown.Code || !strings.Contains(message.Error.Message, tt.wantErrorSubstring) || !strings.Contains(message.Error.Message, "MCP request blocked by hook") {
				t.Fatalf("JSON-RPC error did not explain the request block: %#v", message.Error)
			}
		})
	}
}

func TestMCPProxyHooksRejectResponse(t *testing.T) {
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		if len(input.Message.Result) == 0 {
			return mcp.SessionMessageHook{Accept: true, Message: input.Message}, true, nil
		}
		return mcp.SessionMessageHook{Accept: false, Message: input.Message, Reason: "response blocked"}, true, nil
	}}
	hookProcessor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo"}}`), runner, mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/check"}}}}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := mcpHookResponse(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"secret"}]}}`)
	if err := hookProcessor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"text":"secret"`) {
		t.Fatalf("response was not blocked: %s", body)
	}
	var message mcp.Message
	if err := json.Unmarshal(body, &message); err != nil {
		t.Fatal(err)
	}
	if message.Result != nil || message.Error == nil || message.Error.Code != mcp.ErrRPCUnknown.Code || message.Error.Message != "MCP response blocked by hook: response blocked" || mcp.MessageIDString(message.ID) != "1" {
		t.Fatalf("expected correlated JSON-RPC block error with hook reason, got %s", body)
	}
}

func TestMCPProxyHookBlockWithoutReasonStillExplainsFailure(t *testing.T) {
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		return mcp.SessionMessageHook{Accept: false, Message: input.Message}, true, nil
	}}
	hooks := mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/check"}}}}
	hookProcessor, err := newHookProcessor(mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo"}}`), runner, hooks, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, blocked, _ := hookProcessor.blockedRequest()
	if !blocked {
		t.Fatal("expected request to be blocked")
	}
	var message mcp.Message
	if err := json.Unmarshal(body, &message); err != nil {
		t.Fatal(err)
	}
	if message.Error == nil || message.Error.Message != `MCP request blocked by hook: hook "policy/check" did not provide a reason` {
		t.Fatalf("expected missing hook reason to be explained, got %s", body)
	}
}

func TestMCPProxyHooksFilterLegacySSEAcrossProcessors(t *testing.T) {
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		message := *input.Message
		if len(message.Result) == 0 {
			return mcp.SessionMessageHook{Accept: true, Message: &message}, true, nil
		}
		message.Result = json.RawMessage(`{"contents":[{"uri":"file:///readme","text":"redacted"}]}`)
		return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "resource redacted"}, true, nil
	}}
	hooks := mcp.Hooks{{Name: "resources/read", Params: map[string]string{"name": "file:///readme"}, Targets: []mcp.HookTarget{{Target: "policy/redact"}}}}
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	collector := new(recordingProxyAuditCollector)
	storageClient := newMCPProxyTestStorage()
	store := newHookCorrelationStore(storageClient, metadata)

	post := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":"request-1","method":"resources/read","params":{"uri":"file:///readme"}}`)
	post.URL.RawQuery = "sessionid=legacy-session"
	postAuditor, err := newProxyAudit(post, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	postHookProcessor, err := newHookProcessor(post, runner, hooks, nil, postAuditor, store)
	if err != nil {
		t.Fatal(err)
	}
	postAuditor.recordRequest()
	var upstreamRequest mcp.Message
	if err := json.NewDecoder(post.Body).Decode(&upstreamRequest); err != nil {
		t.Fatal(err)
	}
	if upstreamRequest.ID != "request-1" {
		t.Fatalf("legacy SSE upstream request ID changed: %v", upstreamRequest.ID)
	}
	accepted := &http.Response{StatusCode: http.StatusAccepted, Header: make(http.Header), Body: http.NoBody}
	if err := postHookProcessor.filterResponse(accepted); err != nil {
		t.Fatal(err)
	}
	if err := postAuditor.wrapResponse(accepted); err != nil {
		t.Fatal(err)
	}

	get, err := http.NewRequest(http.MethodGet, "http://obot.example/sse", nil)
	if err != nil {
		t.Fatal(err)
	}
	getAuditor, err := newProxyAudit(get, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	// This is deliberately a separate processor with no state from the POST,
	// matching the behavior when the two requests land on different replicas.
	getHookProcessor, err := newHookProcessor(get, runner, hooks, nil, getAuditor, store)
	if err != nil {
		t.Fatal(err)
	}
	upstreamResponse, err := json.Marshal(mcp.Message{
		JSONRPC: "2.0", ID: upstreamRequest.ID,
		Result: json.RawMessage(`{"contents":[{"uri":"file:///readme","text":"secret"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader("event: endpoint\ndata: http://upstream.example/messages?sessionid=legacy-session\n\n" +
			"event: message\ndata: " + string(upstreamResponse) + "\n\n")),
	}
	if err := getHookProcessor.filterResponse(stream); err != nil {
		t.Fatal(err)
	}
	if err := getAuditor.wrapResponse(stream); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"text":"secret"`) || !strings.Contains(string(body), `"text":"redacted"`) {
		t.Fatalf("legacy SSE response was not redacted: %s", body)
	}
	if !strings.Contains(string(body), `"id":"request-1"`) {
		t.Fatalf("legacy SSE response ID changed: %s", body)
	}
	if len(collector.entries) != 2 {
		t.Fatalf("got %d audit entries, want request and response", len(collector.entries))
	}
	if statuses := collector.entries[0].WebhookStatuses; len(statuses) != 1 || statuses[0].Type != "request" {
		t.Fatalf("unexpected request hook statuses: %#v", statuses)
	}
	if statuses := collector.entries[1].WebhookStatuses; len(statuses) != 1 || statuses[0].Type != "response" {
		t.Fatalf("unexpected response hook statuses: %#v", statuses)
	}
}

func TestMCPProxyHooksFilterServerRequestResponseAcrossProcessors(t *testing.T) {
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		message := *input.Message
		if len(message.Result) > 0 {
			message.Result = json.RawMessage(`{"roots":[{"uri":"file:///allowed"}]}`)
			return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "roots filtered"}, true, nil
		}
		return mcp.SessionMessageHook{Accept: true, Message: &message}, true, nil
	}}
	hooks := mcp.Hooks{{Name: "roots/list", Targets: []mcp.HookTarget{{Target: "policy/roots"}}}}
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	storageClient := newMCPProxyTestStorage()
	store := newHookCorrelationStore(storageClient, metadata)
	collector := new(recordingProxyAuditCollector)

	get, err := http.NewRequest(http.MethodGet, "http://obot.example/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	get.Header.Set(mcpSessionHeader, "session-1")
	getAuditor, err := newProxyAudit(get, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	getProcessor, err := newHookProcessor(get, runner, hooks, nil, getAuditor, store)
	if err != nil {
		t.Fatal(err)
	}
	stream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"roots/list\"}\n\n")),
	}
	if err := getProcessor.filterResponse(stream); err != nil {
		t.Fatal(err)
	}
	if err := getAuditor.wrapResponse(stream); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(stream.Body); err != nil {
		t.Fatal(err)
	}
	if err := stream.Body.Close(); err != nil {
		t.Fatal(err)
	}

	post := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":9,"result":{"roots":[{"uri":"file:///secret"}]}}`)
	post.Header.Set(mcpSessionHeader, "session-1")
	postAuditor, err := newProxyAudit(post, metadata, collector, storageClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newHookProcessor(post, runner, hooks, nil, postAuditor, store); err != nil {
		t.Fatal(err)
	}
	responseBody, err := io.ReadAll(post.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(responseBody), "file:///secret") || !strings.Contains(string(responseBody), "file:///allowed") {
		t.Fatalf("server-request response was not filtered: %s", responseBody)
	}
	if runner.callCount() != 2 {
		t.Fatalf("got %d hook calls, want server request and client response", runner.callCount())
	}
	if len(collector.entries) != 2 || collector.received[0] || !collector.received[1] {
		t.Fatalf("unexpected audit entries: count=%d received=%v", len(collector.entries), collector.received)
	}
	if statuses := collector.entries[0].WebhookStatuses; len(statuses) != 1 || statuses[0].Type != "request" {
		t.Fatalf("unexpected request hook statuses: %#v", statuses)
	}
	if statuses := collector.entries[1].WebhookStatuses; len(statuses) != 1 || statuses[0].Type != "response" {
		t.Fatalf("unexpected response hook statuses: %#v", statuses)
	}
}

func TestMCPProxyHooksFilterStreamableHTTPSSE(t *testing.T) {
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		message := *input.Message
		if len(message.Result) == 0 {
			return mcp.SessionMessageHook{Accept: true, Message: &message}, true, nil
		}
		message.Result = json.RawMessage(`{"contents":[{"uri":"file:///readme","text":"redacted"}]}`)
		return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message, Reason: "resource redacted"}, true, nil
	}}
	hooks := mcp.Hooks{{Name: "resources/read", Params: map[string]string{"name": "file:///readme"}, Targets: []mcp.HookTarget{{Target: "policy/redact"}}}}
	request := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":"request-1","method":"resources/read","params":{"uri":"file:///readme"}}`)
	hookProcessor, err := newHookProcessor(request, runner, hooks, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stream := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader("event: message\n" +
			"data: {\"jsonrpc\":\"2.0\",\"id\":\"request-1\",\"result\":{\"contents\":[{\"uri\":\"file:///readme\",\"text\":\"secret\"}]}}\n\n")),
	}
	if err := hookProcessor.filterResponse(stream); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"text":"secret"`) || !strings.Contains(string(body), `"text":"redacted"`) {
		t.Fatalf("Streamable HTTP SSE response was not redacted: %s", body)
	}
	if runner.callCount() != 2 {
		t.Fatalf("got %d hook calls, want request and response", runner.callCount())
	}
}

func TestMCPProxyHooksInspectEncodedResponse(t *testing.T) {
	hookProcessor, err := newHookProcessor(
		mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo"}}`),
		new(scriptedMCPHookRunner),
		mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/check"}}}},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("Content-Encoding", "gzip")
	headers.Set("ETag", `"original"`)
	response := &http.Response{
		Header:        headers,
		Body:          io.NopCloser(bytes.NewReader(compressed.Bytes())),
		ContentLength: int64(compressed.Len()),
	}
	if err := hookProcessor.filterResponse(response); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(body) || response.Header.Get("Content-Encoding") != "" || response.Header.Get("ETag") != "" || response.ContentLength != -1 {
		t.Fatalf("encoded response was not safely decoded: body=%s headers=%v length=%d", body, response.Header, response.ContentLength)
	}
}

func mustMCPHookRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	return request
}

func mcpHookResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
