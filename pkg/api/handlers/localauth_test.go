package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/setup"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/bootstrap"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	localSetupPassword = "initial-local-password"
)

type localSetupProviderGetter struct {
	name string
}

type localSetupFixture struct {
	gateway   *gateway.Client
	storage   storage.Client
	provider  *localauth.Provider
	handler   *LocalAuthHandler
	setup     *setup.Handler
	bootstrap *bootstrap.Bootstrap
	getter    *localSetupProviderGetter
}

func (g *localSetupProviderGetter) GetConfiguredAuthProvider(context.Context) (string, error) {
	return g.name, nil
}

func newLocalSetupFixture(t *testing.T, owners, admins []string) *localSetupFixture {
	t.Helper()

	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	storageClient := newFakeStorage(t, &v1.UserDefaultRoleSetting{
		Name:      system.DefaultRoleSettingName,
		Namespace: system.DefaultNamespace,
		Spec: v1.UserDefaultRoleSettingSpec{
			Role: apitypes.RoleBasic,
		},
	})
	c := gateway.New(t.Context(), db, storageClient, nil, nil, owners, admins, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = c.Close() })

	require.NoError(t, c.UpsertCredential(t.Context(), gwtypes.Credential{
		Context: system.LocalAuthProvider,
		Name:    system.LocalAuthProvider,
		Secrets: map[string]string{localauth.EmailDomainsEnvVar: "*"},
	}))

	provider, err := localauth.New(c, "https://obot.example.com")
	require.NoError(t, err)
	getter := &localSetupProviderGetter{name: system.LocalAuthProvider}
	t.Setenv("OBOT_BOOTSTRAP_TOKEN", "test-bootstrap-token")
	boot, err := bootstrap.New(t.Context(), "https://obot.example.com", c, getter, true, false, false)
	require.NoError(t, err)

	return &localSetupFixture{
		gateway:   c,
		storage:   storageClient,
		provider:  provider,
		handler:   NewLocalAuthHandler(provider),
		setup:     setup.NewHandler("https://obot.example.com", boot, getter),
		bootstrap: boot,
		getter:    getter,
	}
}

func (f *localSetupFixture) create(t *testing.T, caller, email string, passwordChange bool) error {
	t.Helper()

	body, err := json.Marshal(localAuthUserRequest{
		Email:                 email,
		Password:              localSetupPassword,
		RequirePasswordChange: &passwordChange,
	})
	require.NoError(t, err)

	return f.handler.Create(api.Context{
		Request:        httptest.NewRequest(http.MethodPost, "/api/local-auth/users", strings.NewReader(string(body))),
		ResponseWriter: httptest.NewRecorder(),
		GatewayClient:  f.gateway,
		User:           &kuser.DefaultInfo{Name: caller},
	})
}

func (f *localSetupFixture) identity(t *testing.T, email, provider string, role apitypes.Role) api.Context {
	t.Helper()

	u, err := f.gateway.EnsureIdentityWithRole(t.Context(), &gwtypes.Identity{
		Email:                 email,
		ProviderUsername:      email,
		ProviderUserID:        email,
		AuthProviderName:      provider,
		AuthProviderNamespace: system.DefaultNamespace,
	}, "", role, gateway.UserLimit{Unlimited: true})
	require.NoError(t, err)

	return api.Context{
		Request:        httptest.NewRequest(http.MethodGet, "/api/setup/oauth-complete", nil),
		ResponseWriter: httptest.NewRecorder(),
		GatewayClient:  f.gateway,
		Storage:        f.storage,
		User: &kuser.DefaultInfo{
			Name:   email,
			UID:    strconv.Itoa(int(u.ID)),
			Groups: role.Groups(),
			Extra: map[string][]string{
				"email":                   {email},
				"auth_provider_user_id":   {email},
				"auth_provider_name":      {provider},
				"auth_provider_namespace": {system.DefaultNamespace},
			},
		},
	}
}

func TestLocalAuthBootstrapCreation(t *testing.T) {
	f := newLocalSetupFixture(t, []string{"owner@example.com"}, []string{"admin@example.com"})

	err := f.create(t, system.BootstrapName, "Admin <ADMIN@example.com>", false)
	require.ErrorContains(t, err, "configured as an Admin")

	require.NoError(t, f.create(t, system.BootstrapName, "owner@example.com", false))
	err = f.create(t, system.BootstrapName, "other@example.com", false)
	var httpErr *apitypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusConflict, httpErr.Code)

	require.NoError(t, f.create(t, "owner@example.com", "admin@example.com", true))
	users, err := f.gateway.LocalAuthUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 2)
}

func TestLocalAuthBootstrapLoginCompletesInOneSession(t *testing.T) {
	for _, passwordChange := range []bool{false, true} {
		t.Run(strconv.FormatBool(passwordChange), func(t *testing.T) {
			f := newLocalSetupFixture(t, nil, nil)
			require.NoError(t, f.create(t, system.BootstrapName, "owner@example.com", passwordChange))
			providerURL, err := f.provider.Start(t.Context())
			require.NoError(t, err)

			httpClient := &http.Client{
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			}
			login := func(password string) *http.Response {
				t.Helper()

				response, err := httpClient.PostForm(providerURL.String()+"/oauth2/start", url.Values{
					"email":    {"owner@example.com"},
					"password": {password},
					"rd":       {"/api/setup/oauth-complete"},
				})
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				return response
			}

			failed := login("wrong-password")
			require.Empty(t, failed.Cookies())
			require.Contains(t, failed.Header.Get("Location"), "/login/local?")

			response := login(localSetupPassword)
			require.Equal(t, http.StatusFound, response.StatusCode)
			require.Len(t, response.Cookies(), 1)
			cookie := response.Cookies()[0]
			require.Equal(t, auth.ObotAccessTokenCookie, cookie.Name)
			sessionID := hash.String(cookie.Value)

			req := f.identity(t, "owner@example.com", system.LocalAuthProvider, apitypes.RoleBasic)
			req.AddCookie(cookie)
			req.AddCookie(&http.Cookie{Name: bootstrap.ObotBootstrapCookie, Value: "test-bootstrap-token"})

			if passwordChange {
				require.Contains(t, response.Header.Get("Location"), "/change-password?rd=")
				require.ErrorContains(t, f.setup.OAuthComplete(req), "change your password")

				_, user, err := f.gateway.LocalAuthSession(t.Context(), sessionID)
				require.NoError(t, err)
				require.NoError(t, f.provider.ChangePassword(t.Context(), user.ID, "chosen-owner-password", sessionID))
			} else {
				require.Equal(t, "/api/setup/oauth-complete", response.Header.Get("Location"))
			}

			require.NoError(t, f.setup.OAuthComplete(req))
			result := req.ResponseWriter.(*httptest.ResponseRecorder).Result()
			require.Equal(t, "/admin", result.Header.Get("Location"))
			require.Len(t, result.Cookies(), 1)
			require.Equal(t, bootstrap.ObotBootstrapCookie, result.Cookies()[0].Name)
			require.Equal(t, -1, result.Cookies()[0].MaxAge)

			_, _, err = f.gateway.LocalAuthSession(t.Context(), sessionID)
			require.NoError(t, err)
			user, err := f.gateway.UserByID(t.Context(), req.User.GetUID())
			require.NoError(t, err)
			require.True(t, user.Role.HasRole(apitypes.RoleOwner))
			require.Nil(t, f.gateway.GetTempUserCache(t.Context()))
			enabled, err := f.bootstrap.Enabled(t.Context())
			require.NoError(t, err)
			require.False(t, enabled)

			// A repeated callback with fresh authentication also finishes without logging out.
			req.User.(*kuser.DefaultInfo).Groups = user.Role.Groups()
			req.ResponseWriter = httptest.NewRecorder()
			require.NoError(t, f.setup.OAuthComplete(req))
			require.Equal(t, "/admin", req.ResponseWriter.(*httptest.ResponseRecorder).Header().Get("Location"))
		})
	}
}

func TestLocalAuthSetupPromotionGuards(t *testing.T) {
	for _, test := range []struct {
		name       string
		provider   string
		configured string
		localUsers int
		email      string
		admins     []string
		owner      bool
		wantError  string
	}{
		{
			name:       "external provider still requires handoff",
			provider:   "google-auth-provider",
			configured: "google-auth-provider",
			localUsers: 1,
			email:      "owner@example.com",
		},
		{
			name:       "legacy multiple accounts still require handoff",
			provider:   system.LocalAuthProvider,
			configured: system.LocalAuthProvider,
			localUsers: 2,
			email:      "owner@example.com",
		},
		{
			name:       "different local identity",
			provider:   system.LocalAuthProvider,
			configured: system.LocalAuthProvider,
			localUsers: 1,
			email:      "other@example.com",
			wantError:  "sign in with the initial local account",
		},
		{
			name:       "local provider is not active",
			provider:   system.LocalAuthProvider,
			configured: "google-auth-provider",
			localUsers: 1,
			email:      "owner@example.com",
			wantError:  "not the configured provider",
		},
		{
			name:       "no local account",
			provider:   system.LocalAuthProvider,
			configured: system.LocalAuthProvider,
			email:      "owner@example.com",
			wantError:  "no local account",
		},
		{
			name:       "explicit admin remains admin",
			provider:   system.LocalAuthProvider,
			configured: system.LocalAuthProvider,
			localUsers: 1,
			email:      "owner@example.com",
			admins:     []string{"owner@example.com"},
			wantError:  "configured as an Admin",
		},
		{
			name:       "setup already completed",
			provider:   system.LocalAuthProvider,
			configured: system.LocalAuthProvider,
			localUsers: 1,
			email:      "owner@example.com",
			owner:      true,
			wantError:  "not found",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newLocalSetupFixture(t, nil, test.admins)
			f.getter.name = test.configured

			for i := range test.localUsers {
				email := "owner@example.com"
				if i > 0 {
					email = "legacy@example.com"
				}

				require.NoError(t, f.create(t, "existing-admin", email, false))
			}

			if test.owner {
				f.identity(t, "existing-owner@example.com", test.provider, apitypes.RoleOwner)
			}

			req := f.identity(t, test.email, test.provider, apitypes.RoleBasic)
			err := f.setup.OAuthComplete(req)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				require.Nil(t, f.gateway.GetTempUserCache(t.Context()))
			} else {
				require.NoError(t, err)
				require.NotNil(t, f.gateway.GetTempUserCache(t.Context()))
				require.Equal(t, "/oauth2/sign_out?rd=/admin?setup=complete", req.ResponseWriter.(*httptest.ResponseRecorder).Header().Get("Location"))
			}

			user, err := f.gateway.UserByID(t.Context(), req.User.GetUID())
			require.NoError(t, err)
			require.False(t, user.Role.HasRole(apitypes.RoleOwner))
		})
	}
}
