package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeHookMCPClient struct {
	params *gomcp.CallToolParams
	result *gomcp.CallToolResult
	err    error
	calls  int
}

func TestMCPHookRunnerRecoversAfterHTTPServerRestart(t *testing.T) {
	for _, jsonError := range []bool{false, true} {
		t.Run(fmt.Sprintf("json_error=%t", jsonError), func(t *testing.T) {
			server := gomcp.NewServer(&gomcp.Implementation{Name: "filter"}, nil)
			gomcp.AddTool(server, &gomcp.Tool{Name: "check"}, func(context.Context, *gomcp.CallToolRequest, SessionMessageHook) (*gomcp.CallToolResult, SessionMessageHook, error) {
				return nil, SessionMessageHook{Accept: false, Reason: "blocked"}, nil
			})
			newHandler := func() *gomcp.StreamableHTTPHandler {
				return gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
			}
			var handler atomic.Pointer[gomcp.StreamableHTTPHandler]
			handler.Store(newHandler())
			var missingSession atomic.Value
			missingSession.Store("")
			var missingSessionDeletes atomic.Int32
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete && r.Header.Get("Mcp-Session-Id") == missingSession.Load().(string) {
					missingSessionDeletes.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if jsonError && r.Method == http.MethodPost && r.Header.Get("Mcp-Session-Id") != "" && r.Header.Get("Mcp-Session-Id") == missingSession.Load().(string) {
					var request struct {
						ID any `json:"id"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"jsonrpc": "2.0",
						"id":      request.ID,
						"error":   map[string]any{"code": -32001, "message": "Session not found"},
					})
					return
				}
				handler.Load().ServeHTTP(w, r)
			}))
			defer endpoint.Close()

			manager := &SessionManager{}
			config := ServerConfig{MCPServerName: "sms1policy", UserID: "user-1"}
			sessions := &sync.Map{}
			manager.sessions.Store(config.MCPServerName, sessions)
			key := clientID(config, hookClientScope)
			runner := NewHookRunner(manager)
			connections := 0
			runner.clientForServer = func(ctx context.Context, _ ServerConfig) (hookMCPClient, error) {
				if cached, ok := sessions.Load(key); ok {
					return cached.(*Client), nil
				}
				client := gomcp.NewClient(&gomcp.Implementation{Name: "test"}, nil)
				httpClient := *endpoint.Client()
				httpClient.Transport = &hookSessionTransport{base: httpClient.Transport}
				session, err := client.Connect(ctx, &gomcp.StreamableClientTransport{Endpoint: endpoint.URL, HTTPClient: &httpClient, DisableStandaloneSSE: true}, nil)
				if err != nil {
					return nil, err
				}
				t.Cleanup(func() { _ = session.Close() })
				connections++
				result := &Client{ClientSession: session}
				sessions.Store(key, result)
				return result, nil
			}
			if _, err := runner.clientForServer(t.Context(), config); err != nil {
				t.Fatal(err)
			}

			cached, _ := sessions.Load(key)
			missingSession.Store(cached.(*Client).ID())

			// Replacing the handler loses its sessions, as restarting the filter would.
			handler.Store(newHandler())
			output, err := runner.RunHook(t.Context(), HookServerConfigs{"policy": config}, SessionMessageHook{}, "policy/check")
			if err != nil {
				t.Fatal(err)
			}
			if connections != 2 || output == nil || output.Accept || output.Reason != "blocked" {
				t.Fatalf("connections=%d, output=%#v", connections, output)
			}
			if missingSessionDeletes.Load() != 0 {
				t.Fatal("recovery sent DELETE for a missing session; a stalled DELETE would block the retry")
			}
		})
	}
}

func TestHookSessionTransport(t *testing.T) {
	for _, tc := range []struct {
		name        string
		method      string
		sessionID   string
		status      int
		wantMissing bool
	}{
		{
			name:        "existing session missing",
			method:      http.MethodPost,
			sessionID:   "stale",
			status:      http.StatusNotFound,
			wantMissing: true,
		},
		{
			name:   "initial connection not found",
			method: http.MethodPost,
			status: http.StatusNotFound,
		},
		{
			name:      "tool error on existing session",
			method:    http.MethodPost,
			sessionID: "active",
			status:    http.StatusOK,
		},
		{
			name:      "server failure",
			method:    http.MethodPost,
			sessionID: "active",
			status:    http.StatusInternalServerError,
		},
		{
			name:      "unsupported SSE endpoint",
			method:    http.MethodGet,
			sessionID: "active",
			status:    http.StatusNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const body = `{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"Session not found"}}`
			response := &http.Response{
				StatusCode: tc.status,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
			defer response.Body.Close()
			transport := &hookSessionTransport{base: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response, nil
			})}
			request, err := http.NewRequest(tc.method, "https://example.com/mcp", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Mcp-Session-Id", tc.sessionID)

			got, err := transport.RoundTrip(request)
			if err != nil || got != response {
				t.Fatalf("response=%v, error=%v", got, err)
			}
			if tc.wantMissing {
				if got.StatusCode != http.StatusNotFound || got.Body != http.NoBody || got.ContentLength != 0 {
					t.Fatalf("missing-session response was not normalized: %#v", got)
				}
				return
			}
			data, err := io.ReadAll(got.Body)
			if err != nil || string(data) != body {
				t.Fatalf("response body changed: %s, error=%v", data, err)
			}
		})
	}
}

func (c *fakeHookMCPClient) CallTool(_ context.Context, params *gomcp.CallToolParams) (*gomcp.CallToolResult, error) {
	c.params = params
	c.calls++
	return c.result, c.err
}

func TestMCPHookRunnerSessionRecovery(t *testing.T) {
	reconnectErr := errors.New("reconnect failed")
	for _, tc := range []struct {
		name          string
		firstErr      error
		secondErr     error
		connectErr    error
		wantErr       error
		wantClients   int
		wantRetries   int
		wantEvictions int
	}{
		{
			name:          "retry is bounded and failed replacement is evicted",
			firstErr:      fmt.Errorf("transport: %w", gomcp.ErrSessionMissing),
			secondErr:     gomcp.ErrSessionMissing,
			wantErr:       gomcp.ErrSessionMissing,
			wantClients:   2,
			wantRetries:   1,
			wantEvictions: 2,
		},
		{
			name:          "reconnection failure propagates",
			firstErr:      gomcp.ErrSessionMissing,
			connectErr:    reconnectErr,
			wantErr:       reconnectErr,
			wantClients:   2,
			wantEvictions: 1,
		},
		{
			name:        "ambiguous connection failure is not retried",
			firstErr:    gomcp.ErrConnectionClosed,
			wantErr:     gomcp.ErrConnectionClosed,
			wantClients: 1,
		},
		{
			name:        "cancellation is not retried",
			firstErr:    context.Canceled,
			wantErr:     context.Canceled,
			wantClients: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := &fakeHookMCPClient{err: tc.firstErr}
			second := &fakeHookMCPClient{err: tc.secondErr}
			server := ServerConfig{MCPServerName: "sms1policy", UserID: "user-1"}
			var clients, evictions int
			runner := &SessionManagerHookRunner{
				clientForServer: func(_ context.Context, got ServerConfig) (hookMCPClient, error) {
					clients++
					if !reflect.DeepEqual(got, server) {
						t.Fatalf("unexpected server: %#v", got)
					}
					if clients == 1 {
						return first, nil
					}
					if evictions == 0 {
						t.Fatal("reconnected before evicting stale client")
					}
					return second, tc.connectErr
				},
				invalidateClient: func(_ ServerConfig, client hookMCPClient) {
					evictions++
					if (evictions == 1 && client != first) || (evictions == 2 && client != second) {
						t.Fatal("evicted the wrong client")
					}
				},
			}

			input := SessionMessageHook{Accept: true, Message: &Message{Method: "tools/call"}}
			_, err := runner.RunHook(t.Context(), HookServerConfigs{"policy": server}, input, "policy/check")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got error %v, want %v", err, tc.wantErr)
			}
			if clients != tc.wantClients || second.calls != tc.wantRetries || evictions != tc.wantEvictions || first.calls != 1 {
				t.Fatalf("clients=%d, retries=%d, evictions=%d, first calls=%d", clients, second.calls, evictions, first.calls)
			}
			if second.calls > 0 && !reflect.DeepEqual(first.params, second.params) {
				t.Fatal("retry changed the tool arguments")
			}
		})
	}
}

func TestMCPHookRunnerInvalidationPreservesReplacement(t *testing.T) {
	manager := &SessionManager{}
	server := ServerConfig{MCPServerName: "sms1policy", UserID: "user-1"}
	stale, replacement := &Client{}, &Client{}
	sessions := &sync.Map{}
	key := clientID(server, hookClientScope)
	sessions.Store(key, replacement)
	manager.sessions.Store(server.MCPServerName, sessions)

	NewHookRunner(manager).invalidateClient(server, stale)

	if got, ok := sessions.Load(key); !ok || got != replacement {
		t.Fatal("invalidating a stale client removed its replacement")
	}
}

func TestMCPHookRunnerCallsConfiguredServerTool(t *testing.T) {
	client := &fakeHookMCPClient{result: &gomcp.CallToolResult{
		StructuredContent: map[string]any{"accept": true, "reason": "allowed"},
	}}
	wantServer := ServerConfig{MCPServerName: "sms1policy", UserID: "user-1"}
	runner := &SessionManagerHookRunner{clientForServer: func(_ context.Context, server ServerConfig) (hookMCPClient, error) {
		if server.MCPServerName != wantServer.MCPServerName || server.UserID != wantServer.UserID {
			t.Fatalf("got hook server %#v, want %#v", server, wantServer)
		}
		return client, nil
	}}
	input := SessionMessageHook{Accept: true, Message: &Message{Method: "tools/call"}}

	output, err := runner.RunHook(t.Context(), HookServerConfigs{"policy": wantServer}, input, "policy/validate")
	if err != nil {
		t.Fatal(err)
	}
	if output == nil || !output.Accept || output.Reason != "allowed" {
		t.Fatalf("unexpected hook output: %#v", output)
	}
	if client.params == nil || client.params.Name != "validate" || !reflect.DeepEqual(client.params.Arguments, input) {
		t.Fatalf("unexpected tool call params: %#v", client.params)
	}
}

func TestMCPHookRunnerDecodesJSONTextContent(t *testing.T) {
	client := &fakeHookMCPClient{result: &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: `{"accept":false,"reason":"blocked"}`}},
	}}
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return client, nil
	}}
	output, err := runner.RunHook(
		t.Context(), HookServerConfigs{"policy": {MCPServerName: "sms1policy"}},
		SessionMessageHook{Message: &Message{Method: "tools/call"}}, "policy/check",
	)
	if err != nil {
		t.Fatal(err)
	}
	if output == nil || output.Accept || output.Reason != "blocked" {
		t.Fatalf("unexpected JSON text hook output: %#v", output)
	}
}

func TestMCPHookRunnerReturnsToolErrorMessage(t *testing.T) {
	client := &fakeHookMCPClient{result: &gomcp.CallToolResult{
		IsError: true,
		Content: []gomcp.Content{&gomcp.TextContent{Text: "policy service denied the request"}},
	}}
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return client, nil
	}}

	_, err := runner.RunHook(
		t.Context(), HookServerConfigs{"policy": {MCPServerName: "sms1policy"}},
		SessionMessageHook{}, "policy/check",
	)
	if err == nil || !strings.Contains(err.Error(), "policy service denied the request") {
		t.Fatalf("expected hook tool error message, got %v", err)
	}
}

func TestMCPHookRunnerValidatesTargetAndServer(t *testing.T) {
	runner := &SessionManagerHookRunner{clientForServer: func(context.Context, ServerConfig) (hookMCPClient, error) {
		return nil, errors.New("should not be called")
	}}

	if _, err := runner.RunHook(t.Context(), nil, SessionMessageHook{}, "invalid"); err == nil || !strings.Contains(err.Error(), "expected server/tool") {
		t.Fatalf("expected invalid target error, got %v", err)
	}
	if _, err := runner.RunHook(t.Context(), nil, SessionMessageHook{}, "policy/check"); err == nil || !strings.Contains(err.Error(), "is not configured") {
		t.Fatalf("expected missing server error, got %v", err)
	}
}
