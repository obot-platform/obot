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
	"github.com/obot-platform/obot/pkg/i18n"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func newLocalSetupFixture(t *testing.T) *localSetupFixture {
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
	c := gateway.New(t.Context(), db, storageClient, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = c.Close() })

	require.NoError(t, c.UpsertCredential(t.Context(), gwtypes.Credential{
		Context: system.LocalAuthProvider,
		Name:    system.LocalAuthProvider,
		Secrets: map[string]string{localauth.EmailDomainsEnvVar: "*"},
	}))

	provider, err := localauth.New(c, "https://obot.example.com", "https://obot.example.com")
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

	user := &kuser.DefaultInfo{Name: caller}
	if caller == system.BootstrapName {
		// Mirror the bootstrap authenticator: the bootstrap sign-in is recognized by its auth provider, not its name.
		user.Extra = map[string][]string{"auth_provider_name": {system.BootstrapName}}
	}
	return f.createAs(t, user, email, passwordChange)
}

func (f *localSetupFixture) createAs(t *testing.T, user kuser.Info, email string, passwordChange bool) error {
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
		User:           user,
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

	// The gateway sets this cookie when a setup login finishes its OAuth round trip, which is the only way to reach
	// the endpoint in a browser.
	request := httptest.NewRequest(http.MethodGet, "/api/setup/oauth-complete", nil)
	request.AddCookie(&http.Cookie{Name: auth.SetupLoginCookieName(true), Value: "setup-request"})

	return api.Context{
		Request:        request,
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
	f := newLocalSetupFixture(t)

	require.NoError(t, f.create(t, system.BootstrapName, "owner@example.com", false))
	err := f.create(t, system.BootstrapName, "other@example.com", false)
	var httpErr *apitypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusConflict, httpErr.Code)

	require.NoError(t, f.create(t, "owner@example.com", "admin@example.com", true))
	users, err := f.gateway.LocalAuthUsers(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 2)
}

// A user who renamed themself to "bootstrap" signed in through a real auth provider, so they are not the bootstrap
// user and the bootstrap user's one-user limit does not apply to them.
func TestLocalAuthUserNamedBootstrapIsNotBootstrap(t *testing.T) {
	f := newLocalSetupFixture(t)
	renamed := &kuser.DefaultInfo{
		Name: system.BootstrapName,
		Extra: map[string][]string{
			"auth_provider_name":      {"local-auth-provider"},
			"auth_provider_namespace": {"default"},
		},
	}

	require.NoError(t, f.createAs(t, renamed, "first@example.com", false))
	require.NoError(t, f.createAs(t, renamed, "second@example.com", false))
}

func TestLocalAuthValidationUsesRequestLocale(t *testing.T) {
	f := newLocalSetupFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/api/local-auth/users", strings.NewReader(`{"email":"invalid","password":"long-enough-password"}`))
	req = req.WithContext(i18n.WithLocale(req.Context(), "ko"))
	recorder := httptest.NewRecorder()
	err := f.handler.Create(api.Context{
		Request:        req,
		ResponseWriter: recorder,
		GatewayClient:  f.gateway,
		User:           &kuser.DefaultInfo{Name: "owner"},
	})
	var httpErr *apitypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
	require.Contains(t, httpErr.Message, "유효한 이메일 주소")
}

func TestLocalAuthUserRoleProtections(t *testing.T) {
	passwordHash, err := localauth.HashPassword(localSetupPassword)
	require.NoError(t, err)

	for _, operation := range []string{"reset password", "delete"} {
		for _, callerRole := range []apitypes.Role{apitypes.RoleAdmin, apitypes.RoleOwner} {
			for _, target := range []struct {
				name                string
				role                apitypes.Role
				provider            string
				pendingInitialOwner bool
				ownerGranted        bool
			}{
				{
					name:     "owner",
					role:     apitypes.RoleOwner,
					provider: system.LocalAuthProvider,
				},
				{
					name:     "auditor",
					role:     apitypes.RoleAuditor,
					provider: system.LocalAuthProvider,
				},
				{
					name:     "admin with auditor role",
					role:     apitypes.RoleAdmin | apitypes.RoleAuditor,
					provider: system.LocalAuthProvider,
				},
				{
					name:     "admin",
					role:     apitypes.RoleAdmin,
					provider: system.LocalAuthProvider,
				},
				{
					name:     "basic",
					role:     apitypes.RoleBasic,
					provider: system.LocalAuthProvider,
				},
				{
					name: "no org user",
				},
				{
					name:                "pending initial owner with no org user",
					pendingInitialOwner: true,
				},
				{
					name:                "pending initial owner whose user was granted Owner and deleted",
					pendingInitialOwner: true,
					ownerGranted:        true,
				},
				{
					name:     "owner under another provider with same email",
					role:     apitypes.RoleOwner,
					provider: "google-auth-provider",
				},
			} {
				t.Run(operation+"/"+strconv.Itoa(int(callerRole))+"/"+target.name, func(t *testing.T) {
					f := newLocalSetupFixture(t)
					// Keep a separate owner so last-owner/admin checks do not mask authorization.
					f.identity(t, "existing-owner@example.com", system.LocalAuthProvider, apitypes.RoleOwner)
					var localUser *gwtypes.LocalAuthUser
					if target.pendingInitialOwner {
						localUser, err = f.gateway.CreateInitialLocalAuthUser(t.Context(), "target@example.com", passwordHash, hash.String("setup-token"), time.Now().Add(time.Hour))
					} else {
						localUser, err = f.gateway.CreateLocalAuthUser(t.Context(), "target@example.com", passwordHash, false)
					}
					require.NoError(t, err)
					if target.ownerGranted {
						// The first sign-in grants Owner. Deleting that user leaves the account without one, and the
						// grant is not given again.
						userID, err := strconv.ParseUint(f.identity(t, localUser.Email, system.LocalAuthProvider, apitypes.RoleBasic).User.GetUID(), 10, 64)
						require.NoError(t, err)
						_, err = f.gateway.RemoveIdentityAndUser(t.Context(), &gwtypes.Identity{UserID: uint(userID)})
						require.NoError(t, err)
					}
					var orgUserID string
					if target.provider != "" {
						orgUserID = f.identity(t, localUser.Email, target.provider, target.role).User.GetUID()
					}
					sessionID := hash.String("target-session")
					require.NoError(t, f.gateway.CreateLocalAuthSession(t.Context(), sessionID, localUser.ID, time.Now().Add(time.Hour)))

					request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"password":"replacement-password"}`))
					request.SetPathValue("id", strconv.FormatUint(uint64(localUser.ID), 10))
					req := api.Context{
						Request:        request,
						ResponseWriter: httptest.NewRecorder(),
						GatewayClient:  f.gateway,
						Storage:        f.storage,
						User:           &kuser.DefaultInfo{Groups: callerRole.Groups()},
					}
					if operation == "delete" {
						err = f.handler.Delete(req)
					} else {
						err = f.handler.SetPassword(req)
					}

					privileged := target.role.HasRole(apitypes.RoleOwner) || target.role.HasRole(apitypes.RoleAuditor)
					denied := callerRole != apitypes.RoleOwner && (target.pendingInitialOwner && !target.ownerGranted || target.provider == system.LocalAuthProvider && privileged)
					if denied {
						var httpErr *apitypes.ErrHTTP
						require.ErrorAs(t, err, &httpErr)
						require.Equal(t, http.StatusForbidden, httpErr.Code)
						stored, err := f.gateway.LocalAuthUserByID(t.Context(), localUser.ID)
						require.NoError(t, err)
						require.Equal(t, passwordHash, stored.PasswordHash)
						require.Equal(t, localUser.RequirePasswordChange, stored.RequirePasswordChange)
						_, _, err = f.gateway.LocalAuthSession(t.Context(), sessionID)
						require.NoError(t, err)
						if orgUserID != "" {
							_, err = f.gateway.UserByID(t.Context(), orgUserID)
							require.NoError(t, err)
						}
						var cleanups v1.UserDeleteList
						require.NoError(t, f.storage.List(t.Context(), &cleanups))
						require.Empty(t, cleanups.Items)
						return
					}

					require.NoError(t, err)
					_, _, err = f.gateway.LocalAuthSession(t.Context(), sessionID)
					require.ErrorIs(t, err, gorm.ErrRecordNotFound)
					stored, err := f.gateway.LocalAuthUserByID(t.Context(), localUser.ID)
					if operation == "delete" {
						require.ErrorIs(t, err, gorm.ErrRecordNotFound)
					} else {
						require.NoError(t, err)
						require.NoError(t, localauth.VerifyPassword(stored.PasswordHash, "replacement-password"))
					}
					if orgUserID != "" {
						_, err := f.gateway.UserByID(t.Context(), orgUserID)
						if operation == "delete" && target.provider == system.LocalAuthProvider {
							require.ErrorIs(t, err, gorm.ErrRecordNotFound)
						} else {
							require.NoError(t, err)
						}
					}
				})
			}
		}
	}
}

func TestLocalAuthBootstrapLoginCompletesInOneSession(t *testing.T) {
	for _, passwordChange := range []bool{false, true} {
		t.Run(strconv.FormatBool(passwordChange), func(t *testing.T) {
			f := newLocalSetupFixture(t)
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
			// Both the bootstrap login and the setup login marker end here, because no sign-out follows.
			require.Len(t, result.Cookies(), 2)
			for i, name := range []string{bootstrap.ObotBootstrapCookie, auth.SetupLoginCookieName(true)} {
				require.Equal(t, name, result.Cookies()[i].Name)
				require.Equal(t, -1, result.Cookies()[i].MaxAge)
			}

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
		owner      bool
		// navigated marks a request that another page navigated the browser to, without a setup login.
		navigated bool
		// planted replaces the setup login cookie with an unprefixed one, as a sibling subdomain could set it.
		planted   bool
		wantError string
	}{
		{
			name:       "same-site tenant planted an unprefixed setup login cookie",
			provider:   system.LocalAuthProvider,
			configured: system.LocalAuthProvider,
			localUsers: 1,
			email:      "owner@example.com",
			navigated:  true,
			planted:    true,
			wantError:  "did not start a setup login",
		},
		{
			name:       "browser navigated here without a setup login",
			provider:   system.LocalAuthProvider,
			configured: system.LocalAuthProvider,
			localUsers: 1,
			email:      "owner@example.com",
			navigated:  true,
			wantError:  "did not start a setup login",
		},
		{
			name:       "external provider identity navigated here without a setup login",
			provider:   "google-auth-provider",
			configured: "google-auth-provider",
			localUsers: 1,
			email:      "owner@example.com",
			navigated:  true,
			wantError:  "did not start a setup login",
		},
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
			f := newLocalSetupFixture(t)
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
			if test.navigated {
				req.Request.Header.Del("Cookie")
			}
			if test.planted {
				// The fixture's server URL is https, where only the __Host- name counts.
				req.AddCookie(&http.Cookie{Name: auth.SetupLoginCookieName(false), Value: "setup-request"})
			}
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
