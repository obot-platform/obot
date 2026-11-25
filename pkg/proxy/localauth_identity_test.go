package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auth"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/localauth"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type unlimitedEntitlements struct{}

func (unlimitedEntitlements) Entitlements(context.Context) (apitypes.Entitlements, error) {
	return apitypes.Entitlements{}, nil
}

// Setup records the local owner with localauth.Identity before they ever sign in, so the first request made with
// their session must resolve to that same user. This runs the real sign-in path: the local auth provider's
// /obot-get-state, the proxy's mapping of that state, and the UserDecorator that ensures the identity.
func TestLocalAuthIdentityMatchesSignedInIdentity(t *testing.T) {
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.UserDefaultRoleSetting{
			Name:      system.DefaultRoleSettingName,
			Namespace: system.DefaultNamespace,
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: apitypes.RoleBasic,
			},
		}).
		Build()
	c := gateway.New(t.Context(), db, storageClient, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.UpsertCredential(t.Context(), gwtypes.Credential{
		Context: system.LocalAuthProvider,
		Name:    system.LocalAuthProvider,
		Secrets: map[string]string{localauth.EmailDomainsEnvVar: "*"},
	}))

	provider, err := localauth.New(c, "https://obot.example.com", "https://obot.example.com")
	require.NoError(t, err)
	providerURL, err := provider.Start(t.Context())
	require.NoError(t, err)

	localUser, err := provider.CreateUser(t.Context(), "Owner@Example.com", "initial-local-password", false, true)
	require.NoError(t, err)

	// What setup records ahead of the first sign-in.
	owner, err := c.EnsureIdentityWithRole(t.Context(), localauth.Identity(localUser.Email), "", apitypes.RoleOwner, gateway.UserLimit{Unlimited: true})
	require.NoError(t, err)

	token, _, err := provider.StartSession(t.Context(), localUser.ID)
	require.NoError(t, err)

	p, err := newProxy(system.DefaultNamespace, localauth.ProviderName, providerURL.String(), "")
	require.NoError(t, err)
	var signedIn user.Info
	proxyAuth := authenticator.RequestFunc(func(req *http.Request) (*authenticator.Response, bool, error) {
		resp, ok, err := p.authenticateRequest(req)
		if ok {
			signedIn = resp.User
		}
		return resp, ok, err
	})
	decorator := gateway.NewUserDecorator(proxyAuth, c, unlimitedEntitlements{})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.ObotAccessTokenCookie, Value: token})
	resp, ok, err := decorator.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)

	// The identity the proxy builds from the provider's state is the one setup recorded.
	want := localauth.Identity(localUser.Email)
	require.Equal(t, want.ProviderUserID, signedIn.GetUID())
	require.Equal(t, want.ProviderUsername, signedIn.GetName())
	require.Equal(t, []string{want.Email}, signedIn.GetExtra()["email"])
	require.Equal(t, []string{want.AuthProviderName}, signedIn.GetExtra()["auth_provider_name"])
	require.Equal(t, []string{want.AuthProviderNamespace}, signedIn.GetExtra()["auth_provider_namespace"])

	// So the request resolves to the Owner rather than a new user.
	require.Equal(t, strconv.Itoa(int(owner.ID)), resp.User.GetUID())
	require.Contains(t, resp.User.GetGroups(), apitypes.GroupOwner)
}
