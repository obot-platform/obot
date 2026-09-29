package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	apiKeyLifecycleTestProvider = gatewayclient.AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "okta-auth-provider",
	}
)

func createAPIKeyLifecycleTestUser(t *testing.T, client *gatewayclient.Client, username string) *gatewaytypes.User {
	t.Helper()

	user, err := client.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		AuthProviderName:      apiKeyLifecycleTestProvider.Name,
		AuthProviderNamespace: apiKeyLifecycleTestProvider.Namespace,
		ProviderUsername:      username,
		ProviderUserID:        "00u-" + username,
		Email:                 username + "@example.com",
	}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return user
}

func apiKeyRequest(key string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	return req
}

func TestAPIKeyAuthenticatorReportsTheUsersStatus(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	user := createAPIKeyLifecycleTestUser(t, client, "alice")
	created, err := client.CreateAPIKey(ctx, user.ID, "cli", "", nil, gatewaytypes.APIKeyScopes{
		CanAccessAPI: true,
	})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}
	authenticator := NewAPIKeyAuthenticator(client, nil)

	assertStatus := func(want types2.UserStatus) {
		t.Helper()
		response, ok, err := authenticator.AuthenticateRequest(apiKeyRequest(created.Key))
		if err != nil || !ok {
			t.Fatalf("authenticate = %v, %v; want the key accepted for the admission check", ok, err)
		}
		if got, recorded := principal.UserStatus(response.User); !recorded || got != want {
			t.Fatalf("recorded status = %q, %v; want %q", got, recorded, want)
		}
	}

	assertStatus(types2.UserStatusActive)

	if _, err := client.DisableUser(ctx, apiKeyLifecycleTestProvider, user.ID, gatewaytypes.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	assertStatus(types2.UserStatusDisabled)

	if _, err := client.ReactivateUser(ctx, apiKeyLifecycleTestProvider, user.ID); err != nil {
		t.Fatalf("failed to reactivate user: %v", err)
	}
	assertStatus(types2.UserStatusActive)
}

func TestAPIKeyAuthenticatorDeniesAKeyThatOutlivedItsUser(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	user := createAPIKeyLifecycleTestUser(t, client, "bob")
	createAPIKeyLifecycleTestUser(t, client, "owner")
	created, err := client.CreateAPIKey(ctx, user.ID, "cli", "", nil, gatewaytypes.APIKeyScopes{
		CanAccessAPI: true,
	})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}
	if err := client.DeleteUser(ctx, fmt.Sprint(user.ID)); err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}

	_, ok, err := NewAPIKeyAuthenticator(client, nil).AuthenticateRequest(apiKeyRequest(created.Key))
	denied, isDenied := errors.AsType[*gatewayclient.UserAccessDeniedError](err)
	if ok || !isDenied || denied.Status != types2.UserStatusDeleted {
		t.Fatalf("authenticate = %v, %v; want the key denied for a deleted user", ok, err)
	}
}

func TestAPIKeyAuthenticatorFailsWhenItCannotReadTheKey(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	user := createAPIKeyLifecycleTestUser(t, client, "carol")
	created, err := client.CreateAPIKey(t.Context(), user.ID, "cli", "", nil, gatewaytypes.APIKeyScopes{
		CanAccessAPI: true,
	})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}
	authenticator := NewAPIKeyAuthenticator(client, nil)

	// An invalid key is declined, so that the rest of the chain can try the credential.
	response, ok, err := authenticator.AuthenticateRequest(apiKeyRequest(fmt.Sprintf("ok1-%d-%d-wrong", user.ID, created.ID)))
	if response != nil || ok || err != nil {
		t.Fatalf("authenticate an invalid key = %v, %v, %v; want it declined", response, ok, err)
	}

	// A key that could not be read fails the request instead of continuing to anonymous access.
	if err := client.Close(); err != nil {
		t.Fatalf("failed to close gateway client: %v", err)
	}
	_, ok, err = authenticator.AuthenticateRequest(apiKeyRequest(created.Key))
	if lookupErr, isLookup := errors.AsType[*gatewayclient.UserAccessLookupError](err); ok || !isLookup || lookupErr.UserID != user.ID {
		t.Fatalf("authenticate with an unreadable key = %v, %v; want a lookup failure for user %d", ok, err, user.ID)
	}
}

func TestHostedAgentKeyCarriesItsOwnersStatus(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	owner := createAPIKeyLifecycleTestUser(t, client, "dave")

	const instanceName = "hai1lifecycle"
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.HostedAgent{
			Name:      "agent",
			Namespace: system.DefaultNamespace,
		},
		&v1.HostedAgentInstance{
			Name:      instanceName,
			Namespace: system.DefaultNamespace,
			Spec: v1.HostedAgentInstanceSpec{
				UserID:          fmt.Sprint(owner.ID),
				HostedAgentName: "agent",
			},
		},
	).Build()
	created, err := client.CreateHostedAgentAPIKey(ctx, instanceName, owner.ID, "agent")
	if err != nil {
		t.Fatalf("failed to create hosted agent key: %v", err)
	}
	authenticator := NewAPIKeyAuthenticator(client, storage)

	assertOwnerStatus := func(want types2.UserStatus) {
		t.Helper()
		response, ok, err := authenticator.AuthenticateRequest(apiKeyRequest(created.Key))
		if err != nil || !ok {
			t.Fatalf("authenticate = %v, %v; want the agent key accepted for the admission check", ok, err)
		}
		if !principal.IsHostedAgent(response.User) {
			t.Fatalf("principal %q is not a hosted agent", response.User.GetUID())
		}
		if got, recorded := principal.UserStatus(response.User); !recorded || got != want {
			t.Fatalf("recorded owner status = %q, %v; want %q", got, recorded, want)
		}
	}

	assertOwnerStatus(types2.UserStatusActive)

	if _, err := client.DisableUser(ctx, apiKeyLifecycleTestProvider, owner.ID, gatewaytypes.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable owner: %v", err)
	}
	assertOwnerStatus(types2.UserStatusDisabled)
}

func TestAPIKeyWebhookDeniesInactiveUsers(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	user := createAPIKeyLifecycleTestUser(t, client, "erin")
	created, err := client.CreateAPIKey(ctx, user.ID, "mcp", "", nil, gatewaytypes.APIKeyScopes{
		MCPServerIDs: []string{"*"},
	})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	authenticate := func() apiKeyAuthResponse {
		t.Helper()
		apiContext, recorder := newTokenRequestAPIContext(t, client, http.MethodPost, "/api/api-keys/auth", apiKeyAuthRequest{
			ValidateOnly: true,
		}, nil)
		apiContext.Request.Header.Set("Authorization", "Bearer "+created.Key)
		if err := s.authenticateAPIKey(apiContext); err != nil {
			t.Fatalf("failed to authenticate API key: %v", err)
		}
		var response apiKeyAuthResponse
		decodeTokenRequestResponse(t, recorder, &response)
		return response
	}

	if response := authenticate(); !response.Allowed {
		t.Fatalf("webhook response for an active user = %+v, want allowed", response)
	}

	if _, err := client.DisableUser(ctx, apiKeyLifecycleTestProvider, user.ID, gatewaytypes.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if response := authenticate(); response.Allowed || response.Reason != "user is not active" {
		t.Fatalf("webhook response for a disabled user = %+v, want denied as not active", response)
	}
}
