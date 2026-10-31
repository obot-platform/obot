package mcpgateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestBuiltinClientOnlyServesVMCPLoopbacks(t *testing.T) {
	h := &Handler{}
	var servedUser user.Info
	h.RegisterBuiltin("test", func(req api.Context) http.Handler {
		servedUser = req.User
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "ok")
		})
	})

	tests := []struct {
		name        string
		groups      []string
		builtinName string
		wantStatus  int
	}{
		{
			name:        "vMCP loopback reaches the built-in server",
			groups:      []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
			builtinName: "test",
		},
		{
			name:        "direct connection is refused",
			groups:      []string{types.GroupMCP, types.GroupAuthenticated},
			builtinName: "test",
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "admin browser session is refused",
			groups:      types.RoleAdmin.Groups(),
			builtinName: "test",
			wantStatus:  http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servedUser = nil
			caller := &user.DefaultInfo{Name: "admin", UID: "1", Groups: tt.groups}
			client, url, err := h.builtinClient(api.Context{User: caller}, obotmcp.ServerConfig{BuiltinName: tt.builtinName})
			if tt.wantStatus != 0 {
				var httpErr *types.ErrHTTP
				if !errors.As(err, &httpErr) || httpErr.Code != tt.wantStatus {
					t.Fatalf("builtinClient() error = %v, want status %d", err, tt.wantStatus)
				}
				return
			}
			if err != nil {
				t.Fatalf("builtinClient() error = %v", err)
			}

			resp, err := client.Post(url, "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatalf("Post() error = %v", err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if resp.StatusCode != http.StatusOK || string(body) != "ok" {
				t.Fatalf("response = %d %q, want 200 %q", resp.StatusCode, body, "ok")
			}
			if servedUser != caller {
				t.Fatalf("built-in server served user %v, want the vMCP's caller %v", servedUser, caller)
			}
		})
	}
}

func TestBuiltinTransportStreamsResponses(t *testing.T) {
	release := make(chan struct{})
	handlerDone := make(chan error, 1)
	transport := builtinTransport{server: func(api.Context) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusAccepted)
			// Headers changed after they're written aren't sent.
			w.Header().Set("X-Late", "true")
			_, _ = io.WriteString(w, "first\n")
			<-release
			_, _ = io.WriteString(w, "second\n")
			// Stay open like a subscriptions/listen stream until the caller goes away.
			<-r.Context().Done()
			handlerDone <- r.Context().Err()
		})
	}}

	req, err := http.NewRequest(http.MethodPost, "http://builtin.obot.internal/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := roundTripWithin(t, transport, req)
	if resp.StatusCode != http.StatusAccepted || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("response = %d %v, want 202 with an event stream", resp.StatusCode, resp.Header)
	}
	if resp.Header.Get("X-Late") != "" {
		t.Fatal("a header set after WriteHeader was sent")
	}

	buf := make([]byte, len("first\n"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil || string(buf) != "first\n" {
		t.Fatalf("first read = %q, %v", buf, err)
	}
	close(release)
	buf = make([]byte, len("second\n"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil || string(buf) != "second\n" {
		t.Fatalf("second read = %q, %v", buf, err)
	}

	// Closing the body is how the caller abandons a stream; the handler must see it.
	_ = resp.Body.Close()
	select {
	case err := <-handlerDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("handler context error = %v, want canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler didn't see the caller close the body")
	}
}

func TestBuiltinTransportAnswersHandlersThatWriteNothing(t *testing.T) {
	transport := builtinTransport{server: func(api.Context) http.Handler {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}}
	req, err := http.NewRequest(http.MethodPost, "http://builtin.obot.internal/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := roundTripWithin(t, transport, req)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK || len(body) != 0 {
		t.Fatalf("response = %d %q, %v, want an empty 200", resp.StatusCode, body, err)
	}
}

func TestBuiltinTransportGivesUpWhenTheCallerDoes(t *testing.T) {
	handlerDone := make(chan struct{})
	transport := builtinTransport{server: func(api.Context) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer close(handlerDone)
			<-r.Context().Done()
			// A handler that writes after the caller gave up must not block on the unread body.
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "too late")
		})
	}}
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://builtin.obot.internal/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 1)
	go func() {
		resp, err := transport.RoundTrip(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		errs <- err
	}()
	cancel()
	select {
	case err := <-errs:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RoundTrip() error = %v, want canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RoundTrip() didn't return after the caller gave up")
	}
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler blocked writing a response nobody reads")
	}
}

func roundTripWithin(t *testing.T, transport http.RoundTripper, req *http.Request) *http.Response {
	t.Helper()
	type result struct {
		resp *http.Response
		err  error
	}
	results := make(chan result, 1)
	go func() {
		resp, err := transport.RoundTrip(req)
		results <- result{resp: resp, err: err}
	}()
	select {
	case r := <-results:
		if r.err != nil {
			t.Fatalf("RoundTrip() error = %v", r.err)
		}
		return r.resp
	case <-time.After(5 * time.Second):
		t.Fatal("RoundTrip() didn't return once headers were written")
		return nil
	}
}

// A vMCP keeps a built-in component after the server is turned off. The component then has no
// tools, so the vMCP keeps working with its other components.
func TestUnavailableBuiltinServerHasNoTools(t *testing.T) {
	h := &Handler{}
	caller := &user.DefaultInfo{Name: "admin", UID: "1", Groups: []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}}
	client, url, err := h.builtinClient(api.Context{User: caller}, obotmcp.ServerConfig{BuiltinName: "obot-admin"})
	if err != nil {
		t.Fatalf("builtinClient() error = %v", err)
	}

	session, err := gomcp.NewClient(&gomcp.Implementation{Name: "mmmcp", Version: "1"}, nil).
		Connect(t.Context(), &gomcp.StreamableClientTransport{Endpoint: url, HTTPClient: client}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer session.Close()
	result, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(result.Tools) != 0 {
		t.Fatalf("tools = %v, want none", result.Tools)
	}
}
