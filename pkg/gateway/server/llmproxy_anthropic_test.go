package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
)

func TestAnthropicFilesProxy(t *testing.T) {
	var received *http.Request
	var body string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		received, body = r, string(data)
		w.Header().Set("Set-Cookie", "session=anthropic")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file_1"}`))
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := &anthropicProxy{upstream: *target, transport: upstream.Client().Transport}

	serve := func(method, path, rest string, header http.Header, body string) *httptest.ResponseRecorder {
		t.Helper()
		received = nil
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header = header
		if rest != "" {
			req.SetPathValue("path", rest)
		}
		rec := httptest.NewRecorder()
		require.NoError(t, proxy.files(api.Context{ResponseWriter: rec, Request: req}))
		return rec
	}

	for _, tc := range []struct {
		name       string
		method     string
		path       string
		rest       string
		credential string
		wantPath   string
	}{
		{
			name:       "upload with OAuth token",
			method:     http.MethodPost,
			path:       "/api/llm-proxy/v1/files?beta=true",
			credential: "Authorization",
			wantPath:   "/v1/files?beta=true",
		},
		{
			name:       "download with API key",
			method:     http.MethodGet,
			path:       "/api/llm-proxy/v1/files/file_1/content",
			rest:       "file_1/content",
			credential: "X-Api-Key",
			wantPath:   "/v1/files/file_1/content",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := http.Header{
				"Anthropic-Version": {"2023-06-01"},
				"Anthropic-Beta":    {"files-api-2025-04-14"},
				"Content-Type":      {"text/plain"},
				"Cookie":            {"obot_access_token=secret"},
				"X-Obot-Api-Key":    {"ok1-1-2-secret"},
				"X-Forwarded-For":   {"10.0.0.1"},
			}
			header.Set(tc.credential, "Bearer sk-ant-oat01-caller")
			rec := serve(tc.method, tc.path, tc.rest, header, "file contents")
			require.Equal(t, http.StatusOK, rec.Code)
			require.JSONEq(t, `{"id":"file_1"}`, rec.Body.String())
			require.Empty(t, rec.Header().Values("Set-Cookie"))

			require.NotNil(t, received)
			require.Equal(t, tc.method, received.Method)
			require.Equal(t, tc.wantPath, received.URL.RequestURI())
			require.Equal(t, "Bearer sk-ant-oat01-caller", received.Header.Get(tc.credential))
			require.Equal(t, "2023-06-01", received.Header.Get("Anthropic-Version"))
			require.Equal(t, "files-api-2025-04-14", received.Header.Get("Anthropic-Beta"))
			for _, name := range []string{"Cookie", "X-Obot-Api-Key", "X-Forwarded-For"} {
				require.Empty(t, received.Header.Get(name), name)
			}
			if tc.method == http.MethodPost {
				require.Equal(t, "file contents", body)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		rest   string
		header http.Header
		want   int
	}{
		{
			name:   "no Anthropic credential",
			header: http.Header{"X-Obot-Api-Key": {"ok1-1-2-secret"}},
			want:   http.StatusUnauthorized,
		},
		{
			name:   "Obot API key as bearer token",
			header: http.Header{"Authorization": {"bearer ok1-1-2-secret"}},
			want:   http.StatusUnauthorized,
		},
		{
			name:   "Obot JWT as bearer token",
			header: http.Header{"Authorization": {"Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl"}},
			want:   http.StatusUnauthorized,
		},
		{
			name:   "Obot token as bearer token",
			header: http.Header{"Authorization": {"Bearer 2f8a4c1e9b7d6a5f3c2e1d0b9a8f7e6d"}},
			want:   http.StatusUnauthorized,
		},
		{
			name:   "Obot API key beside an Anthropic credential",
			header: http.Header{"Authorization": {"Bearer sk-ant-oat01-caller"}, "X-Api-Key": {"ok1-1-2-secret"}},
			want:   http.StatusUnauthorized,
		},
		{
			name:   "path outside the Files API",
			rest:   "../messages",
			header: http.Header{"Authorization": {"Bearer sk-ant-oat01-caller"}},
			want:   http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.header.Set("Anthropic-Version", "2023-06-01")
			rec := serve(http.MethodGet, "/api/llm-proxy/v1/files", tc.rest, tc.header, "")
			require.Equal(t, tc.want, rec.Code)
			require.Nil(t, received, "the request must not reach Anthropic")
			var errorBody struct {
				Type string `json:"type"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errorBody))
			require.Equal(t, "error", errorBody.Type, "Anthropic clients get Anthropic errors")
		})
	}
}

// Only model requests belong in the LLM audit log. The other Anthropic API requests the LLM
// proxy forwards are left out; the egress audit log records Claude sessions' requests.
func TestAnthropicProxyRequestsAreNotLLMAudited(t *testing.T) {
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://" + filepath.Join(t.TempDir(), "audit.db")})
	require.NoError(t, err)
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	// The LLM audit log is on, with a short persistence interval so the test can wait for it.
	gatewayClient := client.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 0, 0, 0, true)
	t.Cleanup(func() { _ = gatewayClient.Close() })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file_1","type":"file"}`))
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := &anthropicProxy{upstream: *target, transport: upstream.Client().Transport}

	serve := func(handler api.HandlerFunc, method, path, rest string, header http.Header) int {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header = header
		if rest != "" {
			req.SetPathValue("path", rest)
		}
		rec := httptest.NewRecorder()
		require.NoError(t, handler(api.Context{ResponseWriter: rec, Request: req, GatewayClient: gatewayClient}))
		return rec.Code
	}
	oauth := http.Header{"Authorization": {"Bearer sk-ant-oat01-caller"}, "Anthropic-Version": {"2023-06-01"}}
	require.Equal(t, http.StatusOK, serve(proxy.files, http.MethodGet, "/api/llm-proxy/v1/files/file_1", "file_1", oauth.Clone()))
	require.Equal(t, http.StatusUnauthorized, serve(proxy.files, http.MethodGet, "/api/llm-proxy/v1/files", "", http.Header{"Anthropic-Version": {"2023-06-01"}}))
	require.Equal(t, http.StatusOK, serve(proxy.agentProxyCACert, http.MethodGet, "/api/llm-proxy/v1/code/agent-proxy/ca-cert", "", http.Header{}))

	// Entries are written in the order they are logged, so once this one is stored, any the
	// requests above logged would be stored too.
	gatewayClient.LogLLMAuditEntry(types.LLMAuditLog{ID: "marker", CreatedAt: time.Now(), RequestPath: "/api/llm-proxy/v1/messages"}, nil)
	var logs []types.LLMAuditLog
	require.Eventually(t, func() bool {
		logs, _, err = gatewayClient.GetLLMAuditLogs(t.Context(), client.LLMAuditLogOptions{})
		require.NoError(t, err)
		return len(logs) > 0
	}, 5*time.Second, 20*time.Millisecond)
	require.Len(t, logs, 1)
	require.Equal(t, "marker", logs[0].ID)
}

func TestAnthropicAgentProxyCACert(t *testing.T) {
	var received *http.Request
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write([]byte("-----BEGIN CERTIFICATE-----\n"))
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	proxy := &anthropicProxy{upstream: *target, transport: upstream.Client().Transport}

	req := httptest.NewRequest(http.MethodGet, "/api/llm-proxy/v1/code/agent-proxy/ca-cert", nil)
	req.Header = http.Header{
		"X-Obot-Api-Key": {"ok1-1-2-secret"},
		"Authorization":  {"Bearer sk-ant-oat01-caller"},
	}
	rec := httptest.NewRecorder()
	require.NoError(t, proxy.agentProxyCACert(api.Context{ResponseWriter: rec, Request: req}))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "-----BEGIN CERTIFICATE-----\n", rec.Body.String())
	require.NotNil(t, received)
	require.Equal(t, "/v1/code/agent-proxy/ca-cert", received.URL.RequestURI())
	// The certificate is public: no credential, Obot's or the caller's, leaves Obot.
	for _, name := range []string{"Authorization", "X-Api-Key", "X-Obot-Api-Key"} {
		require.Empty(t, received.Header.Get(name), name)
	}
}
