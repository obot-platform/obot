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
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcptester"
)

type failingModelProxySettings struct{}

func (failingModelProxySettings) ModelProxyEnabled(context.Context) (bool, error) {
	return false, errors.New("private database details")
}

func (failingModelProxySettings) SetModelProxyEnabled(context.Context, bool) error {
	return errors.New("private database details")
}

func TestModelProxySettingsAPI(t *testing.T) {
	for _, configuredURL := range []string{"", "https://model-service.obot.ai", "https://proxy.example/prefix/", "https://proxy.example/prefix/v1/responses"} {
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

			// Extra fields and whitespace use the shared API decoding behavior.
			for _, body := range []string{`{"enabled":true,"url":"https://ignored.example"}`, strings.Repeat(" ", 1025) + `{"enabled":true}`} {
				if err := handler.Update(api.Context{Request: httptest.NewRequest(http.MethodPut, "/api/model-proxy", strings.NewReader(body)), ResponseWriter: httptest.NewRecorder()}); err != nil {
					t.Fatal(err)
				}
				get(true)
			}

			for _, body := range []string{"", "null", "{}", `{"enabled":null}`, `{"enabled":"true"}`, `{"enabled":false} {}`} {
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
		if r.Header.Get("X-Forwarded-For") != "192.0.2.10, 2001:db8::1" || r.Header.Get("X-Real-IP") != "192.0.2.10" {
			t.Error("IP headers were not forwarded")
		}
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
		r.Header.Set("X-Forwarded-For", "192.0.2.10, 2001:db8::1")
		r.Header.Set("X-Real-IP", "192.0.2.10")

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

		if !strings.Contains(w.Body.String(), `"resetAt"`) {
			t.Fatal("missing usage reset field")
		}
	}

	if len(keys) != 2 || keys[0] != "Bearer key-one" || keys[1] != "Bearer key-two" {
		t.Fatalf("license rotation not reflected: %v", keys)
	}

	license.key = ""
	_, err = call()
	requireModelProxyHTTPError(t, err, http.StatusForbidden)

	license.err = errors.New("private database details")
	_, err = call()
	requireModelProxyHTTPError(t, err, http.StatusServiceUnavailable)
	license.err = nil

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
