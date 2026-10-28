package mcp

import (
	"bytes"
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
	"testing/synctest"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
)

type recoveryRequest struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type recoveryHTTPAction struct {
	serve func(http.ResponseWriter, *http.Request, recoveryRequest) bool
}

type reconnectWaitContext struct {
	context.Context
	waiting chan error
	once    sync.Once
}

type recoveryTestServer struct {
	server      *gomcp.Server
	handler     atomic.Pointer[gomcp.StreamableHTTPHandler]
	action      atomic.Pointer[recoveryHTTPAction]
	missing     sync.Map
	deleted     sync.Map
	initializes atomic.Int32
	deletes     atomic.Int32
	pings       atomic.Int32
	toolCalls   atomic.Int32
	badCursor   atomic.Bool
	endpoint    *httptest.Server
	manager     *SessionManager
	config      ServerConfig
}

func (c *reconnectWaitContext) Done() <-chan struct{} {
	// Signal after reconnect has selected the pending attempt, without timing sleeps.
	c.once.Do(func() { c.waiting <- nil })
	return c.Context.Done()
}

func newRecoveryTestServer(t *testing.T, jsonError bool) *recoveryTestServer {
	t.Helper()

	f := &recoveryTestServer{server: gomcp.NewServer(&gomcp.Implementation{Name: "filter"}, &gomcp.ServerOptions{PageSize: 1})}
	gomcp.AddTool(f.server, &gomcp.Tool{Name: "check"}, func(_ context.Context, _ *gomcp.CallToolRequest, input SessionMessageHook) (*gomcp.CallToolResult, SessionMessageHook, error) {
		f.toolCalls.Add(1)
		if input.Reason == "tool error" {
			return &gomcp.CallToolResult{IsError: true, Content: []gomcp.Content{&gomcp.TextContent{Text: "denied"}}}, SessionMessageHook{}, nil
		}
		return nil, SessionMessageHook{Accept: false, Reason: "blocked"}, nil
	})
	gomcp.AddTool(f.server, &gomcp.Tool{Name: "other"}, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	})
	f.server.AddReceivingMiddleware(func(next gomcp.MethodHandler) gomcp.MethodHandler {
		return func(ctx context.Context, method string, req gomcp.Request) (gomcp.Result, error) {
			if method == "tools/list" && f.badCursor.Load() && req.GetParams().(*gomcp.ListToolsParams).Cursor != "" {
				return nil, errors.New("cursor belongs to a lost session")
			}
			return next(ctx, method, req)
		}
	})
	f.reset()
	f.endpoint = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request recoveryRequest
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			if err := json.Unmarshal(body, &request); err != nil {
				t.Error(err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		if request.Method == "initialize" {
			f.initializes.Add(1)
		}
		if request.Method == "ping" {
			f.pings.Add(1)
		}
		_, missing := f.missing.Load(r.Header.Get("Mcp-Session-Id"))
		if r.Method == http.MethodDelete {
			f.deleted.Store(r.Header.Get("Mcp-Session-Id"), true)
		}
		if r.Method == http.MethodDelete && missing {
			f.deletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if action := f.action.Load(); action != nil && action.serve(w, r, request) {
			return
		}
		if jsonError && r.Method == http.MethodPost && missing {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error":   map[string]any{"code": -32001, "message": "Session not found"},
			})
			return
		}
		f.handler.Load().ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		// Handler resets deliberately orphan server-side sessions, like a restart.
		for session := range f.server.Sessions() {
			_ = session.Close()
		}
		f.endpoint.Close()
	})
	f.manager = &SessionManager{
		backend:                   &kubernetesBackend{},
		remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true},
	}
	f.config = ServerConfig{MCPServerName: "filter", UserID: "system", Runtime: types.RuntimeRemote, URL: f.endpoint.URL}
	t.Cleanup(f.manager.Close)

	return f
}

func (f *recoveryTestServer) reset(clients ...*Client) {
	for _, client := range clients {
		f.missing.Store(client.ID(), true)
	}
	f.handler.Store(gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return f.server }, nil))
}

func (f *recoveryTestServer) client(t *testing.T, scope string) *Client {
	t.Helper()

	client, err := f.manager.clientForServerWithScope(t.Context(), scope, f.config)
	if err != nil {
		t.Fatal(err)
	}
	// System clients normally have no JWT; give this fixture a valid cache lifetime.
	client.jwt = jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))})
	return client
}

func receiveRecoveryError(t *testing.T, result <-chan error) error {
	t.Helper()

	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not finish")
		return nil
	}
}

func TestSharedSessionRecoveryAfterRestart(t *testing.T) {
	for _, jsonError := range []bool{false, true} {
		t.Run(fmt.Sprintf("json_error=%t", jsonError), func(t *testing.T) {
			f := newRecoveryTestServer(t, jsonError)
			client := f.client(t, "default")
			hook := f.client(t, hookClientScope)
			original := client.ClientSession
			oldID := client.ID()
			iterator := original.Tools(t.Context(), nil)

			f.reset(client, hook)
			tools, err := f.manager.ListTools(t.Context(), f.config)
			if err != nil || len(tools) != 2 {
				t.Fatalf("default ListTools: tools=%v, error=%v", tools, err)
			}
			output, err := NewHookRunner(f.manager).RunHook(t.Context(), HookServerConfigs{"policy": f.config}, SessionMessageHook{}, "policy/check")
			if err != nil || output == nil || output.Accept || output.Reason != "blocked" {
				t.Fatalf("hook: output=%#v, error=%v", output, err)
			}
			tools, err = collectAll(iterator)
			if err != nil || len(tools) != 2 {
				t.Fatalf("retained iterator: tools=%v, error=%v", tools, err)
			}
			if err := original.Ping(t.Context(), nil); err != nil {
				t.Fatal(err)
			}

			f.reset(client)
			if err := original.Ping(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
			cached, err := f.manager.clientForServer(t.Context(), f.config)
			if err != nil || cached != client || client.ClientSession != original || client.ID() == oldID {
				t.Fatalf("cache/reference changed or session did not: cached=%p, client=%p, error=%v", cached, client, err)
			}
			if f.initializes.Load() != 5 || f.deletes.Load() != 0 {
				t.Fatalf("initializes=%d, missing DELETEs=%d", f.initializes.Load(), f.deletes.Load())
			}
			if client.InitializeResult().ProtocolVersion != "2025-11-25" {
				t.Fatal("retained SDK references require the request/response protocol")
			}
		})
	}
}

func TestSharedSessionRecoveryConcurrent(t *testing.T) {
	f := newRecoveryTestServer(t, true)
	client := f.client(t, "default")
	f.reset(client)

	const callers = 24
	start := make(chan struct{})
	results := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			results <- client.Ping(t.Context(), nil)
		}()
	}
	close(start)
	for range callers {
		if err := receiveRecoveryError(t, results); err != nil {
			t.Fatal(err)
		}
	}
	if f.initializes.Load() != 2 || f.deletes.Load() != 0 {
		t.Fatalf("initializes=%d, missing DELETEs=%d", f.initializes.Load(), f.deletes.Load())
	}
	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatalf("replacement was closed: %v", err)
	}
}

func TestSharedSessionRecoveryDoesNotReplayAcceptedCall(t *testing.T) {
	f := newRecoveryTestServer(t, true)
	client := f.client(t, "default")
	accepted := make(chan error, 1)
	var executions atomic.Int32
	f.action.Store(&recoveryHTTPAction{serve: func(w http.ResponseWriter, r *http.Request, request recoveryRequest) bool {
		if request.Method == "tools/call" {
			executions.Add(1)
			// The operation has executed, but its response stream is still open.
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			accepted <- nil
			<-r.Context().Done()
			return true
		}
		return false
	}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.CallTool(ctx, &gomcp.CallToolParams{Name: "check"})
		result <- err
	}()
	if err := receiveRecoveryError(t, accepted); err != nil {
		t.Fatal(err)
	}

	f.reset(client)
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := receiveRecoveryError(t, result); err == nil {
		t.Fatal("expected the interrupted response to fail")
	}
	if executions.Load() != 1 {
		t.Fatalf("accepted operation executed %d times", executions.Load())
	}
}

func TestSharedSessionRecoverySubscriptionLifetime(t *testing.T) {
	subscribed := make(chan error, 1)
	updates := make(chan string, 1)
	server := gomcp.NewServer(&gomcp.Implementation{Name: "subscriptions"}, &gomcp.ServerOptions{
		SubscribeHandler: func(context.Context, *gomcp.SubscribeRequest) error {
			subscribed <- nil
			return nil
		},
		UnsubscribeHandler: func(context.Context, *gomcp.UnsubscribeRequest) error {
			return nil
		},
	})
	server.AddResource(&gomcp.Resource{URI: "file:///test", Name: "test"}, func(context.Context, *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
		return &gomcp.ReadResourceResult{}, nil
	})
	endpoint := httptest.NewServer(gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, &gomcp.StreamableHTTPOptions{Stateless: true}))
	sdk := gomcp.NewClient(&gomcp.Implementation{Name: "test"}, &gomcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *gomcp.ResourceUpdatedNotificationRequest) {
			updates <- req.Params.URI
		},
	})
	client, err := newRecoveringClient(t.Context(), sdk, gomcp.StreamableClientTransport{Endpoint: endpoint.URL})
	if err != nil {
		endpoint.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		endpoint.CloseClientConnections()
		// The pinned SDK needs a write to retire a stateless subscription stream.
		_ = server.ResourceUpdated(context.Background(), &gomcp.ResourceUpdatedNotificationParams{URI: "file:///test"})
		endpoint.Close()
	})

	if err := client.Subscribe(t.Context(), &gomcp.SubscribeParams{URI: "file:///test"}); err != nil {
		t.Fatal(err)
	}
	if err := receiveRecoveryError(t, subscribed); err != nil {
		t.Fatal(err)
	}
	if err := server.ResourceUpdated(t.Context(), &gomcp.ResourceUpdatedNotificationParams{URI: "file:///test"}); err != nil {
		t.Fatal(err)
	}
	select {
	case uri := <-updates:
		if uri != "file:///test" {
			t.Fatalf("unexpected resource update: %s", uri)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscription stopped after setup returned")
	}
}

func TestSharedSessionRecoveryBounded(t *testing.T) {
	f := newRecoveryTestServer(t, true)
	client := f.client(t, "default")
	f.action.Store(&recoveryHTTPAction{serve: func(w http.ResponseWriter, _ *http.Request, request recoveryRequest) bool {
		if request.Method != "ping" {
			return false
		}
		http.Error(w, "session missing", http.StatusNotFound)
		return true
	}})

	if err := client.Ping(t.Context(), nil); !errors.Is(err, gomcp.ErrSessionMissing) {
		t.Fatalf("second session loss: %v", err)
	}
	if f.initializes.Load() != 2 || f.pings.Load() != 2 {
		t.Fatalf("unbounded replay: initializes=%d, pings=%d", f.initializes.Load(), f.pings.Load())
	}

	f.action.Store(nil)
	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatalf("later call could not recover: %v", err)
	}
	if f.initializes.Load() != 3 {
		t.Fatalf("initializes=%d", f.initializes.Load())
	}
}

func TestSharedSessionRecoveryReconnectFailure(t *testing.T) {
	f := newRecoveryTestServer(t, false)
	client := f.client(t, "default")
	f.reset(client)
	f.action.Store(&recoveryHTTPAction{serve: func(w http.ResponseWriter, _ *http.Request, request recoveryRequest) bool {
		if request.Method != "initialize" {
			return false
		}
		http.Error(w, "reconnect unavailable", http.StatusInternalServerError)
		return true
	}})

	if err := client.Ping(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "Internal Server Error") {
		t.Fatalf("reconnect failure: %v", err)
	}
	if f.initializes.Load() != 2 {
		t.Fatalf("failed initialization was retried: %d", f.initializes.Load())
	}

	f.action.Store(nil)
	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatalf("failed reconnect wedged recovery: %v", err)
	}
}

func TestSharedSessionRecoveryWaitingAndClose(t *testing.T) {
	for _, closeClient := range []bool{false, true} {
		t.Run(fmt.Sprintf("close=%t", closeClient), func(t *testing.T) {
			f := newRecoveryTestServer(t, false)
			client := f.client(t, "default")
			f.reset(client)
			initializing := make(chan error, 1)
			release := make(chan struct{})
			defer close(release)
			f.action.Store(&recoveryHTTPAction{serve: func(_ http.ResponseWriter, r *http.Request, request recoveryRequest) bool {
				if request.Method != "initialize" {
					return false
				}
				initializing <- nil
				select {
				case <-release:
					return false
				case <-r.Context().Done():
					return true
				}
			}})
			leader := make(chan error, 1)
			go func() { leader <- client.Ping(t.Context(), nil) }()
			_ = receiveRecoveryError(t, initializing)

			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			waiter := make(chan error, 1)
			go func() { waiter <- client.Ping(ctx, nil) }()
			if err := receiveRecoveryError(t, waiter); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("waiting caller did not cancel: %v", err)
			}

			if closeClient {
				closed := make(chan error, 1)
				go func() { closed <- client.Close() }()
				if err := receiveRecoveryError(t, closed); err != nil {
					t.Fatal(err)
				}
				if err := receiveRecoveryError(t, leader); !errors.Is(err, gomcp.ErrConnectionClosed) {
					t.Fatalf("Close did not cancel reconnect: %v", err)
				}
				if err := client.Wait(); err != nil {
					t.Fatal(err)
				}
				if err := client.Ping(t.Context(), nil); !errors.Is(err, gomcp.ErrConnectionClosed) {
					t.Fatalf("closed client reconnected: %v", err)
				}
			} else {
				release <- struct{}{}
				if err := receiveRecoveryError(t, leader); err != nil {
					t.Fatal(err)
				}
			}
			if f.initializes.Load() != 2 || f.deletes.Load() != 0 {
				t.Fatalf("initializes=%d, missing DELETEs=%d", f.initializes.Load(), f.deletes.Load())
			}
		})
	}
}

func TestSharedSessionRecoveryDoesNotReplayOtherFailures(t *testing.T) {
	for _, failure := range []string{"HTTP 500", "tool error", "canceled", "deadline", "timeout", "connection closed", "connection interrupted"} {
		t.Run(failure, func(t *testing.T) {
			f := newRecoveryTestServer(t, false)
			scope := "default"
			if failure == "tool error" {
				scope = hookClientScope
			}
			client := f.client(t, scope)
			ctx := t.Context()
			switch failure {
			case "HTTP 500":
				f.action.Store(&recoveryHTTPAction{serve: func(w http.ResponseWriter, _ *http.Request, request recoveryRequest) bool {
					if request.Method != "ping" {
						return false
					}
					http.Error(w, "failed", http.StatusInternalServerError)
					return true
				}})
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "deadline":
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			case "timeout":
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
				f.action.Store(&recoveryHTTPAction{serve: func(_ http.ResponseWriter, r *http.Request, request recoveryRequest) bool {
					if request.Method != "ping" {
						return false
					}
					<-r.Context().Done()
					return true
				}})
			case "connection closed":
				if err := client.ClientSession.Close(); err != nil {
					t.Fatal(err)
				}
			case "connection interrupted":
				f.action.Store(&recoveryHTTPAction{serve: func(w http.ResponseWriter, _ *http.Request, request recoveryRequest) bool {
					if request.Method != "ping" {
						return false
					}
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return true
					}
					_ = conn.Close()
					return true
				}})
			}

			if failure == "tool error" {
				_, err := NewHookRunner(f.manager).RunHook(ctx, HookServerConfigs{"policy": f.config}, SessionMessageHook{Reason: "tool error"}, "policy/check")
				if err == nil || !strings.Contains(err.Error(), "denied") || f.toolCalls.Load() != 1 {
					t.Fatalf("tool calls=%d, error=%v", f.toolCalls.Load(), err)
				}
			} else if err := client.Ping(ctx, nil); err == nil {
				t.Fatal("expected failure")
			}
			if f.initializes.Load() != 1 || f.pings.Load() > 1 {
				t.Fatalf("replayed %s: initializes=%d, pings=%d", failure, f.initializes.Load(), f.pings.Load())
			}
		})
	}
}

func TestSharedSessionRecoveryInitial404(t *testing.T) {
	f := newRecoveryTestServer(t, false)
	f.action.Store(&recoveryHTTPAction{serve: func(w http.ResponseWriter, _ *http.Request, _ recoveryRequest) bool {
		http.Error(w, "endpoint not found", http.StatusNotFound)
		return true
	}})

	_, err := f.manager.clientForServer(t.Context(), f.config)
	if err == nil || f.initializes.Load() != 1 {
		t.Fatalf("initial failure was retried: initializes=%d, error=%v", f.initializes.Load(), err)
	}
}

func TestSharedSessionRecoveryDoesNotRestartPagination(t *testing.T) {
	f := newRecoveryTestServer(t, false)
	client := f.client(t, "default")
	seen := 0
	for tool, err := range client.Tools(t.Context(), nil) {
		if err != nil {
			if seen != 1 || !strings.Contains(err.Error(), "cursor belongs to a lost session") {
				t.Fatalf("seen=%d, error=%v", seen, err)
			}
			break
		}
		if tool == nil {
			t.Fatal("nil tool")
		}
		seen++
		f.reset(client)
		f.badCursor.Store(true)
	}
	if seen != 1 || f.initializes.Load() != 2 {
		t.Fatalf("pagination replayed or lost its error: seen=%d, initializes=%d", seen, f.initializes.Load())
	}
}

func TestSharedSessionRecoveryPreservesParamsAndOptions(t *testing.T) {
	f := newRecoveryTestServer(t, true)
	f.config.Headers = []string{"X-Recovery-Test=secret"}
	var mu sync.Mutex
	var calls []string
	var implementations []gomcp.Implementation
	f.action.Store(&recoveryHTTPAction{serve: func(_ http.ResponseWriter, r *http.Request, request recoveryRequest) bool {
		mu.Lock()
		defer mu.Unlock()

		if r.Method == http.MethodPost && r.Header.Get("X-Recovery-Test") != "secret" {
			t.Error("lost configured HTTP header")
		}
		switch request.Method {
		case "initialize":
			var params gomcp.InitializeParams
			if err := json.Unmarshal(request.Params, &params); err != nil {
				t.Error(err)
			}
			implementations = append(implementations, *params.ClientInfo)
		case "tools/call":
			calls = append(calls, string(request.Params))
		}
		return false
	}})
	client, err := f.manager.clientForServerWithOptions(t.Context(), "default", f.config, ClientOption{ClientName: "recovery-test", ClientVersion: "test-version"})
	if err != nil {
		t.Fatal(err)
	}
	original := client.ClientSession
	f.reset(client)
	params := &gomcp.CallToolParams{
		Meta:      gomcp.Meta{"progressToken": "progress-1", "trace": "unchanged"},
		Name:      "check",
		Arguments: SessionMessageHook{Reason: "metadata"},
	}
	if _, err := original.CallTool(t.Context(), params); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 2 || calls[0] != calls[1] || !strings.Contains(calls[1], "progress-1") {
		t.Fatalf("replay changed params/metadata: %v", calls)
	}
	if len(implementations) != 2 || !reflect.DeepEqual(implementations[0], implementations[1]) || implementations[1].Name != "recovery-test" || implementations[1].Version != "test-version" {
		t.Fatalf("reconnect changed SDK implementation: %v", implementations)
	}
}

func TestSharedSessionRecoveryStateless404(t *testing.T) {
	f := newRecoveryTestServer(t, false)
	f.handler.Store(gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return f.server }, &gomcp.StreamableHTTPOptions{Stateless: true}))
	client := f.client(t, "default")
	if client.ID() != "" || client.InitializeResult().ProtocolVersion != "2026-07-28" {
		t.Fatal("modern stateless protocol negotiation changed")
	}
	f.action.Store(&recoveryHTTPAction{serve: func(w http.ResponseWriter, _ *http.Request, request recoveryRequest) bool {
		if request.Method != "ping" {
			return false
		}
		http.Error(w, "endpoint missing", http.StatusNotFound)
		return true
	}})

	if err := client.Ping(t.Context(), nil); err == nil {
		t.Fatal("expected sessionless endpoint error")
	}
	if f.pings.Load() != 1 || f.initializes.Load() != 0 {
		t.Fatalf("sessionless error replayed: pings=%d, initializes=%d", f.pings.Load(), f.initializes.Load())
	}
}

func TestSharedSessionRecoveryInitiatorCancellation(t *testing.T) {
	f := newRecoveryTestServer(t, false)
	client := f.client(t, "default")
	f.reset(client)
	initializing := make(chan error, 1)
	f.action.Store(&recoveryHTTPAction{serve: func(_ http.ResponseWriter, r *http.Request, request recoveryRequest) bool {
		if request.Method != "initialize" {
			return false
		}
		initializing <- nil
		<-r.Context().Done()
		return true
	}})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- client.Ping(ctx, nil) }()
	_ = receiveRecoveryError(t, initializing)

	waitCtx := &reconnectWaitContext{Context: t.Context(), waiting: make(chan error, 1)}
	waiter := make(chan error, 1)
	client.recovery.mu.Lock()
	stale := client.recovery.current
	client.recovery.mu.Unlock()
	go func() {
		_, err := client.recovery.reconnect(waitCtx, stale)
		waiter <- err
	}()
	_ = receiveRecoveryError(t, waitCtx.waiting)
	f.action.Store(nil)

	cancel()
	if err := receiveRecoveryError(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("initiating cancellation lost: %v", err)
	}

	if err := receiveRecoveryError(t, waiter); err != nil {
		t.Fatalf("live waiter inherited the initiator's cancellation: %v", err)
	}

	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatalf("canceled reconnect wedged future calls: %v", err)
	}
	if f.initializes.Load() != 3 {
		t.Fatalf("initializes=%d", f.initializes.Load())
	}
}

func TestSharedSessionRecoveryInitializationDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stale := &recoverableSession{}
		r := &sessionRecovery{ctx: t.Context(), current: stale}
		var connections atomic.Int32
		started := make(chan struct{})
		r.connect = func(ctx context.Context) (*recoverableSession, error) {
			if connections.Add(1) == 1 {
				close(started)
			}
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) != time.Minute {
				t.Errorf("initialization deadline = %v, present = %t", deadline, ok)
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		leader := make(chan error, 1)
		go func() {
			_, err := r.reconnect(t.Context(), stale)
			leader <- err
		}()
		<-started
		waitCtx := &reconnectWaitContext{Context: t.Context(), waiting: make(chan error, 1)}
		waiter := make(chan error, 1)
		go func() {
			_, err := r.reconnect(waitCtx, stale)
			waiter <- err
		}()
		if err := <-waitCtx.waiting; err != nil {
			t.Fatal(err)
		}

		// Virtual time advances only after both callers are waiting.
		for _, result := range []<-chan error{leader, waiter} {
			if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("initialization error = %v", err)
			}
		}
		if connections.Load() != 1 || r.pending != nil || t.Context().Err() != nil {
			t.Fatal("timeout retried, retained pending state, or canceled the caller")
		}
	})
}

func TestSharedSessionRecoveryPreservesEarlierDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	stale := &recoverableSession{}
	wantErr := errors.New("connection failed")
	r := &sessionRecovery{
		ctx:     t.Context(),
		current: stale,
		connect: func(connectCtx context.Context) (*recoverableSession, error) {
			if got, ok := connectCtx.Deadline(); !ok || !got.Equal(deadline) {
				t.Errorf("caller deadline changed: %v", got)
			}
			return nil, wantErr
		},
	}

	if _, err := r.reconnect(ctx, stale); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("initialization canceled the caller's context")
	}
}

func TestSharedSessionRecoveryCloseCurrent(t *testing.T) {
	f := newRecoveryTestServer(t, false)
	client := f.client(t, "default")
	original := client.ClientSession
	f.reset(client)
	if err := client.Ping(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	currentID := client.ID()
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.deleted.Load(currentID); !ok || f.deletes.Load() != 0 {
		t.Fatal("Close did not terminate just the current session")
	}
	if err := original.Ping(t.Context(), nil); !errors.Is(err, gomcp.ErrConnectionClosed) {
		t.Fatalf("retained reference reconnected after Close: %v", err)
	}
}

func TestSharedSessionRecoveryCloseDiscardsReplacement(t *testing.T) {
	f := newRecoveryTestServer(t, false)
	client := f.client(t, "default")
	f.reset(client)
	connect := client.recovery.connect
	connected := make(chan string, 1)
	release := make(chan struct{})
	defer close(release)
	client.recovery.connect = func(ctx context.Context) (*recoverableSession, error) {
		session, err := connect(ctx)
		if err == nil {
			connected <- session.ID()
			<-release
		}
		return session, err
	}
	result := make(chan error, 1)
	go func() { result <- client.Ping(t.Context(), nil) }()
	var replacementID string
	select {
	case replacementID = <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement did not initialize")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	release <- struct{}{}
	if err := receiveRecoveryError(t, result); !errors.Is(err, gomcp.ErrConnectionClosed) {
		t.Fatalf("replacement published after Close: %v", err)
	}
	if _, ok := f.deleted.Load(replacementID); !ok || f.deletes.Load() != 0 {
		t.Fatal("unpublished replacement leaked or missing session was deleted")
	}
}

func TestSharedSessionRecoveryCloseCancelsRequest(t *testing.T) {
	f := newRecoveryTestServer(t, false)
	client := f.client(t, "default")
	started := make(chan error, 1)
	f.action.Store(&recoveryHTTPAction{serve: func(_ http.ResponseWriter, r *http.Request, request recoveryRequest) bool {
		if request.Method != "ping" {
			return false
		}
		started <- nil
		<-r.Context().Done()
		return true
	}})
	result := make(chan error, 1)
	go func() { result <- client.Ping(t.Context(), nil) }()
	_ = receiveRecoveryError(t, started)
	closed := make(chan error, 1)
	go func() { closed <- client.Close() }()
	if err := receiveRecoveryError(t, closed); err != nil {
		t.Fatal(err)
	}
	if err := receiveRecoveryError(t, result); err == nil {
		t.Fatal("in-flight request did not cancel on Close")
	}
}

func TestSessionTransport(t *testing.T) {
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
			transport := &sessionTransport{missing: &atomic.Bool{}, base: roundTripFunc(func(*http.Request) (*http.Response, error) {
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
