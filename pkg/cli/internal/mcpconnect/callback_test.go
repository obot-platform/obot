package mcpconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/require"
)

func TestCallbackRelay(t *testing.T) {
	gateway, err := gatewayBaseURL("https://obot.example/prefix/mcp-connect/vmcpi1")
	require.NoError(t, err)
	paths, err := allowedCallbackPaths([]string{"/oauth/callback", "/custom/callback"})
	require.NoError(t, err)
	h := &callbackHandler{gateway: gateway, providerPaths: paths, result: make(chan callbackResult, 1)}
	for _, path := range []string{"/oauth/callback", "/custom/callback"} {
		r := httptest.NewRequest(http.MethodGet, path+"?code=upstream&state=upstream-state&redirect_uri=https%3A%2F%2Fevil.example", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		require.Equal(t, http.StatusFound, w.Code)
		require.Equal(t, "https://obot.example/prefix/oauth/mcp/callback?"+r.URL.RawQuery, w.Header().Get("Location"))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oauth/callback?error=access_denied&state=state", nil))
	require.Equal(t, http.StatusFound, w.Code)
	require.Contains(t, w.Header().Get("Location"), "error=access_denied")
}

func TestCallbackStateAndErrors(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "denied"}[denied], func(t *testing.T) {
			opened := make(chan struct{})
			h := &callbackHandler{openBrowser: func(string) error { close(opened); return nil }}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			done := make(chan callbackResult, 1)
			go func() {
				result, err := h.fetch(ctx, &auth.AuthorizationArgs{URL: "https://obot.example/authorize?state=expected"})
				done <- callbackResult{result: result, err: err}
			}()
			<-opened
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, obotCallbackPath+"?code=bad&state=wrong", nil))
			require.Equal(t, http.StatusBadRequest, w.Code)
			values := url.Values{"code": {"obot-code"}, "state": {"expected"}}
			if denied {
				values.Set("error", "access_denied")
			}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, obotCallbackPath+"?"+values.Encode(), nil))
			result := <-done
			if denied {
				require.ErrorContains(t, result.err, "access_denied")
			} else {
				require.NoError(t, result.err)
				require.Equal(t, "obot-code", result.result.Code)
			}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, obotCallbackPath+"?"+values.Encode(), nil))
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestGatewayBaseURL(t *testing.T) {
	for _, value := range []string{"file:///mcp-connect/a", "https:///mcp-connect/a", "https://user:secret@obot.example/mcp-connect/a", "https://obot.example/mcp-connect/", "https://obot.example/mcp-connect/a?redirect=evil", "https://obot.example/mcp-connect/a/b"} {
		_, err := gatewayBaseURL(value)
		require.Error(t, err, value)
	}
}

func TestAllowedCallbackPaths(t *testing.T) {
	defaults, err := allowedCallbackPaths(nil)
	require.NoError(t, err)
	require.Equal(t, map[string]struct{}{"/oauth/callback": {}}, defaults)
	custom, err := allowedCallbackPaths([]string{"/custom/callback", "/second%20callback", "/custom/callback"})
	require.NoError(t, err)
	require.Equal(t, map[string]struct{}{"/custom/callback": {}, "/second callback": {}}, custom)
	for _, path := range []string{"", "relative", "https://evil.example/callback", "//evil.example/callback", "/callback?x=1", "/callback#fragment", "/oauth/obot/callback", "/oauth/%6fbot/callback", "/a/../callback"} {
		_, err := allowedCallbackPaths([]string{path})
		require.Error(t, err, path)
	}
}

func TestProviderCallbacksRequireAllowedPathAndActiveLogin(t *testing.T) {
	gateway, err := gatewayBaseURL("https://obot.example/mcp-connect/test")
	require.NoError(t, err)
	paths, err := allowedCallbackPaths([]string{"/custom/callback", "/second/callback"})
	require.NoError(t, err)
	opened := make(chan struct{})
	h := &callbackHandler{gateway: gateway, providerPaths: paths, openBrowser: func(string) error { close(opened); return nil }}
	check := func(path string, status int) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+"?state=provider-state&code=provider-code", nil))
		require.Equal(t, status, w.Code, path)
		if status == http.StatusFound {
			require.Equal(t, "https://obot.example/oauth/mcp/callback?state=provider-state&code=provider-code", w.Header().Get("Location"))
		} else {
			require.Empty(t, w.Header().Get("Location"))
		}
	}
	check("/custom/callback", http.StatusBadRequest)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := h.fetch(ctx, &auth.AuthorizationArgs{URL: "https://obot.example/authorize?state=cli-state"})
		done <- err
	}()
	<-opened
	check("/oauth/callback", http.StatusNotFound)
	check("/unexpected", http.StatusNotFound)
	check("/custom/callback/extra", http.StatusNotFound)
	// Provider state is deliberately different from the CLI state; Obot validates it.
	check("/custom/callback", http.StatusFound)
	check("/second/callback", http.StatusFound)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, obotCallbackPath+"?state=cli-state&code=obot-code", nil))
	require.NoError(t, <-done)
	check("/custom/callback", http.StatusBadRequest)
}

func TestProviderCallbacksStopAfterCanceledLogin(t *testing.T) {
	paths, err := allowedCallbackPaths(nil)
	require.NoError(t, err)
	opened := make(chan struct{})
	h := &callbackHandler{providerPaths: paths, openBrowser: func(string) error { close(opened); return nil }}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := h.fetch(ctx, &auth.AuthorizationArgs{URL: "https://obot.example/authorize?state=cli-state"})
		done <- err
	}()
	<-opened
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oauth/callback?state=provider-state&code=code", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
}
