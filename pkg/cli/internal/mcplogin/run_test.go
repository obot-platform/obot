package mcplogin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestLoginRelaysOnlyTheUIAttempt(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprint(denied), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			port := listener.Addr().(*net.TCPAddr).Port
			require.NoError(t, listener.Close())
			redirect := fmt.Sprintf("http://localhost:%d/custom/callback", port)
			var callbacks atomic.Int32
			var serverURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Authorization"), "the CLI must not use an Obot token")
				switch r.URL.Path {
				case "/oauth/mcp/login/attempt":
					require.Equal(t, http.MethodGet, r.Method)
					q := url.Values{"redirect_uri": {redirect}, "state": {"attempt"}}
					require.NoError(t, json.NewEncoder(w).Encode(types.MCPLocalLogin{AuthorizationURL: serverURL + "/provider?" + q.Encode(), RedirectURL: redirect, State: "attempt"}))
				case "/provider":
					http.Redirect(w, r, redirect+"?state=attempt&code=provider-code", http.StatusFound)
				case "/oauth/mcp/callback":
					callbacks.Add(1)
					require.Equal(t, "provider-code", r.URL.Query().Get("code"))
					status := "status=complete"
					if denied {
						status = "error=authentication_failed"
					}
					http.Redirect(w, r, fmt.Sprintf("http://localhost:%d/oauth/obot/callback?state=attempt&%s", port, status), http.StatusFound)
				default:
					t.Errorf("unexpected CLI request (no registration, token exchange, or MCP traffic expected): %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			serverURL = server.URL
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			err = run(ctx, server.URL+"/oauth/mcp/login/attempt", server.Client(), func(authURL string) error {
				// Wrong states and paths cannot complete or consume the attempt.
				for _, target := range []string{redirect + "?state=wrong&code=bad", fmt.Sprintf("http://localhost:%d/unexpected?state=attempt", port)} {
					response, err := http.Get(target)
					require.NoError(t, err)
					require.GreaterOrEqual(t, response.StatusCode, 400)
					require.NoError(t, response.Body.Close())
				}
				response, err := http.Get(authURL)
				if err != nil {
					return err
				}
				return response.Body.Close()
			})
			if denied {
				require.ErrorContains(t, err, "authentication failed")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, int32(1), callbacks.Load())
		})
	}
}

func TestLoginRejectsConnectionURL(t *testing.T) {
	err := run(t.Context(), "https://obot.example/mcp-connect/server", nil, nil)
	require.ErrorContains(t, err, "not an MCP connection URL")
}

func TestLoginRejectsExpiredAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	defer server.Close()
	err := run(t.Context(), server.URL+"/oauth/mcp/login/expired", server.Client(), func(string) error { t.Fatal("must not open browser"); return nil })
	require.ErrorContains(t, err, "retry authentication in Obot")
}

func TestLoginRejectsInvalidMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		redirect string
		state    string
	}{
		{
			name:     "remote callback",
			redirect: "http://remote.example:54321/oauth/callback",
			state:    "attempt",
		},
		{
			name:     "reserved callback",
			redirect: "http://localhost:54321/oauth/obot/callback",
			state:    "attempt",
		},
		{
			name:     "wrong attempt",
			redirect: "http://localhost:54321/oauth/callback",
			state:    "other-attempt",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				q := url.Values{"state": {tc.state}, "redirect_uri": {tc.redirect}}
				require.NoError(t, json.NewEncoder(w).Encode(types.MCPLocalLogin{
					AuthorizationURL: "https://provider.example/authorize?" + q.Encode(),
					RedirectURL:      tc.redirect,
					State:            tc.state,
				}))
			}))
			defer server.Close()
			err := run(t.Context(), server.URL+"/oauth/mcp/login/attempt", server.Client(), func(string) error { t.Fatal("must not open browser"); return nil })
			require.ErrorContains(t, err, "invalid")
		})
	}
}

func TestLoginCancellationClosesListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	redirect := fmt.Sprintf("http://localhost:%d/oauth/callback", port)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		q := url.Values{"state": {"attempt"}, "redirect_uri": {redirect}}
		require.NoError(t, json.NewEncoder(w).Encode(types.MCPLocalLogin{
			AuthorizationURL: "https://provider.example/authorize?" + q.Encode(),
			RedirectURL:      redirect,
			State:            "attempt",
		}))
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err = run(ctx, server.URL+"/oauth/mcp/login/attempt", server.Client(), func(string) error { cancel(); return nil })
	require.ErrorIs(t, err, context.Canceled)
	listener, err = net.Listen("tcp", address)
	require.NoError(t, err, "callback listener must close when login is cancelled")
	require.NoError(t, listener.Close())
}
