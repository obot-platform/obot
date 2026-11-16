package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/bootstrap"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func (f *localSetupFixture) createLocalOwner(caller, body string) (api.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	return api.Context{
		Request:        httptest.NewRequest(http.MethodPost, "/api/setup/local-owner", strings.NewReader(body)),
		ResponseWriter: recorder,
		GatewayClient:  f.gateway,
		Storage:        f.storage,
		User:           &kuser.DefaultInfo{Name: caller},
	}, recorder
}

func TestCreateLocalOwnerSignsInAsOwnerAndClosesSetup(t *testing.T) {
	f := newLocalSetupFixture(t)

	req, recorder := f.createLocalOwner(system.BootstrapName, `{"email":"Owner@Example.com","password":"`+localSetupPassword+`"}`)
	require.NoError(t, f.setup.CreateLocalOwner(req))
	require.JSONEq(t, `{"email":"owner@example.com","redirect":"/admin"}`, recorder.Body.String())

	cookies := map[string]*http.Cookie{}
	for _, cookie := range recorder.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	require.Equal(t, -1, cookies[bootstrap.ObotBootstrapCookie].MaxAge)
	session := cookies[auth.ObotAccessTokenCookie]
	require.NotNil(t, session)
	require.NotEmpty(t, session.Value)

	_, localUser, err := f.gateway.LocalAuthSession(t.Context(), hash.String(session.Value))
	require.NoError(t, err)
	require.Equal(t, "owner@example.com", localUser.Email)
	require.False(t, localUser.RequirePasswordChange)

	user, err := f.gateway.UserFromProviderUserID(t.Context(), system.DefaultNamespace, system.LocalAuthProvider, "owner@example.com")
	require.NoError(t, err)
	require.True(t, user.Role.HasRole(apitypes.RoleOwner))

	setupEnabled, err := f.bootstrap.SetupEnabled(t.Context())
	require.NoError(t, err)
	require.False(t, setupEnabled)

	// The first request with the new session resolves to the same Owner rather than a new user.
	signedIn := f.identity(t, "owner@example.com", system.LocalAuthProvider, apitypes.RoleBasic)
	require.Equal(t, strconv.Itoa(int(user.ID)), signedIn.User.GetUID())

	// Setup is closed, so another submission is refused.
	req, _ = f.createLocalOwner(system.BootstrapName, `{"email":"other@example.com","password":"`+localSetupPassword+`"}`)
	var httpErr *apitypes.ErrHTTP
	require.ErrorAs(t, f.setup.CreateLocalOwner(req), &httpErr)
	require.Equal(t, http.StatusNotFound, httpErr.Code)
}

func TestCreateLocalOwnerGuards(t *testing.T) {
	for _, test := range []struct {
		name          string
		caller        string
		configured    string
		existingLocal bool
		existingOwner bool
		body          string
		wantCode      int
		wantMessage   string
	}{
		{
			name:       "non-bootstrap caller",
			caller:     "someone@example.com",
			configured: system.LocalAuthProvider,
			body:       `{"email":"owner@example.com","password":"` + localSetupPassword + `"}`,
			wantCode:   http.StatusForbidden,
		},
		{
			name:          "owner already signed in",
			caller:        system.BootstrapName,
			configured:    system.LocalAuthProvider,
			existingOwner: true,
			body:          `{"email":"owner@example.com","password":"` + localSetupPassword + `"}`,
			wantCode:      http.StatusNotFound,
		},
		{
			name:        "another provider is configured",
			caller:      system.BootstrapName,
			configured:  "google-auth-provider",
			body:        `{"email":"owner@example.com","password":"` + localSetupPassword + `"}`,
			wantCode:    http.StatusPreconditionFailed,
			wantMessage: "not the configured provider",
		},
		{
			name:          "a local account already exists",
			caller:        system.BootstrapName,
			configured:    system.LocalAuthProvider,
			existingLocal: true,
			body:          `{"email":"owner@example.com","password":"` + localSetupPassword + `"}`,
			wantCode:      http.StatusConflict,
			wantMessage:   "local account already exists",
		},
		{
			name:        "password too short",
			caller:      system.BootstrapName,
			configured:  system.LocalAuthProvider,
			body:        `{"email":"owner@example.com","password":"short"}`,
			wantCode:    http.StatusBadRequest,
			wantMessage: "password must be at least",
		},
		{
			name:        "invalid email",
			caller:      system.BootstrapName,
			configured:  system.LocalAuthProvider,
			body:        `{"email":"invalid","password":"` + localSetupPassword + `"}`,
			wantCode:    http.StatusBadRequest,
			wantMessage: "valid email address",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newLocalSetupFixture(t)
			f.getter.name = test.configured
			if test.existingLocal {
				require.NoError(t, f.create(t, system.BootstrapName, "first@example.com", false))
			}
			if test.existingOwner {
				f.identity(t, "existing-owner@example.com", test.configured, apitypes.RoleOwner)
			}
			before, err := f.gateway.LocalAuthUsers(t.Context())
			require.NoError(t, err)

			req, recorder := f.createLocalOwner(test.caller, test.body)
			err = f.setup.CreateLocalOwner(req)
			var httpErr *apitypes.ErrHTTP
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, test.wantCode, httpErr.Code)
			if test.wantMessage != "" {
				require.Contains(t, httpErr.Message, test.wantMessage)
			}
			require.Empty(t, recorder.Result().Cookies())

			after, err := f.gateway.LocalAuthUsers(t.Context())
			require.NoError(t, err)
			require.Len(t, after, len(before))
			_, err = f.gateway.UserFromProviderUserID(t.Context(), system.DefaultNamespace, system.LocalAuthProvider, "owner@example.com")
			require.Error(t, err)
		})
	}
}

func TestLocalAuthIdentityMatchesProxyIdentity(t *testing.T) {
	id := localauth.Identity(" Owner@Example.com ")
	require.Equal(t, "owner@example.com", id.Email)
	require.Equal(t, "owner@example.com", id.ProviderUserID)
	require.Equal(t, "owner@example.com", id.ProviderUsername)
	require.Equal(t, system.LocalAuthProvider, id.AuthProviderName)
	require.Equal(t, system.DefaultNamespace, id.AuthProviderNamespace)
}
