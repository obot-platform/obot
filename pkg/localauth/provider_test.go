package localauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoginRejectsCrossOriginRequests(t *testing.T) {
	provider, err := New(nil, "https://obot.example.ai", "https://ui.example.ai")
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		host     string
		headers  map[string]string
		rejected bool
	}{
		{
			name: "same-origin login",
			headers: map[string]string{
				"Sec-Fetch-Site": "same-origin",
				"Origin":         "https://obot.example.ai",
			},
		},
		{
			name: "login form served from the UI hostname",
			headers: map[string]string{
				"Sec-Fetch-Site": "same-site",
				"Origin":         "https://ui.example.ai",
			},
		},
		{
			name: "reverse proxy rewrites the host",
			host: "127.0.0.1:8080",
			headers: map[string]string{
				"Origin": "https://obot.example.ai",
			},
		},
		{
			name: "client without browser headers",
		},
		{
			name: "cross-site login",
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "https://evil.example",
			},
			rejected: true,
		},
		{
			name: "same-origin login with only a Referer",
			headers: map[string]string{
				"Referer": "https://obot.example.ai/login/local",
			},
		},
		{
			name: "UI login with only a Referer",
			headers: map[string]string{
				"Referer": "https://ui.example.ai/login/local",
			},
		},
		{
			name: "cross-site login with only a Referer",
			headers: map[string]string{
				"Referer": "https://evil.example/page",
			},
			rejected: true,
		},
		{
			name: "invalid Referer",
			headers: map[string]string{
				"Referer": "not a url",
			},
			rejected: true,
		},
		{
			name: "same-site tenant login without fetch metadata",
			headers: map[string]string{
				"Origin": "https://tenant-b.example.ai",
			},
			rejected: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Empty credentials fail after the cross-origin check without reaching the database.
			req := httptest.NewRequest(http.MethodPost, "https://obot.example.ai/oauth2/start", strings.NewReader(url.Values{"rd": {"/"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.host != "" {
				req.Host = tc.host
			}
			for name, value := range tc.headers {
				req.Header.Set(name, value)
			}
			response := httptest.NewRecorder()
			provider.handler().ServeHTTP(response, req)

			if tc.rejected {
				require.Equal(t, http.StatusForbidden, response.Code)
			} else {
				require.Equal(t, http.StatusFound, response.Code)
				require.Contains(t, response.Header().Get("Location"), LoginPath+"?")
			}
		})
	}
}
