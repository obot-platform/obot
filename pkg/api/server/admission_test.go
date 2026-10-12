package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/proxy"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

type failingAuthenticator struct {
	err error
}

func (f failingAuthenticator) AuthenticateRequest(*http.Request) (*authenticator.Response, bool, error) {
	return nil, false, f.err
}

func TestWrapRefusesInactiveAndUnverifiableUsers(t *testing.T) {
	denied := &gclient.UserAccessDeniedError{
		UserID: 7,
		Status: types2.UserStatusDisabled,
	}
	lookup := &gclient.UserAccessLookupError{
		UserID: 7,
		Err:    errors.New("database unavailable"),
	}

	const pageAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

	for _, tt := range []struct {
		name string
		err  error
		// path is the request's path, /api/me unless set, and accept its Accept header.
		path            string
		accept          string
		wantStatus      int
		wantBody        string
		wantLocation    string
		wantCookieReset bool
		// wantInactiveCookie is set when the response tells the login page that the account is not active.
		wantInactiveCookie bool
	}{
		{
			name:            "denied by the admission check",
			err:             denied,
			wantStatus:      http.StatusForbidden,
			wantBody:        gclient.AccountNotActiveMessage,
			wantCookieReset: true,
		},
		{
			name:            "denied by an authenticator inside the chain",
			err:             utilerrors.NewAggregate([]error{utilerrors.NewAggregate([]error{errors.New("declined"), denied})}),
			wantStatus:      http.StatusForbidden,
			wantBody:        gclient.AccountNotActiveMessage,
			wantCookieReset: true,
		},
		{
			name:               "a page load denied by the admission check goes to the login page, which says why",
			err:                denied,
			path:               "/mcp-servers",
			accept:             pageAccept,
			wantStatus:         http.StatusFound,
			wantLocation:       accountInactiveLoginPath,
			wantCookieReset:    true,
			wantInactiveCookie: true,
		},
		{
			name:            "the login page is refused as text rather than redirected again",
			err:             denied,
			path:            accountInactiveLoginPath,
			accept:          pageAccept,
			wantStatus:      http.StatusForbidden,
			wantBody:        gclient.AccountNotActiveMessage,
			wantCookieReset: true,
		},
		{
			name:            "an asset load denied by the admission check",
			err:             denied,
			path:            "/_app/immutable/entry/app.js",
			accept:          "*/*",
			wantStatus:      http.StatusForbidden,
			wantBody:        gclient.AccountNotActiveMessage,
			wantCookieReset: true,
		},
		{
			name:            "an API call from a page denied by the admission check",
			err:             denied,
			accept:          pageAccept,
			wantStatus:      http.StatusForbidden,
			wantBody:        gclient.AccountNotActiveMessage,
			wantCookieReset: true,
		},
		{
			name:       "user status could not be read",
			err:        utilerrors.NewAggregate([]error{lookup}),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "Unable to verify account status",
		},
		{
			name:       "other authentication failure",
			err:        errors.New("bad credentials"),
			wantStatus: http.StatusUnauthorized,
			wantBody:   "bad credentials",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				authenticator: authn.NewAuthenticator(failingAuthenticator{
					err: tt.err,
				}),
			}
			handler := s.Wrap(func(api.Context) error {
				t.Fatal("the handler ran for an unauthenticated request")
				return nil
			})

			path := tt.path
			if path == "" {
				path = "/api/me"
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
			if location := rec.Header().Get("Location"); location != tt.wantLocation {
				t.Errorf("Location = %q, want %q", location, tt.wantLocation)
			}

			var cookieReset, inactiveCookie bool
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == proxy.ObotAccessTokenCookie && cookie.MaxAge < 0 {
					cookieReset = true
				}
				if cookie.Name == accountInactiveCookie && cookie.Value == "true" && cookie.MaxAge > 0 {
					inactiveCookie = true
				}
			}
			if cookieReset != tt.wantCookieReset {
				t.Errorf("session cookie cleared = %v, want %v", cookieReset, tt.wantCookieReset)
			}
			if inactiveCookie != tt.wantInactiveCookie {
				t.Errorf("account inactive cookie set = %v, want %v", inactiveCookie, tt.wantInactiveCookie)
			}
		})
	}
}
