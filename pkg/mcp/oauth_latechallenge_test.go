package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// lateChallengeUpstream is a remote MCP server, served by the MCP SDK, with its own authorization server, in front of
// which a test decides where the server asks for OAuth. It records every request to its MCP endpoint.
type lateChallengeUpstream struct {
	server *httptest.Server
	// challenge is the JSON-RPC method the server challenges: "initialize", "tools/list", or "" for none before a
	// tool is called.
	challenge string
	// wellKnown reports that the server publishes its protected resource metadata at the well-known location for its
	// URL, as one that only challenges tools/call does.
	wellKnown bool
	// toolsList, when set, answers tools/list instead of the server.
	toolsList http.HandlerFunc

	lock     sync.Mutex
	requests []lateChallengeRequest
}

// lateChallengeRequest is one request to the upstream's MCP endpoint.
type lateChallengeRequest struct {
	// method is the JSON-RPC method, or the HTTP method of a request without one.
	method string
	header http.Header
	// status is the status the upstream answered a DELETE with.
	status int
}

// statusRecorder records the status a handler answers with, and passes it to onStatus, if set, before the answer is
// sent, so that a client that has its answer finds the status recorded.
type statusRecorder struct {
	http.ResponseWriter
	status   int
	onStatus func(int)
}

// storedTokenStorage holds a token for the server, as a grant that is authorized again does.
type storedTokenStorage struct {
	recordingTokenStorage
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	if r.onStatus != nil {
		r.onStatus(status)
	}
	r.ResponseWriter.WriteHeader(status)
}

func newLateChallengeUpstream(t *testing.T, challenge string, wellKnown bool, toolsList http.HandlerFunc) *lateChallengeUpstream {
	t.Helper()
	u := &lateChallengeUpstream{challenge: challenge, wellKnown: wellKnown, toolsList: toolsList}
	server := gomcp.NewServer(&gomcp.Implementation{Name: "late", Version: "test"}, nil)
	server.AddTool(&gomcp.Tool{Name: "test", InputSchema: map[string]any{"type": "object"}}, nil)
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
	var base string
	writeJSON := func(w http.ResponseWriter, body any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
	protectedResource := func(w http.ResponseWriter) {
		writeJSON(w, map[string]any{
			"resource":              base + "/mcp",
			"authorization_servers": []string{base},
			"scopes_supported":      []string{"from-metadata"},
		})
	}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			var message struct {
				Method string `json:"method"`
			}
			_ = json.Unmarshal(body, &message)
			request := lateChallengeRequest{method: message.Method, header: r.Header.Clone()}
			if request.method == "" {
				request.method = r.Method
			}
			// Recorded as it arrives, in the order the client sent it: the client sends each request once it has the
			// answer to the one before, but the handler of that one, such as an initialize answered with an event
			// stream, may return after the next one arrives.
			u.lock.Lock()
			index := len(u.requests)
			u.requests = append(u.requests, request)
			u.lock.Unlock()
			switch {
			case request.method == u.challenge:
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+base+`/metadata", scope="tools:read"`)
				http.Error(w, "login required", http.StatusUnauthorized)
			case request.method == "tools/list" && u.toolsList != nil:
				u.toolsList(w, r)
			case r.Method == http.MethodDelete:
				transport.ServeHTTP(&statusRecorder{ResponseWriter: w, onStatus: func(status int) {
					u.lock.Lock()
					u.requests[index].status = status
					u.lock.Unlock()
				}}, r)
			default:
				transport.ServeHTTP(w, r)
			}
		case "/metadata":
			protectedResource(w)
		case "/.well-known/oauth-protected-resource/mcp":
			if !u.wellKnown {
				http.NotFound(w, r)
				return
			}
			protectedResource(w)
		case "/.well-known/oauth-authorization-server":
			// Published whatever the server does, so that only protected resource metadata can show that it uses OAuth.
			writeJSON(w, map[string]any{
				"issuer":                   base,
				"response_types_supported": []string{"code"},
				"authorization_endpoint":   base + "/authorize",
				"token_endpoint":           base + "/token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	base = u.server.URL
	t.Cleanup(u.server.Close)
	return u
}

func (u *lateChallengeUpstream) mcpURL() string {
	return u.server.URL + "/mcp"
}

func (u *lateChallengeUpstream) recorded() []lateChallengeRequest {
	u.lock.Lock()
	defer u.lock.Unlock()
	return append([]lateChallengeRequest(nil), u.requests...)
}

func (*storedTokenStorage) GetTokenConfig(context.Context) (*oauth2.Config, *oauth2.Token, error) {
	return nil, &oauth2.Token{AccessToken: "stored-access-token"}, nil
}

// start starts an authorization of the upstream as a profile grant does, for a server whose configuration has a header
// and a stored token, neither of which any probe may send.
func (u *lateChallengeUpstream) start(ctx context.Context, staticOAuth bool) (string, error) {
	sm := &SessionManager{remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true}}
	return sm.StartOAuthAuthorization(ctx, ServerConfig{
		URL:           u.mcpURL(),
		MCPServerName: "late",
		Headers:       []string{"X-Configured-Key=configured-secret"},
	}, ClientOption{
		CallbackHandler:  &oauthAuthorizeCallbackHandler{},
		ClientLookup:     &oauthTestClientCredLookup{clientID: "static-client"},
		TokenStorage:     &storedTokenStorage{},
		OAuthClientName:  "Obot",
		OAuthRedirectURL: "https://obot.example/oauth/mcp/callback",
	}, staticOAuth)
}

// requireProbesUnauthenticated requires that every request the upstream received carried only the headers of the
// transport and the MCP session, never a configured header or a token, and that every session a probe started was
// ended.
func requireProbesUnauthenticated(t *testing.T, requests []lateChallengeRequest) {
	t.Helper()
	allowed := []string{"Accept", "Accept-Encoding", "Content-Length", "Content-Type", "Mcp-Protocol-Version", "Mcp-Session-Id", "User-Agent"}
	var session string
	for i, request := range requests {
		for name := range request.header {
			require.Contains(t, allowed, name, "request %d (%s) carried header %s", i, request.method, name)
		}
		if i == 0 {
			require.Equal(t, "initialize", request.method)
			require.Empty(t, request.header.Get("Mcp-Session-Id"))
			continue
		}
		// Every later request is in the session initialize started, with the protocol version it agreed on.
		if session == "" {
			session = request.header.Get("Mcp-Session-Id")
			require.NotEmpty(t, session, "request %d (%s) is in no session", i, request.method)
		}
		require.Equal(t, session, request.header.Get("Mcp-Session-Id"), "request %d (%s)", i, request.method)
		require.Equal(t, "2025-06-18", request.header.Get("Mcp-Protocol-Version"), "request %d (%s)", i, request.method)
	}
	if session != "" {
		last := requests[len(requests)-1]
		require.Equal(t, http.MethodDelete, last.method, "the probe's session was not ended")
		require.Equal(t, http.StatusNoContent, last.status, "the probe's session was not found to end")
	}
}

// TestStartOAuthAuthorizationFindsLateChallenges checks where the start of a profile grant learns how a server asks for
// OAuth: the initialize challenge as before; the challenge to tools/list, with its scope, for a server that lets anyone
// initialize; the protected resource metadata a server publishes when it challenges neither, as one that only
// challenges tools/call does; and nowhere for a server that does none of these, which is refused as not requiring
// OAuth, as before. Each probe is unauthenticated and ends the session it started.
func TestStartOAuthAuthorizationFindsLateChallenges(t *testing.T) {
	initialized := []string{"initialize", "notifications/initialized", "tools/list", http.MethodDelete}
	unavailable := func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}
	for _, tt := range []struct {
		name        string
		challenge   string
		wellKnown   bool
		toolsList   http.HandlerFunc
		staticOAuth bool
		// wantScope is the scope the authorization asks for, when it starts.
		wantScope string
		// wantNotRequired is whether the server is refused as not requiring OAuth.
		wantNotRequired bool
		// wantErr is part of the error that fails the start otherwise.
		wantErr string
		// wantMethods are the requests the server's MCP endpoint receives, in order.
		wantMethods []string
	}{
		{
			name:        "initialize challenge",
			challenge:   "initialize",
			wantScope:   "tools:read",
			wantMethods: []string{"initialize"},
		},
		{
			name:        "tools/list challenge",
			challenge:   "tools/list",
			wantScope:   "tools:read",
			wantMethods: initialized,
		},
		{
			name:        "tools/call challenge with published metadata",
			wellKnown:   true,
			wantScope:   "from-metadata",
			wantMethods: initialized,
		},
		{
			name:            "no challenge and no metadata",
			wantNotRequired: true,
			wantMethods:     initialized,
		},
		{
			// Static OAuth starts from the well-known locations whether or not the server challenges, as before.
			name:        "no challenge with static OAuth",
			staticOAuth: true,
			wantMethods: initialized,
		},
		{
			// Whether the server uses OAuth is unknown, so it is not called one that does not.
			name:        "tools/list failure and no metadata",
			toolsList:   unavailable,
			wantErr:     "tools/list was answered with status 503",
			wantMethods: initialized,
		},
		{
			name:        "tools/list failure with published metadata",
			toolsList:   unavailable,
			wellKnown:   true,
			wantScope:   "from-metadata",
			wantMethods: initialized,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := newLateChallengeUpstream(t, tt.challenge, tt.wellKnown, tt.toolsList)
			authURL, err := upstream.start(t.Context(), tt.staticOAuth)

			requests := upstream.recorded()
			methods := make([]string, 0, len(requests))
			for _, request := range requests {
				methods = append(methods, request.method)
			}
			require.Equal(t, tt.wantMethods, methods)
			requireProbesUnauthenticated(t, requests)

			switch {
			case tt.wantNotRequired:
				require.ErrorIs(t, err, ErrOAuthNotRequired)
				require.Empty(t, authURL)
			case tt.wantErr != "":
				require.ErrorContains(t, err, tt.wantErr)
				require.NotErrorIs(t, err, ErrOAuthNotRequired)
			default:
				require.NoError(t, err)
				parsed, err := url.Parse(authURL)
				require.NoError(t, err)
				require.Equal(t, upstream.server.URL+"/authorize", parsed.Scheme+"://"+parsed.Host+parsed.Path)
				require.Equal(t, "static-client", parsed.Query().Get("client_id"))
				require.Equal(t, tt.wantScope, parsed.Query().Get("scope"))
			}
		})
	}
}

// TestStartOAuthAuthorizationBoundsAServerThatHangs checks that a server that never answers tools/list fails the start
// once the time it has runs out, rather than holding the administrator's request, without falling back to its metadata,
// and that the session the probe started is still ended, though the request that ran out of time is done.
func TestStartOAuthAuthorizationBoundsAServerThatHangs(t *testing.T) {
	hang := func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}
	requireEnded := func(t *testing.T, upstream *lateChallengeUpstream) {
		t.Helper()
		// The DELETE is sent once the hung tools/list ran out of time.
		require.Eventually(t, func() bool {
			requests := upstream.recorded()
			return len(requests) == 4 && requests[3].status != 0
		}, 5*time.Second, 10*time.Millisecond)
		var methods []string
		for _, request := range upstream.recorded() {
			methods = append(methods, request.method)
			if request.method == http.MethodDelete {
				require.Equal(t, http.StatusNoContent, request.status, "the probe's session was not ended")
			}
		}
		require.ElementsMatch(t, []string{"initialize", "notifications/initialized", "tools/list", http.MethodDelete}, methods)
	}

	t.Run("the request's deadline", func(t *testing.T) {
		upstream := newLateChallengeUpstream(t, "", true, hang)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		started := time.Now()
		_, err := upstream.start(ctx, false)
		require.ErrorContains(t, err, "tools/list")
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(started), 10*time.Second)
		requireEnded(t, upstream)
	})

	t.Run("the client's timeout", func(t *testing.T) {
		upstream := newLateChallengeUpstream(t, "", true, hang)
		started := time.Now()
		_, err := probeOAuthChallenge(t.Context(), &http.Client{Timeout: time.Second}, upstream.mcpURL())
		require.ErrorContains(t, err, "tools/list")
		var timeout interface{ Timeout() bool }
		require.True(t, errors.As(err, &timeout) && timeout.Timeout(), "error = %v, want a timeout", err)
		require.Less(t, time.Since(started), 10*time.Second)
		requireEnded(t, upstream)
	})
}

// TestNegotiatedProtocolVersion checks how the protocol version an initialize answer agreed on is read, from a JSON or
// an event-stream body, and that one that cannot be sent back in a header is not.
func TestNegotiatedProtocolVersion(t *testing.T) {
	for _, tt := range []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			name:        "JSON",
			contentType: "application/json",
			body:        `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`,
			want:        "2025-03-26",
		},
		{
			name:        "event stream",
			contentType: "text/event-stream; charset=utf-8",
			body:        "event: message\nid: 1\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2025-06-18\"}}\n\n",
			want:        "2025-06-18",
		},
		{
			name:        "no version",
			contentType: "application/json",
			body:        `{"jsonrpc":"2.0","id":1,"result":{}}`,
		},
		{
			name:        "not a header value",
			contentType: "application/json",
			body:        `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18\r\nAuthorization: Bearer x"}}`,
		},
		{
			name:        "not JSON",
			contentType: "text/html",
			body:        "<html></html>",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: http.Header{"Content-Type": {tt.contentType}},
				Body:   io.NopCloser(strings.NewReader(tt.body)),
			}
			require.Equal(t, tt.want, negotiatedProtocolVersion(resp))
		})
	}
}
