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
	h := &callbackHandler{gateway: gateway}
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
