package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	"github.com/obot-platform/mmmcp/component"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
)

// fakeCompositeTokens encodes token claims as JSON and, like the persistent
// token service, rejects tokens that have expired.
type fakeCompositeTokens struct{}

func (fakeCompositeTokens) NewToken(_ context.Context, tokenContext persistent.TokenContext) (*jwt.Token, string, error) {
	data, err := json.Marshal(tokenContext)
	return nil, string(data), err
}

func (fakeCompositeTokens) DecodeToken(_ context.Context, token string) (*persistent.TokenContext, error) {
	var tokenContext persistent.TokenContext
	if err := json.Unmarshal([]byte(token), &tokenContext); err != nil {
		return nil, err
	}
	if !tokenContext.ExpiresAt.After(time.Now()) {
		return nil, errors.New("token is expired")
	}
	return &tokenContext, nil
}

func compositeTestTokenContext() persistent.TokenContext {
	return persistent.TokenContext{
		Audience:         "https://obot.example/mcp-connect-composite/vmcpi1test",
		UserID:           "user-1",
		UserName:         "user",
		UserGroups:       []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
		MCPID:            "vmcpi1test",
		AuthorizedMCPIDs: []string{"ms1gmail"},
	}
}

func compositeTestToken(t *testing.T, tokenContext persistent.TokenContext, expiresAt time.Time) string {
	t.Helper()
	tokenContext.ExpiresAt = persistent.NewTime(expiresAt)
	_, token, err := fakeCompositeTokens{}.NewToken(t.Context(), tokenContext)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func compositeTestAuthContext(ctx context.Context, authorization string) context.Context {
	headers := http.Header{}
	if authorization != "" {
		headers.Set("Authorization", authorization)
	}
	return component.ContextWithRequestHeaders(ctx, headers)
}

func TestCompositeComponentAuthRemintsExpiringLoopbackToken(t *testing.T) {
	tokens := fakeCompositeTokens{}
	original := compositeTestTokenContext()
	// Still valid, but inside the refresh window, like a pooled session's token
	// once the request that opened the session is long past.
	initial := compositeTestToken(t, original, time.Now().Add(5*time.Second))

	handler, err := compositeComponentAuth{ctx: t.Context(), tokens: tokens}.OAuthHandler(compositeTestAuthContext(t.Context(), "Bearer "+initial), mmmcpconfig.Server{})
	if err != nil {
		t.Fatal(err)
	}
	if handler == nil {
		t.Fatal("OAuthHandler() = nil, want a handler for a composite loopback token")
	}
	tokenSource, err := handler.TokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken == initial {
		t.Fatal("token source returned the expiring token, want a re-minted token")
	}
	if earliest := time.Now().Add(compositeLoopbackTokenTTL - time.Minute); token.Expiry.Before(earliest) {
		t.Fatalf("re-minted token expiry = %v, want after %v", token.Expiry, earliest)
	}

	decoded, err := tokens.DecodeToken(t.Context(), token.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Audience != original.Audience || decoded.UserID != original.UserID || decoded.UserName != original.UserName || decoded.MCPID != original.MCPID {
		t.Fatalf("re-minted token identity = %#v, want %#v", decoded, original)
	}
	if !slices.Equal(decoded.UserGroups, original.UserGroups) || !slices.Equal(decoded.AuthorizedMCPIDs, original.AuthorizedMCPIDs) {
		t.Fatalf("re-minted token authorization = %v/%v, want %v/%v", decoded.UserGroups, decoded.AuthorizedMCPIDs, original.UserGroups, original.AuthorizedMCPIDs)
	}
}

func TestCompositeComponentAuthReusesValidLoopbackToken(t *testing.T) {
	initial := compositeTestToken(t, compositeTestTokenContext(), time.Now().Add(compositeLoopbackTokenTTL))

	handler, err := compositeComponentAuth{ctx: t.Context(), tokens: fakeCompositeTokens{}}.OAuthHandler(compositeTestAuthContext(t.Context(), "Bearer "+initial), mmmcpconfig.Server{})
	if err != nil || handler == nil {
		t.Fatalf("OAuthHandler() = %v, %v, want a handler", handler, err)
	}
	tokenSource, err := handler.TokenSource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != initial {
		t.Fatal("token source re-minted a token that is not near expiry")
	}
}

func TestCompositeComponentAuthIgnoresOtherAuthorization(t *testing.T) {
	nonComposite := compositeTestTokenContext()
	nonComposite.UserGroups = []string{types.GroupMCP, types.GroupAuthenticated}

	for _, tc := range []struct {
		name          string
		authorization string
	}{
		{
			name: "missing",
		},
		{
			name:          "not bearer",
			authorization: "Basic dXNlcjpwYXNz",
		},
		{
			name:          "undecodable",
			authorization: "Bearer not-a-token",
		},
		{
			name:          "expired",
			authorization: "Bearer " + compositeTestToken(t, compositeTestTokenContext(), time.Now().Add(-time.Second)),
		},
		{
			name:          "not composite",
			authorization: "Bearer " + compositeTestToken(t, nonComposite, time.Now().Add(compositeLoopbackTokenTTL)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := compositeComponentAuth{ctx: t.Context(), tokens: fakeCompositeTokens{}}.OAuthHandler(compositeTestAuthContext(t.Context(), tc.authorization), mmmcpconfig.Server{})
			if err != nil || handler != nil {
				t.Fatalf("OAuthHandler() = %v, %v, want no handler", handler, err)
			}
		})
	}
}

// A pooled component session outlives the frontend request that opened it.
// When its standalone SSE stream reconnects after that request's loopback
// token has expired, the reconnect must still authenticate or the session
// closes and later tool calls fail.
func TestCompositeComponentSessionSurvivesLoopbackTokenExpiry(t *testing.T) {
	tokens := fakeCompositeTokens{}
	server := gomcp.NewServer(&gomcp.Implementation{Name: "component", Version: "test"}, nil)
	server.AddTool(&gomcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		return &gomcp.CallToolResult{}, nil
	})
	mcpHandler := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)

	var (
		mu            sync.Mutex
		streams       int
		latestExpiry  time.Time
		reconnected   = make(chan int, 1)
		frontendToken = func() (string, time.Time) {
			expiresAt := time.Now().Add(2 * time.Second)
			return "Bearer " + compositeTestToken(t, compositeTestTokenContext(), expiresAt), expiresAt.Truncate(time.Second)
		}
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		_, err := tokens.DecodeToken(r.Context(), bearer)
		if r.Method == http.MethodGet {
			mu.Lock()
			streams++
			stream, expiry := streams, latestExpiry
			mu.Unlock()
			if stream == 1 && err == nil {
				// End the first standalone stream once the token of the request
				// that opened the session has expired, forcing a reconnect.
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("retry: 10\n\n"))
				w.(http.Flusher).Flush()
				time.Sleep(time.Until(expiry) + 100*time.Millisecond)
				return
			}
			if stream == 2 {
				status := http.StatusOK
				if err != nil {
					status = http.StatusUnauthorized
				}
				reconnected <- status
			}
		}
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://obot.example/metadata"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()

	composite, err := mmmcp.New(t.Context(), &mmmcpconfig.Config{}, mmmcp.Options{
		OAuth: compositeComponentAuth{ctx: t.Context(), tokens: tokens},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer composite.Close()
	cfg := obotmcp.MMMCPConfig(obotmcp.ServerConfig{
		Runtime:       types.RuntimeVMCP,
		MCPServerName: "vmcpi1test",
		Components: []obotmcp.ComponentServer{
			{
				DisplayName: "component",
				URL:         upstream.URL,
			},
		},
	}, nil)
	// Like the gateway, authenticate each frontend request with a freshly
	// minted loopback token.
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization, expiresAt := frontendToken()
		mu.Lock()
		latestExpiry = expiresAt
		mu.Unlock()
		r.Header.Set("Authorization", authorization)
		composite.HTTPHandler().ServeHTTP(w, r.WithContext(mmmcp.ContextWithConfig(r.Context(), cfg)))
	}))
	defer frontend.Close()

	// Stateful sessions, like the MCP Tester's, pool component sessions.
	session, err := gomcp.NewClient(&gomcp.Implementation{Name: "client", Version: "test"}, nil).Connect(t.Context(), &gomcp.StreamableClientTransport{Endpoint: frontend.URL, DisableStandaloneSSE: true}, &gomcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: "echo"}); err != nil {
		t.Fatal(err)
	}

	select {
	case status := <-reconnected:
		if status != http.StatusOK {
			t.Fatalf("standalone SSE reconnect status = %d, want %d", status, http.StatusOK)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("component standalone SSE stream did not reconnect")
	}
	if _, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: "echo"}); err != nil {
		t.Fatalf("tool call after loopback token expiry: %v", err)
	}
}
