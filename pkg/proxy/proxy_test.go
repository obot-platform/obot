package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestServeHTTPSignOutFallbackRedirect(t *testing.T) {
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.AuthProvider{}, "status.configured", func(o kclient.Object) []string {
			return []string{o.(*v1.AuthProvider).Get("status.configured")}
		}).
		Build()
	pm := NewProxyManager(dispatcher.New(nil, storageClient, nil, nil, "", "", ""), "", "")

	for _, scenario := range []struct {
		name    string
		manager *Manager
		user    user.DefaultInfo
	}{
		{
			name: "auth disabled",
		},
		{
			name:    "no configured provider",
			manager: pm,
		},
		{
			name:    "proxy creation fails",
			manager: pm,
			// The session names a deleted provider, so URLForAuthProvider fails its lookup.
			user: user.DefaultInfo{
				Extra: map[string][]string{
					"auth_provider_namespace": {system.DefaultNamespace},
					"auth_provider_name":      {"deleted-provider"},
				},
			},
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			for _, tt := range []struct {
				name string
				rd   string
				want string
			}{
				{
					name: "empty",
					want: "/",
				},
				{
					name: "local path",
					rd:   "/admin?tab=users#details",
					want: "/admin?tab=users#details",
				},
				{
					name: "absolute URL",
					rd:   "https://evil.example.com",
					want: "/",
				},
				{
					name: "protocol relative URL",
					rd:   "//evil.example.com",
					want: "/",
				},
				{
					name: "backslash",
					rd:   `/\evil.example.com`,
					want: "/",
				},
				{
					name: "tab",
					rd:   "/\t/evil.example.com",
					want: "/",
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, "/oauth2/sign_out?rd="+url.QueryEscape(tt.rd), nil)
					rec := httptest.NewRecorder()

					scenario.manager.ServeHTTP(&scenario.user, rec, req)

					if rec.Code != http.StatusFound {
						t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
					}
					if got := rec.Header().Get("Location"); got != tt.want {
						t.Errorf("Location = %q, want %q", got, tt.want)
					}
				})
			}
		})
	}
}

// Sign-out is a GET that a browser sends with the session cookie when any page navigates to it, so a
// sign-out that a page outside Obot starts must be refused. An allowed sign-out falls through to the
// redirect for a deployment without a configured provider.
func TestServeHTTPRejectsForgedSignOut(t *testing.T) {
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.AuthProvider{}, "status.configured", func(o kclient.Object) []string {
			return []string{o.(*v1.AuthProvider).Get("status.configured")}
		}).
		Build()
	pm := NewProxyManager(dispatcher.New(nil, storageClient, nil, nil, "", "", ""), "https://obot.example.ai", "https://ui.example.ai")

	for _, tt := range []struct {
		name    string
		headers map[string]string
		// setupCookie is the name of the setup login cookie that the browser sends, if any.
		setupCookie string
		rejected    bool
		clearsSetup bool
	}{
		{
			name: "Log out button",
			headers: map[string]string{
				"Sec-Fetch-Site": "same-origin",
			},
		},
		{
			name: "typed or bookmarked URL",
			headers: map[string]string{
				"Sec-Fetch-Site": "none",
			},
		},
		{
			name: "client without fetch metadata",
		},
		{
			name: "UI served from its own hostname",
			headers: map[string]string{
				"Sec-Fetch-Site": "same-site",
				"Referer":        "https://ui.example.ai/profile",
			},
		},
		{
			name: "same-site tenant navigates to sign-out",
			headers: map[string]string{
				"Sec-Fetch-Site": "same-site",
				"Referer":        "https://tenant-b.example.ai/",
			},
			rejected: true,
		},
		{
			name: "cross-site page without a Referer",
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
			},
			rejected: true,
		},
		{
			name: "setup login redirected from the identity provider",
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Referer":        "https://idp.example.com/",
			},
			setupCookie: auth.SetupLoginCookieName(true),
			clearsSetup: true,
		},
		{
			// A sibling subdomain can plant a cookie for the parent domain, which the browser then sends to Obot,
			// but it cannot plant a __Host- one. Over https only the prefixed name counts.
			name: "same-site tenant plants an unprefixed setup login cookie",
			headers: map[string]string{
				"Sec-Fetch-Site": "same-site",
				"Referer":        "https://tenant-b.example.ai/",
			},
			setupCookie: auth.SetupLoginCookieName(false),
			rejected:    true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/oauth2/sign_out?rd=%2Fadmin", nil)
			for name, value := range tt.headers {
				req.Header.Set(name, value)
			}
			if tt.setupCookie != "" {
				req.AddCookie(&http.Cookie{Name: tt.setupCookie, Value: "setup-request"})
			}
			rec := httptest.NewRecorder()

			pm.ServeHTTP(&user.DefaultInfo{}, rec, req)

			if tt.rejected {
				if rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
				}
				return
			}
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/admin" {
				t.Fatalf("status = %d, Location = %q, want %d to /admin", rec.Code, rec.Header().Get("Location"), http.StatusFound)
			}

			var cleared bool
			for _, c := range rec.Result().Cookies() {
				if c.Name == auth.SetupLoginCookieName(true) && c.MaxAge < 0 && c.Secure {
					cleared = true
				}
			}
			if cleared != tt.clearsSetup {
				t.Fatalf("cleared %s = %v, want %v", auth.SetupLoginCookieName(true), cleared, tt.clearsSetup)
			}
		})
	}
}

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

func TestCheckStateResponse(t *testing.T) {
	tests := []struct {
		name               string
		statusCode         int
		body               string
		wantErr            bool
		wantInvalidSession bool
	}{
		{
			name:       "ok",
			statusCode: http.StatusOK,
			body:       `{"user":"abc"}`,
		},
		{
			name:               "unauthorized",
			statusCode:         http.StatusUnauthorized,
			body:               "failed to get state: failed to refresh token: invalid session: refreshing token returned 401: Unauthorized\n",
			wantErr:            true,
			wantInvalidSession: true,
		},
		{
			name:               "older provider failing to refresh with 401",
			statusCode:         http.StatusInternalServerError,
			body:               "failed to get state: failed to refresh token: refreshing token returned 401: Unauthorized\n",
			wantErr:            true,
			wantInvalidSession: true,
		},
		{
			name:               "older provider failing to refresh with 403",
			statusCode:         http.StatusInternalServerError,
			body:               "failed to get state: failed to refresh token: refreshing token returned 403: Forbidden\n",
			wantErr:            true,
			wantInvalidSession: true,
		},
		{
			name:               "session record not found",
			statusCode:         http.StatusInternalServerError,
			body:               "failed to get state: failed to load cookied session: record not found\n",
			wantErr:            true,
			wantInvalidSession: true,
		},
		{
			name:               "session ticket failed validation",
			statusCode:         http.StatusInternalServerError,
			body:               "failed to get state: session ticket cookie failed validation: bad signature\n",
			wantErr:            true,
			wantInvalidSession: true,
		},
		{
			name:               "OAuth provider rejects a Local session cookie",
			statusCode:         http.StatusInternalServerError,
			body:               "failed to get state: failed to load cookied session: cookie signature not valid\n",
			wantErr:            true,
			wantInvalidSession: true,
		},
		{
			name:       "other provider error",
			statusCode: http.StatusInternalServerError,
			body:       "failed to get state: failed to load cookied session: error loading postgres session: connection refused\n",
			wantErr:    true,
		},
		{
			name:       "bad request",
			statusCode: http.StatusBadRequest,
			body:       "failed to decode request body: EOF\n",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkStateResponse(tt.statusCode, []byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkStateResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := errors.Is(err, ErrInvalidSession); got != tt.wantInvalidSession {
				t.Errorf("errors.Is(err, ErrInvalidSession) = %v, want %v (err: %v)", got, tt.wantInvalidSession, err)
			}
		})
	}
}

func TestProxyServeHTTPPathAllowlist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	p, err := newProxy(system.DefaultNamespace, "test-auth-provider", upstream.URL, "")
	if err != nil {
		t.Fatalf("newProxy() error = %v", err)
	}

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "start",
			path:       "/oauth2/start",
			wantStatus: http.StatusOK,
		},
		{
			name:       "callback",
			path:       "/oauth2/callback",
			wantStatus: http.StatusOK,
		},
		{
			name:       "error page stylesheet",
			path:       "/oauth2/static/css/obot-error.min.css",
			wantStatus: http.StatusOK,
		},
		{
			name:       "error page logo",
			path:       "/oauth2/static/images/obot-icon-grumpy-blue.svg",
			wantStatus: http.StatusOK,
		},
		{
			name:       "other static file",
			path:       "/oauth2/static/css/bulma.min.css",
			wantStatus: http.StatusOK,
		},
		{
			name:       "static directory listing",
			path:       "/oauth2/static/css/",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "static root directory listing",
			path:       "/oauth2/static/",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "static prefix without trailing slash",
			path:       "/oauth2/static",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "path traversal out of static",
			path:       "/oauth2/static/../userinfo",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unclean static path",
			path:       "/oauth2/static//css/obot-error.min.css",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unlisted path",
			path:       "/oauth2/userinfo",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			// Set the path directly, so that unclean paths reach serveHTTP as a client could send them.
			req.URL.Path = tt.path
			rec := httptest.NewRecorder()
			p.serveHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
