package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

type failingRegistrySettings struct{}

func (failingRegistrySettings) AnonymousRegistryAccessEnabled(context.Context) (bool, error) {
	return false, errors.New("private database details")
}
func (failingRegistrySettings) SetAnonymousRegistryAccessEnabled(context.Context, bool) error {
	return errors.New("private database details")
}

func TestRegistrySettingsAPI(t *testing.T) {
	h := NewRegistrySettingsHandler(newHandlerTestGateway(t))
	get := func(want bool) {
		t.Helper()
		w := httptest.NewRecorder()
		if err := h.Get(api.Context{Request: httptest.NewRequest(http.MethodGet, "/api/registry-settings", nil), ResponseWriter: w}); err != nil {
			t.Fatal(err)
		}
		var got types.RegistrySettings
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.AnonymousAccessEnabled != want {
			t.Fatalf("response = %s, %v", w.Body, err)
		}
	}
	get(false)
	for _, body := range []string{`{"anonymousAccessEnabled":true}`, `{"anonymousAccessEnabled":false}`, "{}", "null", `{"anonymousAccessEnabled":null}`} {
		if err := h.settings.SetAnonymousRegistryAccessEnabled(t.Context(), true); err != nil {
			t.Fatal(err)
		}
		enabled := body == `{"anonymousAccessEnabled":true}`
		w := httptest.NewRecorder()
		if err := h.Update(api.Context{Request: httptest.NewRequest(http.MethodPut, "/api/registry-settings", strings.NewReader(body)), ResponseWriter: w}); err != nil {
			t.Fatal(err)
		}
		var got types.RegistrySettings
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.AnonymousAccessEnabled != enabled {
			t.Fatalf("response = %s, %v", w.Body, err)
		}
		get(enabled)
	}
	for _, body := range []string{"", `{"anonymousAccessEnabled":"true"}`, `{"anonymousAccessEnabled":1}`, `{"anonymousAccessEnabled":true} {}`} {
		err := h.Update(api.Context{Request: httptest.NewRequest(http.MethodPut, "/api/registry-settings", strings.NewReader(body)), ResponseWriter: httptest.NewRecorder()})
		requireModelProxyHTTPError(t, err, http.StatusBadRequest)
		get(false)
	}
	failed := NewRegistrySettingsHandler(failingRegistrySettings{})
	requireModelProxyHTTPError(t, failed.Get(api.Context{Request: httptest.NewRequest(http.MethodGet, "/api/registry-settings", nil)}), http.StatusServiceUnavailable)
	requireModelProxyHTTPError(t, failed.Update(api.Context{Request: httptest.NewRequest(http.MethodPut, "/api/registry-settings", strings.NewReader(`{"anonymousAccessEnabled":true}`))}), http.StatusServiceUnavailable)
}
