package persistent

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	lifecycleTestProvider = client.AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "okta-auth-provider",
	}
)

func newLifecycleTestTokenService(t *testing.T) (*TokenService, *client.Client) {
	t.Helper()

	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.UserDefaultRoleSetting{
		Namespace: system.DefaultNamespace,
		Name:      system.DefaultRoleSettingName,
		Spec: v1.UserDefaultRoleSettingSpec{
			Role: types.RoleBasic,
		},
	}).Build()
	gatewayClient := client.New(t.Context(), db, storage, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, false)
	t.Cleanup(func() { _ = gatewayClient.Close() })

	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	tokenService, err := NewTokenService(testServerURL, gatewayClient)
	require.NoError(t, err)
	require.NoError(t, tokenService.replaceKey(t.Context(), privateKey))

	return tokenService, gatewayClient
}

func createLifecycleTestUser(t *testing.T, gatewayClient *client.Client, username string, role types.Role) *gatewaytypes.User {
	t.Helper()

	user, err := gatewayClient.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		AuthProviderName:      lifecycleTestProvider.Name,
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		ProviderUsername:      username,
		ProviderUserID:        "00u-" + username,
		Email:                 username + "@example.com",
	}, "", role, client.UserLimit{Unlimited: true})
	require.NoError(t, err)
	return user
}

// deactivateThroughSCIM has the identity provider of provider deactivate the user with the native ID nativeID and the
// email email through SCIM, as it does in production, setting up the provider's SCIM connection first if it has none.
// A user who signed in with that ID is bound to their account, and disabled.
func deactivateThroughSCIM(t *testing.T, c *client.Client, provider client.AuthProviderRef, nativeID, email string) error {
	t.Helper()

	conn, err := c.SCIMConnectionForAuthProvider(t.Context(), provider.Namespace, provider.Name)
	if err != nil {
		return err
	}
	if conn == nil {
		if conn, _, err = c.CreateSCIMConnection(t.Context(), client.CreateSCIMConnectionOptions{
			AuthProviderNamespace: provider.Namespace,
			AuthProviderName:      provider.Name,
			GroupIDPrefix:         "okta/",
			Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
		}); err != nil {
			return err
		}
	}
	_, err = c.CreateSCIMUser(t.Context(), conn, client.SCIMUserInput{
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
	}, client.SCIMUserCreateOptions{
		UserLimit: client.UserLimit{
			Unlimited: true,
		},
		DefaultRole: types.RoleBasic,
	})
	return err
}

func authenticateToken(t *testing.T, tokenService *TokenService, tokenContext TokenContext) (*http.Request, string) {
	t.Helper()

	tokenContext.Audience = testServerURL + "/mcp-connect/server-id"
	tokenContext.IssuedAt = NewTime(time.Now().Add(-time.Minute))
	tokenContext.ExpiresAt = NewTime(time.Now().Add(time.Hour))
	_, token, err := tokenService.NewToken(t.Context(), tokenContext)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, testServerURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req, token
}

func TestAuthenticateRequestRecomputesRolesAndReportsTheUsersStatus(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "alice", types.RoleAdmin)
	tokenGroups := []string{types.GroupMCP, types.GroupAuthenticated}

	req, _ := authenticateToken(t, tokenService, TokenContext{
		UserID:     fmt.Sprint(user.ID),
		UserGroups: tokenGroups,
	})

	response, ok, err := tokenService.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, tokenGroups, response.User.GetGroups())
	// The role groups come from the user's current role, not from the token.
	assert.Equal(t, types.RoleAdmin.RoleGroups(), response.User.GetExtra()["obot_groups"])
	status, recorded := principal.UserStatus(response.User)
	assert.True(t, recorded)
	assert.Equal(t, types.UserStatusActive, status)

	err = deactivateThroughSCIM(t, gatewayClient, lifecycleTestProvider, "00u-"+user.Username, user.Email)
	require.NoError(t, err)

	response, ok, err = tokenService.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	status, recorded = principal.UserStatus(response.User)
	assert.True(t, recorded)
	assert.Equal(t, types.UserStatusDisabled, status)
}

func TestAuthenticateRequestReportsAMissingUserAsDeleted(t *testing.T) {
	tokenService, _ := newLifecycleTestTokenService(t)

	req, _ := authenticateToken(t, tokenService, TokenContext{
		UserID:     "4242",
		UserGroups: []string{types.GroupMCP, types.GroupAuthenticated},
	})

	response, ok, err := tokenService.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	status, recorded := principal.UserStatus(response.User)
	assert.True(t, recorded)
	assert.Equal(t, types.UserStatusDeleted, status)
}

func TestAuthenticateRequestFailsWhenItCannotReadTheUser(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "bob", types.RoleBasic)
	req, _ := authenticateToken(t, tokenService, TokenContext{
		UserID:     fmt.Sprint(user.ID),
		UserGroups: []string{types.GroupMCP, types.GroupAuthenticated},
	})

	require.NoError(t, gatewayClient.Close())

	response, ok, err := tokenService.AuthenticateRequest(req)
	assert.Nil(t, response)
	assert.False(t, ok)
	lookupErr, isLookup := errors.AsType[*client.UserAccessLookupError](err)
	require.True(t, isLookup, "error = %v, want a lookup failure", err)
	assert.Equal(t, user.ID, lookupErr.UserID)
}

func TestAuthenticateRequestReportsAHostedAgentOwnersStatus(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	owner := createLifecycleTestUser(t, gatewayClient, "carol", types.RoleAdmin)
	tokenGroups := []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}

	req, _ := authenticateToken(t, tokenService, TokenContext{
		UserID:             "hosted-agent:hai1abc",
		UserGroups:         tokenGroups,
		HostedAgentOwnerID: fmt.Sprint(owner.ID),
	})

	response, ok, err := tokenService.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "hosted-agent:hai1abc", response.User.GetUID())
	// The agent acts for its owner but carries none of the owner's groups.
	assert.Equal(t, tokenGroups, response.User.GetExtra()["obot_groups"])
	assert.Empty(t, response.User.GetExtra()["auth_provider_groups"])
	status, recorded := principal.UserStatus(response.User)
	assert.True(t, recorded)
	assert.Equal(t, types.UserStatusActive, status)

	err = deactivateThroughSCIM(t, gatewayClient, lifecycleTestProvider, "00u-"+owner.Username, owner.Email)
	require.NoError(t, err)

	response, ok, err = tokenService.AuthenticateRequest(req)
	require.NoError(t, err)
	require.True(t, ok)
	status, recorded = principal.UserStatus(response.User)
	assert.True(t, recorded)
	assert.Equal(t, types.UserStatusDisabled, status)
}

func TestNewTokenRefusesInactiveUsers(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "dave", types.RoleBasic)
	err := deactivateThroughSCIM(t, gatewayClient, lifecycleTestProvider, "00u-"+user.Username, user.Email)
	require.NoError(t, err)

	for name, tokenContext := range map[string]TokenContext{
		"user": {
			UserID: fmt.Sprint(user.ID),
		},
		"hosted agent of the user": {
			UserID:             "hosted-agent:hai1abc",
			HostedAgentOwnerID: fmt.Sprint(user.ID),
		},
	} {
		t.Run(name, func(t *testing.T) {
			tokenContext.Audience = testServerURL
			_, _, err := tokenService.NewToken(t.Context(), tokenContext)
			denied, isDenied := errors.AsType[*client.UserAccessDeniedError](err)
			require.True(t, isDenied, "error = %v, want a denial", err)
			assert.Equal(t, types.UserStatusDisabled, denied.Status)
		})
	}
}

// TestNewTokenTrustsTheAdmittedPrincipal covers a token minted while handling a request: the request's authenticator
// read the status of the user it acts for, so minting does not read it again. Disabling the user in the database
// after that read shows that minting trusted the request's principal.
func TestNewTokenTrustsTheAdmittedPrincipal(t *testing.T) {
	tokenService, gatewayClient := newLifecycleTestTokenService(t)
	user := createLifecycleTestUser(t, gatewayClient, "erin", types.RoleBasic)
	err := deactivateThroughSCIM(t, gatewayClient, lifecycleTestProvider, "00u-"+user.Username, user.Email)
	require.NoError(t, err)

	extra := map[string][]string{}
	principal.RecordUserStatus(extra, types.UserStatusActive)
	admitted := principal.WithAdmittedPrincipal(t.Context(), &kuser.DefaultInfo{
		UID:   fmt.Sprint(user.ID),
		Extra: extra,
	})

	tokenContext := TokenContext{
		Audience: testServerURL,
		UserID:   fmt.Sprint(user.ID),
	}
	_, _, err = tokenService.NewToken(admitted, tokenContext)
	require.NoError(t, err)

	// A token for any other user is still checked.
	other := createLifecycleTestUser(t, gatewayClient, "frank", types.RoleBasic)
	err = deactivateThroughSCIM(t, gatewayClient, lifecycleTestProvider, "00u-"+other.Username, other.Email)
	require.NoError(t, err)
	tokenContext.UserID = fmt.Sprint(other.ID)
	_, _, err = tokenService.NewToken(admitted, tokenContext)
	_, isDenied := errors.AsType[*client.UserAccessDeniedError](err)
	require.True(t, isDenied, "error = %v, want a denial", err)
}
