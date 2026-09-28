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

	_, err = gatewayClient.DisableUser(t.Context(), lifecycleTestProvider, user.ID, gatewaytypes.UserDisabledReasonSCIMInactive)
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

	_, err = gatewayClient.DisableUser(t.Context(), lifecycleTestProvider, owner.ID, gatewaytypes.UserDisabledReasonSCIMInactive)
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
	_, err := gatewayClient.DisableUser(t.Context(), lifecycleTestProvider, user.ID, gatewaytypes.UserDisabledReasonSCIMInactive)
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
