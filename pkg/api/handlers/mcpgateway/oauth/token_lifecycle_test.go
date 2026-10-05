package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	oauthLifecycleTestProvider = gatewayclient.AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "okta-auth-provider",
	}
	// Signing in reads the default role for new users.
	oauthLifecycleTestDefaultRole = &v1.UserDefaultRoleSetting{
		Namespace: system.DefaultNamespace,
		Name:      system.DefaultRoleSettingName,
		Spec: v1.UserDefaultRoleSettingSpec{
			Role: types.RoleBasic,
		},
	}
)

// disableOnTokenCreateStorage disables a user as soon as an OAuth token is created for them, as a disable that
// commits while the token is being issued would, after its delivery already looked for the user's refresh tokens.
type disableOnTokenCreateStorage struct {
	storage.Client
	t             *testing.T
	gatewayClient *gatewayclient.Client
	// nativeID and email identify the user to the identity provider.
	nativeID string
	email    string
}

func (s *disableOnTokenCreateStorage) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if err := s.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}
	if _, ok := obj.(*v1.OAuthToken); ok {
		if err := deactivateThroughSCIM(s.t, s.gatewayClient, oauthLifecycleTestProvider, s.nativeID, s.email); err != nil {
			return err
		}
	}
	return nil
}

// deactivateThroughSCIM has the identity provider of provider deactivate the user with the native ID nativeID and the
// email email through SCIM, as it does in production, setting up the provider's SCIM connection first if it has none.
// A user who signed in with that ID is bound to their account, and disabled.
func deactivateThroughSCIM(t *testing.T, c *gatewayclient.Client, provider gatewayclient.AuthProviderRef, nativeID, email string) error {
	t.Helper()

	conn, err := c.SCIMConnectionForAuthProvider(t.Context(), provider.Namespace, provider.Name)
	if err != nil {
		return err
	}
	if conn == nil {
		if conn, _, err = c.CreateSCIMConnection(t.Context(), gatewayclient.CreateSCIMConnectionOptions{
			AuthProviderNamespace: provider.Namespace,
			AuthProviderName:      provider.Name,
			GroupIDPrefix:         "okta/",
			Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
		}); err != nil {
			return err
		}
	}
	_, err = c.CreateSCIMUser(t.Context(), conn, gatewayclient.SCIMUserInput{
		UserName:   email,
		ExternalID: nativeID,
		Active:     new(false),
		Profile: gatewaytypes.SCIMUserProfile{
			Emails: []gatewaytypes.SCIMMultiValue{
				{
					Value:   email,
					Primary: true,
				},
			},
		},
	}, gatewayclient.SCIMUserCreateOptions{
		UserLimit: gatewayclient.UserLimit{
			Unlimited: true,
		},
		DefaultRole: types.RoleBasic,
	})
	return err
}

// createDisabledOAuthTestUser creates a user of the Okta provider and disables them. It returns once the disable
// event has been delivered, which deletes the refresh tokens the user held, so later tokens exercise the checks at
// exchange and refresh rather than that cleanup.
func createDisabledOAuthTestUser(t *testing.T, storage kclient.Client, gatewayClient *gatewayclient.Client) *gatewaytypes.User {
	t.Helper()

	user, err := gatewayClient.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		AuthProviderName:      oauthLifecycleTestProvider.Name,
		AuthProviderNamespace: oauthLifecycleTestProvider.Namespace,
		ProviderUsername:      "disabled",
		ProviderUserID:        "00u-disabled",
		Email:                 "disabled@example.com",
	}, "", types.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	require.NoError(t, err)

	heldToken := &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      "held-before-disable",
		Spec: v1.OAuthTokenSpec{
			ClientID: "oauth-client",
			UserID:   user.ID,
		},
	}
	require.NoError(t, storage.Create(t.Context(), heldToken))

	require.NoError(t, deactivateThroughSCIM(t, gatewayClient, oauthLifecycleTestProvider, "00u-disabled", user.Email))

	require.Eventually(t, func() bool {
		return apierrors.IsNotFound(storage.Get(t.Context(), kclient.ObjectKeyFromObject(heldToken), &v1.OAuthToken{}))
	}, 5*time.Second, 10*time.Millisecond, "the disable event did not delete the refresh token the user held")

	return user
}

func requireInvalidUserGrant(t *testing.T, err error) {
	t.Helper()

	var errHTTP *types.ErrHTTP
	require.ErrorAs(t, err, &errHTTP)
	assert.Equal(t, http.StatusBadRequest, errHTTP.Code)
	var oauthErr oauthError
	require.NoError(t, json.Unmarshal([]byte(errHTTP.Message), &oauthErr))
	assert.Equal(t, ErrInvalidGrant, oauthErr.Code)
	assert.Equal(t, "Obot: invalid user", oauthErr.Description)
}

func TestDoAuthorizationCodeRefusesADisabledUser(t *testing.T) {
	const (
		clientName = "oauth-client"
		code       = "authorization-code"
	)

	authRequest := &v1.OAuthAuthRequest{
		Namespace: system.DefaultNamespace,
		Name:      "oauth-request",
		Spec: v1.OAuthAuthRequestSpec{
			ClientID:       clientName,
			Resource:       "https://obot.example.com/mcp-connect/server",
			HashedAuthCode: fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
			MCPID:          system.SystemMCPServerPrefix + "test",
		},
	}
	storage, gatewayClient, tokenService := newOAuthTokenTestServices(t, oauthLifecycleTestDefaultRole.DeepCopy())
	// The user approved the request, then was disabled before the client exchanged the code.
	authRequest.Spec.UserID = createDisabledOAuthTestUser(t, storage, gatewayClient).ID
	require.NoError(t, storage.Create(t.Context(), authRequest))

	recorder := httptest.NewRecorder()
	h := &handler{tokenService: tokenService}
	err := h.doAuthorizationCode(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}, v1.OAuthClient{Namespace: system.DefaultNamespace, Name: clientName}, code, "")
	requireInvalidUserGrant(t, err)

	var tokens v1.OAuthTokenList
	require.NoError(t, storage.List(t.Context(), &tokens))
	assert.Empty(t, tokens.Items, "a refresh token was issued to a disabled user")
}

func TestDoRefreshTokenRefusesAndConsumesADisabledUsersToken(t *testing.T) {
	const (
		clientName   = "oauth-client"
		refreshToken = "disabled-user-refresh-token"
	)

	storage, gatewayClient, tokenService := newOAuthTokenTestServices(t, oauthLifecycleTestDefaultRole.DeepCopy())
	user := createDisabledOAuthTestUser(t, storage, gatewayClient)
	tokenName := fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))
	require.NoError(t, storage.Create(t.Context(), &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      tokenName,
		Spec: v1.OAuthTokenSpec{
			ClientID: clientName,
			Resource: "https://obot.example.com/mcp-connect/server",
			UserID:   user.ID,
			MCPID:    system.SystemMCPServerPrefix + "test",
		},
	}))

	h := &handler{baseURL: "https://obot.example.com", tokenService: tokenService}
	err := h.doRefreshToken(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}, v1.OAuthClient{Namespace: system.DefaultNamespace, Name: clientName}, refreshToken)
	requireInvalidUserGrant(t, err)

	// The refresh token is consumed, so it stays revoked if the user is reactivated.
	err = storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: tokenName}, &v1.OAuthToken{})
	assert.True(t, apierrors.IsNotFound(err), "expected the refresh token to be consumed, got %v", err)
}

func TestRefreshTokensIssuedWhileTheUserIsDisabledAreRevoked(t *testing.T) {
	const (
		clientName   = "oauth-client"
		code         = "authorization-code"
		refreshToken = "active-user-refresh-token"
	)

	for _, flow := range []string{"authorization code", "refresh token"} {
		t.Run(flow, func(t *testing.T) {
			baseStorage, gatewayClient, tokenService := newOAuthTokenTestServices(t, oauthLifecycleTestDefaultRole.DeepCopy())
			user, err := gatewayClient.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
				AuthProviderName:      oauthLifecycleTestProvider.Name,
				AuthProviderNamespace: oauthLifecycleTestProvider.Namespace,
				ProviderUsername:      "racing",
				ProviderUserID:        "00u-racing",
				Email:                 "racing@example.com",
			}, "", types.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
			require.NoError(t, err)

			resource := "https://obot.example.com/mcp-connect/server"
			mcpID := system.SystemMCPServerPrefix + "test"
			if flow == "authorization code" {
				require.NoError(t, baseStorage.Create(t.Context(), &v1.OAuthAuthRequest{
					Namespace: system.DefaultNamespace,
					Name:      "oauth-request",
					Spec: v1.OAuthAuthRequestSpec{
						ClientID:       clientName,
						Resource:       resource,
						HashedAuthCode: fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
						UserID:         user.ID,
						MCPID:          mcpID,
					},
				}))
			} else {
				require.NoError(t, baseStorage.Create(t.Context(), &v1.OAuthToken{
					Namespace: system.DefaultNamespace,
					Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))),
					Spec: v1.OAuthTokenSpec{
						ClientID: clientName,
						Resource: resource,
						UserID:   user.ID,
						MCPID:    mcpID,
					},
				}))
			}

			req := api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
				Storage: &disableOnTokenCreateStorage{
					Client:        baseStorage,
					t:             t,
					gatewayClient: gatewayClient,
					nativeID:      "00u-racing",
					email:         user.Email,
				},
				GatewayClient: gatewayClient,
			}
			oauthClient := v1.OAuthClient{Namespace: system.DefaultNamespace, Name: clientName}
			h := &handler{baseURL: "https://obot.example.com", tokenService: tokenService}
			if flow == "authorization code" {
				err = h.doAuthorizationCode(req, oauthClient, code, "")
			} else {
				err = h.doRefreshToken(req, oauthClient, refreshToken)
			}
			requireInvalidUserGrant(t, err)

			var tokens v1.OAuthTokenList
			require.NoError(t, baseStorage.List(t.Context(), &tokens))
			for _, token := range tokens.Items {
				assert.NotEqual(t, user.ID, token.Spec.UserID, "refresh token %s of the disabled user survived", token.Name)
			}
		})
	}
}
