package mcpgateway

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/stretchr/testify/require"
)

type disabledProxyAuditCollector struct {
	recordingProxyAuditCollector
}

func (*disabledProxyAuditCollector) MCPAuditLogEnabled() bool {
	return false
}

func TestDisabledProxyAuditDoesNotCaptureBodies(t *testing.T) {
	for _, contentType := range []string{"application/json", "text/event-stream"} {
		t.Run(contentType, func(t *testing.T) {
			collector := new(disabledProxyAuditCollector)
			req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo"}}`)
			audit, err := newProxyAudit(req, map[string]string{"mcpID": "mcp-1", "userID": "user-1"}, collector, newMCPProxyTestStorage())
			require.NoError(t, err)
			require.Empty(t, audit.entry.RequestBody)
			require.Empty(t, audit.entry.RequestHeaders)

			requestBody := req.Body
			hooks, err := newHookProcessor(req, nil, nil, nil, audit, nil)
			require.NoError(t, err)
			require.Equal(t, requestBody, req.Body, "disabled hooks and auditing must not reread the request")
			audit.recordRequest()

			// Invalid gzip is intentional: audit-only decompression must not run.
			body := io.NopCloser(strings.NewReader(strings.Repeat("payload", 10000)))
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {contentType}, "Content-Encoding": {"gzip"}},
				Body:       body,
			}
			require.NoError(t, hooks.filterResponse(resp))
			require.NoError(t, audit.wrapResponse(resp))
			require.Equal(t, body, resp.Body, "audit response capture must be bypassed")
			require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))
			_, err = io.Copy(io.Discard, resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			audit.recordTransportError(io.ErrUnexpectedEOF, http.StatusBadGateway)
			require.Empty(t, collector.entries)
		})
	}
}

func TestDisabledProxyAuditPreservesVirtualSessions(t *testing.T) {
	collector := new(disabledProxyAuditCollector)
	storage := newMCPProxyTestStorage()
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"client","version":"1"}}}`)
	audit, err := newProxyAudit(req, metadata, collector, storage)
	require.NoError(t, err)
	resp := mcpHookResponse(`{"jsonrpc":"2.0","id":1,"result":{}}`)
	body := resp.Body
	require.NoError(t, audit.wrapResponse(resp))
	require.Equal(t, body, resp.Body)
	require.NoError(t, resp.Body.Close())
	sessionID := resp.Header.Get(mcpSessionHeader)
	require.NotEmpty(t, sessionID)

	followup := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	followup.Header.Set(mcpSessionHeader, sessionID)
	next, err := newProxyAudit(followup, metadata, collector, storage)
	require.NoError(t, err)
	require.Empty(t, followup.Header.Get(mcpSessionHeader))
	require.Equal(t, sessionID, next.entry.SessionID)
	require.Equal(t, "client", next.entry.ClientName)
	require.Empty(t, collector.entries)
}

func TestDisabledProxyAuditPreservesHooks(t *testing.T) {
	collector := new(disabledProxyAuditCollector)
	runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
		return mcp.SessionMessageHook{Accept: false, Message: input.Message, Reason: "blocked by policy"}, true, nil
	}}
	hookConfig := mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/block"}}}}
	req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo"}}`)
	audit, err := newProxyAudit(req, map[string]string{"mcpID": "mcp-1", "userID": "user-1"}, collector, newMCPProxyTestStorage())
	require.NoError(t, err)
	hooks, err := newHookProcessor(req, runner, hookConfig, nil, audit, nil)
	require.NoError(t, err)
	body, blocked, hookErr := hooks.blockedRequest()
	require.True(t, blocked)
	require.Error(t, hookErr)
	require.Contains(t, string(body), "blocked by policy")
	audit.recordBlockedRequest(body, hookErr)
	require.Empty(t, audit.entry.WebhookStatuses)
	require.Empty(t, collector.entries)
}

func TestDisabledProxyAuditPreservesRequestAndResponseMutation(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "JSON"
		if streaming {
			name = "SSE"
		}

		t.Run(name, func(t *testing.T) {
			collector := new(disabledProxyAuditCollector)
			calls := 0
			runner := &scriptedMCPHookRunner{run: func(input mcp.SessionMessageHook, _ string) (mcp.SessionMessageHook, bool, error) {
				calls++
				message := *input.Message
				if len(message.Result) == 0 {
					message.Params = []byte(`{"name":"echo","arguments":{"value":"redacted request"}}`)
				} else {
					message.Result = []byte(`{"content":[{"type":"text","text":"redacted response"}]}`)
				}
				return mcp.SessionMessageHook{Accept: true, Mutated: true, Message: &message}, true, nil
			}}
			hookConfig := mcp.Hooks{{Name: "tools/call", Targets: []mcp.HookTarget{{Target: "policy/redact"}}}}
			req := mustMCPHookRequest(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo","arguments":{"value":"secret request"}}}`)
			audit, err := newProxyAudit(req, map[string]string{"mcpID": "mcp-1", "userID": "user-1"}, collector, newMCPProxyTestStorage())
			require.NoError(t, err)
			hooks, err := newHookProcessor(req, runner, hookConfig, nil, audit, nil)
			require.NoError(t, err)
			requestBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Contains(t, string(requestBody), "redacted request")
			require.NotContains(t, string(requestBody), "secret request")

			responseBody := `{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"secret response"}]}}`
			resp := mcpHookResponse(responseBody)
			if streaming {
				resp.Header.Set("Content-Type", "text/event-stream")
				resp.Body = io.NopCloser(strings.NewReader("data: " + responseBody + "\n\n"))
			}
			require.NoError(t, hooks.filterResponse(resp))
			filteredBody := resp.Body
			require.NoError(t, audit.wrapResponse(resp))
			require.Equal(t, filteredBody, resp.Body, "only the hook's response wrapper should remain")
			response, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Contains(t, string(response), "redacted response")
			require.NotContains(t, string(response), "secret response")
			require.Equal(t, 2, calls)
			require.Nil(t, hooks.audit)
			require.Empty(t, audit.responseHooksByID)
			require.Empty(t, audit.entry.MutatedRequestBody)
			require.Empty(t, collector.entries)
		})
	}
}
