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
	"github.com/obot-platform/obot/pkg/system"
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

// oktaSCIMConnection returns the SCIM connection of the Okta provider, setting it up on first use.
func oktaSCIMConnection(t *testing.T, client *gatewayclient.Client) *gatewaytypes.SCIMConnection {
	t.Helper()

	conn, err := client.SCIMConnectionForAuthProvider(t.Context(), apiKeyLifecycleTestProvider.Namespace, apiKeyLifecycleTestProvider.Name)
	if err != nil {
		t.Fatal(err)
	}
	if conn != nil {
		return conn
	}
	conn, _, err = client.CreateSCIMConnection(t.Context(), gatewayclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: apiKeyLifecycleTestProvider.Namespace,
		AuthProviderName:      apiKeyLifecycleTestProvider.Name,
		GroupIDPrefix:         "okta/",
		Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
	})
	if err != nil {
		t.Fatalf("failed to set up SCIM: %v", err)
	}
	return conn
}

// provisionThroughSCIM has Okta provision the user with the native ID nativeID and the email email through SCIM,
// active or deactivated, as it does in production. A user who signed in with that ID is bound to their account.
func provisionThroughSCIM(t *testing.T, client *gatewayclient.Client, nativeID, email string, active bool) *gatewayclient.SCIMUser {
	t.Helper()

	user, err := client.CreateSCIMUser(t.Context(), oktaSCIMConnection(t, client), gatewayclient.SCIMUserInput{
		UserName:   email,
		ExternalID: nativeID,
		Active:     &active,
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
		DefaultRole: types2.RoleBasic,
	})
	if err != nil {
		t.Fatalf("failed to provision %s through SCIM: %v", nativeID, err)
	}
	return user
}

// setActiveThroughSCIM has Okta activate or deactivate the user it provisioned as scimUserID through SCIM.
func setActiveThroughSCIM(t *testing.T, client *gatewayclient.Client, scimUserID string, active bool) error {
	t.Helper()

	_, err := client.UpdateSCIMUser(t.Context(), oktaSCIMConnection(t, client), scimUserID, func(current gatewayclient.SCIMUser) (gatewayclient.SCIMUserInput, error) {
		return gatewayclient.SCIMUserInput{
			UserName:   current.UserName,
			ExternalID: current.ExternalID,
			Active:     &active,
			Profile:    current.Profile,
		}, nil
	})
	return err
}

func apiKeyRequest(key string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	return req
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

	provisionThroughSCIM(t, client, "00u-erin", user.Email, false)
	if response := authenticate(); response.Allowed || response.Reason != "user is not active" {
		t.Fatalf("webhook response for a disabled user = %+v, want denied as not active", response)
	}
}
