package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/auth"
	"k8s.io/apiserver/pkg/authentication/user"
)

// The staged provider is allowed to beat the session's provider only for a browser carrying an
// open verification. This covers the gate that decides that, before any provider lookup happens:
// without the cookie the answer must be "no staged provider" regardless of what is staged, so an
// ordinary request can never be routed to a replacement that is not serving logins yet.
//
// The remaining conditions -- that something is staged, and that the cookie names an open
// verification -- run through the dispatcher, and LoginableAuthProvider is tested against them in
// its own package.
func TestStagedVerificationProviderRequiresTheVerifyCookie(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		cookie string
	}{
		{
			name: "no cookie at all",
			path: "/oauth2/start",
		},
		{
			name:   "some other cookie",
			path:   "/oauth2/start",
			cookie: CurrentAuthProviderCookie,
		},
	}

	// A nil dispatcher makes the assertion sharper than an equality check: reaching a provider
	// lookup at all would panic, so passing proves the gate returned first.
	pm := &Manager{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: tt.cookie, Value: "default/google"})
			}
			if got := pm.stagedVerificationProvider(req); got != "" {
				t.Fatalf("stagedVerificationProvider() = %q, want %q", got, "")
			}
		})
	}
}

// An empty verify cookie is a cookie the browser still sends after it has been cleared, so it must
// be treated as absent rather than as an open verification.
func TestStagedVerificationProviderIgnoresAnEmptyVerifyCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/oauth2/start", nil)
	req.AddCookie(&http.Cookie{Name: auth.AuthProviderVerifyCookie, Value: ""})

	pm := &Manager{}
	if got := pm.stagedVerificationProvider(req); got != "" {
		t.Fatalf("stagedVerificationProvider() = %q, want %q", got, "")
	}
}

// A callback that arrives without the current auth provider cookie belongs to no login that Obot
// started in this browser, so it is restarted from Obot once, and fails only if it comes back
// without the cookie again.
func TestServeHTTPRestartsCallbackWithoutProviderCookie(t *testing.T) {
	tests := []struct {
		name             string
		restarted        bool
		wantCode         int
		wantLocation     string
		wantRestartValue string
	}{
		{
			name:             "first callback restarts the login",
			restarted:        false,
			wantCode:         http.StatusFound,
			wantLocation:     "/oauth2/start?rd=%2F",
			wantRestartValue: "true",
		},
		{
			name:             "callback after a restart fails",
			restarted:        true,
			wantCode:         http.StatusUnauthorized,
			wantLocation:     "",
			wantRestartValue: "",
		},
	}

	// A nil dispatcher proves that neither case reaches a provider lookup.
	pm := &Manager{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/oauth2/callback?code=abc&state=xyz", nil)
			if tt.restarted {
				req.AddCookie(&http.Cookie{Name: loginRestartedCookie, Value: "true"})
			}
			rec := httptest.NewRecorder()

			pm.ServeHTTP(&user.DefaultInfo{}, rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Location"); got != tt.wantLocation {
				t.Fatalf("Location = %q, want %q", got, tt.wantLocation)
			}

			var restartCookie *http.Cookie
			for _, c := range rec.Result().Cookies() {
				if c.Name == loginRestartedCookie {
					restartCookie = c
				}
			}
			if restartCookie == nil {
				t.Fatalf("response did not set the %s cookie", loginRestartedCookie)
			}
			if restartCookie.Value != tt.wantRestartValue {
				t.Fatalf("%s cookie = %q, want %q", loginRestartedCookie, restartCookie.Value, tt.wantRestartValue)
			}
			// The guard only works if the browser sends the cookie back on the callback.
			if restartCookie.Path != "/oauth2/callback" {
				t.Fatalf("%s cookie path = %q, want %q", loginRestartedCookie, restartCookie.Path, "/oauth2/callback")
			}
		})
	}
}
