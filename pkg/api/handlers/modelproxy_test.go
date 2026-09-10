package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcptester"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

type failingModelProxySettings struct{}

func (failingModelProxySettings) ModelProxyEnabled(context.Context) (bool, error) {
	return false, errors.New("private database details")
}

func (failingModelProxySettings) SetModelProxyEnabled(context.Context, bool) error {
	return errors.New("private database details")
}

func TestModelProxySettingsAPI(t *testing.T) {
	for _, configuredURL := range []string{"", mcptester.DefaultModelProxyURL, "https://proxy.example/prefix/", "https://proxy.example/prefix/v1/responses"} {
		t.Run(configuredURL, func(t *testing.T) {
			store := newHandlerTestGateway(t)
			handler := NewModelProxyHandler(store, configuredURL, nil, nil)

			get := func(want bool) {
				t.Helper()

				w := httptest.NewRecorder()
				if err := handler.Get(api.Context{Request: httptest.NewRequest(http.MethodGet, "/api/model-proxy", nil), ResponseWriter: w}); err != nil {
					t.Fatal(err)
				}

				var result map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result) != 2 || result["enabled"] != want || result["url"] != configuredURL {
					t.Fatalf("settings response = %s, %v", w.Body, err)
				}

				if w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("settings response was cacheable")
				}
			}

			get(true)

			for _, enabled := range []bool{false, true, true} {
				w := httptest.NewRecorder()
				if err := handler.Update(api.Context{Request: httptest.NewRequest(http.MethodPut, "/api/model-proxy", strings.NewReader(`{"enabled":`+strconv.FormatBool(enabled)+`}`)), ResponseWriter: w}); err != nil {
					t.Fatal(err)
				}

				var response types.ModelProxySettings
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Enabled != enabled || response.URL != configuredURL {
					t.Fatalf("update response = %s, %v", w.Body, err)
				}

				get(enabled)
			}

			for _, body := range []string{"", "null", "{}", `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":true,"url":"https://evil.example"}`, `{"enabled":false} {}`, strings.Repeat(" ", 1025) + `{"enabled":false}`} {
				err := handler.Update(api.Context{Request: httptest.NewRequest(http.MethodPut, "/api/model-proxy", strings.NewReader(body)), ResponseWriter: httptest.NewRecorder()})
				requireModelProxyHTTPError(t, err, http.StatusBadRequest)
				get(true)
			}
		})
	}
}

func requireModelProxyHTTPError(t *testing.T, err error, status int) {
	t.Helper()

	failure, ok := errors.AsType[*types.ErrHTTP](err)
	if !ok || failure.Code != status || strings.Contains(failure.Message, "private") {
		t.Fatalf("error = %v, want safe HTTP %d", err, status)
	}
}

func TestModelProxyUsageAPI(t *testing.T) {
	var keys []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Authorization"))
		if r.Method != http.MethodGet || r.URL.Path != "/prefix/v1/usage" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Obot-Machine-Fingerprint") != "persisted-machine" || r.Header.Get("X-Obot-MCP-URL") != "" {
			t.Errorf("unexpected usage request %s %s %v", r.Method, r.URL, r.Header)
		}

		_, _ = io.WriteString(w, `{"input":{"used":100,"max":0},"output":{"used":5,"max":10},"reset_at":"2026-09-11T00:00:00Z"}`)
	}))
	defer upstream.Close()

	endpoint, err := mcptester.ParseModelProxyURL(upstream.URL+"/prefix", true)
	if err != nil {
		t.Fatal(err)
	}

	store := newHandlerTestGateway(t)
	license := &fakeTesterLicense{key: "key-one"}
	h := NewModelProxyHandler(store, upstream.URL+"/prefix", endpoint, license)

	call := func() (*httptest.ResponseRecorder, error) {
		t.Helper()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/model-proxy/usage", nil)
		r.Header.Set("Authorization", "Bearer private-browser-token")
		r.Header.Set("Cookie", "private-browser-cookie")
		r.Header.Set("X-Obot-MCP-URL", "https://private.example")

		return w, h.Usage(api.Context{Request: r, ResponseWriter: w})
	}

	for _, key := range []string{"key-one", "key-two"} {
		license.key = key
		w, err := call()
		if err != nil {
			t.Fatal(err)
		}

		var result types.ModelProxyUsage
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Input.Used != 100 || result.Input.Max != 0 || result.Output.Used != 5 || result.ResetAt.IsZero() {
			t.Fatalf("usage response = %s, %v", w.Body, err)
		}

		if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"resetAt"`) {
			t.Fatal("incorrect usage response headers or reset field")
		}
	}

	if len(keys) != 2 || keys[0] != "Bearer key-one" || keys[1] != "Bearer key-two" {
		t.Fatalf("license rotation not reflected: %v", keys)
	}

	license.key = ""
	_, err = call()
	requireModelProxyHTTPError(t, err, http.StatusServiceUnavailable)

	license.key = "key-three"
	if err := store.SetModelProxyEnabled(t.Context(), false); err != nil {
		t.Fatal(err)
	}

	_, err = call()
	requireModelProxyHTTPError(t, err, http.StatusConflict)

	if err := store.SetModelProxyEnabled(t.Context(), true); err != nil {
		t.Fatal(err)
	}

	h.responsesURL = nil
	_, err = call()
	requireModelProxyHTTPError(t, err, http.StatusConflict)

	h.responsesURL = endpoint
	h.settings = failingModelProxySettings{}
	_, err = call()
	requireModelProxyHTTPError(t, err, http.StatusServiceUnavailable)
	if len(keys) != 2 {
		t.Fatal("disabled or unavailable usage sent an outbound request")
	}
}

func TestTesterRechecksModelProxySwitch(t *testing.T) {
	store := newHandlerTestGateway(t)
	providers := &fakeTesterProviders{}
	endpoint, err := mcptester.ParseModelProxyURL(mcptester.DefaultModelProxyURL, false)
	if err != nil {
		t.Fatal(err)
	}

	h := &MCPTesterHandler{fallback: MCPTesterFallbackOptions{URL: endpoint, Providers: providers, Settings: store}}

	for _, enabled := range []bool{true, false, true} {
		if err := store.SetModelProxyEnabled(t.Context(), enabled); err != nil {
			t.Fatal(err)
		}

		if got, err := h.fallbackEnabled(t.Context()); err != nil || got != enabled {
			t.Fatalf("fallback = %v, %v, want %v", got, err, enabled)
		}
	}

	h.fallback.Settings = failingModelProxySettings{}
	if enabled, err := h.fallbackEnabled(t.Context()); err == nil || enabled {
		t.Fatal("settings failure did not prevent fallback")
	}

	providers.configured = true
	if enabled, err := h.fallbackEnabled(t.Context()); err != nil || enabled {
		t.Fatal("settings failure affected configured provider")
	}
}

func TestDisablingModelProxyAllowsActiveStreamToFinish(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	var calls atomic.Int32
	var canceled atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		if calls.Add(1) == 1 {
			close(started)

			select {
			case <-release:
			case <-r.Context().Done():
				canceled.Store(true)
				return
			}
		}

		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	defer upstream.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newFallbackTestHandler(t, &fakeTesterProviders{}, &fakeTesterLicense{key: "key"}, upstream)
	store := newHandlerTestGateway(t)
	h.fallback.Settings = store
	r := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/ms1tester/tester/chat", strings.NewReader(fallbackChatBody)).WithContext(ctx)
	r.SetPathValue("mcp_server_id", "ms1tester")
	w := httptest.NewRecorder()
	done := make(chan error, 1)

	go func() {
		done <- h.Chat(api.Context{Request: r, ResponseWriter: w, User: &kuser.DefaultInfo{UID: "user-1"}})
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not start")
	}

	if err := store.SetModelProxyEnabled(t.Context(), false); err != nil {
		t.Fatal(err)
	}

	close(release)

	select {
	case err := <-done:
		if err != nil || canceled.Load() || !strings.Contains(w.Body.String(), `"type":"completion"`) {
			t.Fatalf("active stream did not finish: %s, %v", w.Body, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not finish")
	}

	h.modelResolver = fakeMCPTesterModelAccess{allowed: false}
	assertMCPTesterError(t, runMCPTesterChat(t, h, "user-1", fallbackChatBody), http.StatusForbidden, types.MCPTesterErrorModelUnavailable)
	if calls.Load() != 1 {
		t.Fatal("disabled continuation contacted proxy")
	}

	if err := store.SetModelProxyEnabled(t.Context(), true); err != nil {
		t.Fatal(err)
	}

	if response := runMCPTesterChat(t, h, "user-1", fallbackChatBody); response.Code != http.StatusOK || calls.Load() != 2 {
		t.Fatalf("reenabled continuation failed: %s", response.Body)
	}
}
