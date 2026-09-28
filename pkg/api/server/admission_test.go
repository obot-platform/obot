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

	for _, tt := range []struct {
		name            string
		err             error
		wantStatus      int
		wantBody        string
		wantCookieReset bool
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

			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}

			var cookieReset bool
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == proxy.ObotAccessTokenCookie && cookie.MaxAge < 0 {
					cookieReset = true
				}
			}
			if cookieReset != tt.wantCookieReset {
				t.Errorf("session cookie cleared = %v, want %v", cookieReset, tt.wantCookieReset)
			}
		})
	}
}
