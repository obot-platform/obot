package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/bootstrap"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func (f *localSetupFixture) createLocalOwner(caller, body string) (api.Context, *httptest.ResponseRecorder) {
	user := &kuser.DefaultInfo{Name: caller}
	if caller == system.BootstrapName {
		// Mirror the bootstrap authenticator: the bootstrap sign-in is recognized by its auth provider, not its name.
		user.Extra = map[string][]string{"auth_provider_name": {system.BootstrapName}}
	}
	recorder := httptest.NewRecorder()
	return api.Context{
		Request:        httptest.NewRequest(http.MethodPost, "/api/setup/local-owner", strings.NewReader(body)),
		ResponseWriter: recorder,
		GatewayClient:  f.gateway,
		Storage:        f.storage,
		User:           user,
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

// A disabled user already holds the submitted email, so no Owner is recorded. The new local account is removed again,
// so the next submission is not refused because a local account exists.
func TestCreateLocalOwnerRejectsDisabledUserAndRemovesAccount(t *testing.T) {
	f := newLocalSetupFixture(t)

	disabled, err := f.gateway.EnsureIdentity(t.Context(), localauth.Identity("owner@example.com"), "", gateway.UserLimit{Unlimited: true})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&gwtypes.User{}).Where("id = ?", disabled.ID).Update("disabled_at", time.Now()).Error)

	req, recorder := f.createLocalOwner(system.BootstrapName, `{"email":"owner@example.com","password":"`+localSetupPassword+`"}`)
	var httpErr *apitypes.ErrHTTP
	require.ErrorAs(t, f.setup.CreateLocalOwner(req), &httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
	require.Contains(t, httpErr.Message, "disabled user")
	require.Empty(t, recorder.Result().Cookies())

	localUsers, err := f.gateway.LocalAuthUsers(t.Context())
	require.NoError(t, err)
	require.Empty(t, localUsers)

	stillDisabled, err := f.gateway.UserFromProviderUserID(t.Context(), system.DefaultNamespace, system.LocalAuthProvider, "owner@example.com")
	require.NoError(t, err)
	require.NotNil(t, stillDisabled.DisabledAt)
	require.False(t, stillDisabled.Role.HasRole(apitypes.RoleOwner))

	setupEnabled, err := f.bootstrap.SetupEnabled(t.Context())
	require.NoError(t, err)
	require.True(t, setupEnabled)

	// A retry with another email succeeds.
	req, recorder = f.createLocalOwner(system.BootstrapName, `{"email":"other@example.com","password":"`+localSetupPassword+`"}`)
	require.NoError(t, f.setup.CreateLocalOwner(req))
	require.JSONEq(t, `{"email":"other@example.com","redirect":"/admin"}`, recorder.Body.String())

	owner, err := f.gateway.UserFromProviderUserID(t.Context(), system.DefaultNamespace, system.LocalAuthProvider, "other@example.com")
	require.NoError(t, err)
	require.True(t, owner.Role.HasRole(apitypes.RoleOwner))
}

// The Owner is recorded, which closes setup, but the session cannot be started. The account behind the Owner is kept,
// and the browser is sent to sign in with it rather than left holding the bootstrap cookie.
func TestCreateLocalOwnerSendsRecordedOwnerToSignInWhenSessionFails(t *testing.T) {
	f := newLocalSetupFixture(t)
	require.NoError(t, f.db.Exec(`CREATE TRIGGER fail_local_auth_sessions BEFORE INSERT ON local_auth_sessions
BEGIN SELECT RAISE(ABORT, 'session store unavailable'); END`).Error)

	req, recorder := f.createLocalOwner(system.BootstrapName, `{"email":"owner@example.com","password":"`+localSetupPassword+`"}`)
	require.NoError(t, f.setup.CreateLocalOwner(req))
	require.JSONEq(t, `{"email":"owner@example.com","redirect":"/login/local?rd=%2Fadmin"}`, recorder.Body.String())

	cookies := map[string]*http.Cookie{}
	for _, cookie := range recorder.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	require.NotNil(t, cookies[bootstrap.ObotBootstrapCookie])
	require.Equal(t, -1, cookies[bootstrap.ObotBootstrapCookie].MaxAge)
	require.Nil(t, cookies[auth.ObotAccessTokenCookie])

	localUsers, err := f.gateway.LocalAuthUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, localUsers, 1)
	require.Equal(t, "owner@example.com", localUsers[0].Email)

	owner, err := f.gateway.UserFromProviderUserID(t.Context(), system.DefaultNamespace, system.LocalAuthProvider, "owner@example.com")
	require.NoError(t, err)
	require.True(t, owner.Role.HasRole(apitypes.RoleOwner))

	setupEnabled, err := f.bootstrap.SetupEnabled(t.Context())
	require.NoError(t, err)
	require.False(t, setupEnabled)
}

// EnsureIdentityWithRole commits the Owner and then fails. The Owner it committed is used, so setup finishes and the
// browser is signed in rather than the account being removed from behind a recorded Owner.
func TestCreateLocalOwnerUsesOwnerRecordedBeforeEnsureIdentityFailed(t *testing.T) {
	f := newLocalSetupFixture(t)
	f.storage.failRoleChanges.Store(true)

	req, recorder := f.createLocalOwner(system.BootstrapName, `{"email":"owner@example.com","password":"`+localSetupPassword+`"}`)
	require.NoError(t, f.setup.CreateLocalOwner(req))
	require.JSONEq(t, `{"email":"owner@example.com","redirect":"/admin"}`, recorder.Body.String())

	cookies := map[string]*http.Cookie{}
	for _, cookie := range recorder.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	require.Equal(t, -1, cookies[bootstrap.ObotBootstrapCookie].MaxAge)
	require.NotNil(t, cookies[auth.ObotAccessTokenCookie])

	localUsers, err := f.gateway.LocalAuthUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, localUsers, 1)

	owner, err := f.gateway.UserFromProviderUserID(t.Context(), system.DefaultNamespace, system.LocalAuthProvider, "owner@example.com")
	require.NoError(t, err)
	require.True(t, owner.Role.HasRole(apitypes.RoleOwner))

	setupEnabled, err := f.bootstrap.SetupEnabled(t.Context())
	require.NoError(t, err)
	require.False(t, setupEnabled)
}
