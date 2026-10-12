package server

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authn"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	sessionTestHeader = "X-Test-Session"
)

// sessionAuthenticator stands in for the auth provider proxy: it returns the provider's principal for a test header
// naming the provider and one of its users.
type sessionAuthenticator struct {
	provider gatewayclient.AuthProviderRef
}

type credentialRequest struct {
	name    string
	request func() *http.Request
}

type userLimitUnlimited struct{}

func (userLimitUnlimited) UserLimit(context.Context) (gatewayclient.UserLimit, error) {
	return gatewayclient.UserLimit{Unlimited: true}, nil
}

func (s sessionAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	providerUserID, ok := strings.CutPrefix(req.Header.Get(sessionTestHeader), s.provider.Name+"/")
	if !ok {
		return nil, false, nil
	}
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name: providerUserID,
			UID:  providerUserID,
			Extra: map[string][]string{
				"email":                   {providerUserID + "@example.com"},
				"auth_provider_name":      {s.provider.Name},
				"auth_provider_namespace": {s.provider.Namespace},
			},
		},
	}, true, nil
}

// TestLifecycleExitGate checks the lifecycle guarantees across the authenticator chain as the server composes it: a
// user of the provider can be disabled, is then denied on every credential path, and is reactivated with the same
// ID and data; a deleted user stays deleted; a user of another provider is unaffected; and a principal that acts
// for Obot itself still works.
func TestLifecycleExitGate(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	provider := apiKeyLifecycleTestProvider
	localProvider := gatewayclient.AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      system.LocalAuthProvider,
	}

	signIn := func(provider gatewayclient.AuthProviderRef, providerUserID string) *gatewaytypes.User {
		t.Helper()
		u, err := client.EnsureIdentityWithRole(ctx, &gatewaytypes.Identity{
			AuthProviderName:      provider.Name,
			AuthProviderNamespace: provider.Namespace,
			ProviderUsername:      providerUserID,
			ProviderUserID:        providerUserID,
			Email:                 providerUserID + "@example.com",
		}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
		if err != nil {
			t.Fatalf("failed to sign in %s: %v", providerUserID, err)
		}
		return u
	}
	oktaUser := signIn(provider, "00u-okta")
	localUser := signIn(localProvider, "local")

	// Persistent tokens, such as MCP OAuth access tokens.
	_, jwk, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: system.JWKCredentialContext,
		Name:    system.JWKCredentialContext,
		Secrets: map[string]string{
			"JWK_KEY": base64.StdEncoding.EncodeToString(jwk),
		},
	}); err != nil {
		t.Fatalf("failed to store JWK: %v", err)
	}
	tokenService, err := persistent.NewTokenService("https://obot.example.com", client)
	if err != nil {
		t.Fatal(err)
	}

	const instanceName = "hai1gate"
	agents := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.HostedAgent{
			Name:      "agent",
			Namespace: system.DefaultNamespace,
		},
		&v1.HostedAgentInstance{
			Name:      instanceName,
			Namespace: system.DefaultNamespace,
			Spec: v1.HostedAgentInstanceSpec{
				UserID:          fmt.Sprint(oktaUser.ID),
				HostedAgentName: "agent",
			},
		},
	).Build()

	chain := authn.NewAdmissionCheck(union.NewFailOnError(
		union.New(
			gatewayclient.NewUserDecorator(sessionAuthenticator{provider: provider}, client, userLimitUnlimited{}),
			gatewayclient.NewUserDecorator(sessionAuthenticator{provider: localProvider}, client, userLimitUnlimited{}),
			NewAPIKeyAuthenticator(client, agents),
			tokenService,
		),
		authn.Anonymous{},
	))

	credentials := func(u *gatewaytypes.User, provider gatewayclient.AuthProviderRef, providerUserID string) []credentialRequest {
		t.Helper()
		apiKey, err := client.CreateAPIKey(ctx, u.ID, "cli", "", nil, gatewaytypes.APIKeyScopes{
			CanAccessAPI: true,
		})
		if err != nil {
			t.Fatalf("failed to create API key: %v", err)
		}
		agentKey, err := client.CreateHostedAgentAPIKey(ctx, instanceName, u.ID, "agent")
		if err != nil {
			t.Fatalf("failed to create hosted agent key: %v", err)
		}
		now := time.Now().Add(-time.Second)
		_, mcpToken, err := tokenService.NewToken(ctx, persistent.TokenContext{
			Audience:   "https://obot.example.com/mcp-connect/server",
			IssuedAt:   persistent.NewTime(now),
			ExpiresAt:  persistent.NewTime(now.Add(time.Hour)),
			UserID:     fmt.Sprint(u.ID),
			UserGroups: []string{types2.GroupMCP, types2.GroupAuthenticated},
		})
		if err != nil {
			t.Fatalf("failed to issue token: %v", err)
		}

		bearer := func(token string) func() *http.Request {
			return func() *http.Request {
				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/me", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			}
		}
		return []credentialRequest{
			{
				name: "browser session",
				request: func() *http.Request {
					req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/me", nil)
					req.Header.Set(sessionTestHeader, provider.Name+"/"+providerUserID)
					return req
				},
			},
			{
				name:    "personal API key",
				request: bearer(apiKey.Key),
			},
			{
				name:    "hosted agent key",
				request: bearer(agentKey.Key),
			},
			{
				name:    "persistent token",
				request: bearer(mcpToken),
			},
		}
	}

	authenticate := func(req *http.Request) (user.Info, error) {
		t.Helper()
		resp, ok, err := chain.AuthenticateRequest(req)
		if err != nil {
			return nil, err
		}
		if !ok {
			t.Fatal("the chain declined a request, which the anonymous authenticator must prevent")
		}
		return resp.User, nil
	}
	assertAdmitted := func(requests []credentialRequest, wantUID uint) {
		t.Helper()
		for _, credential := range requests {
			u, err := authenticate(credential.request())
			if err != nil {
				t.Errorf("%s: %v, want admitted", credential.name, err)
				continue
			}
			if credential.name != "hosted agent key" && u.GetUID() != fmt.Sprint(wantUID) {
				t.Errorf("%s: authenticated as %q, want user %d", credential.name, u.GetUID(), wantUID)
			}
		}
	}
	assertDenied := func(requests []credentialRequest, wantStatus types2.UserStatus) {
		t.Helper()
		for _, credential := range requests {
			_, err := authenticate(credential.request())
			if denied, ok := findDenied(err); !ok || denied.Status != wantStatus {
				t.Errorf("%s: %v, want denied as %s", credential.name, err, wantStatus)
			}
		}
	}

	oktaCredentials := credentials(oktaUser, provider, "00u-okta")
	localCredentials := credentials(localUser, localProvider, "local")
	assertAdmitted(oktaCredentials, oktaUser.ID)
	assertAdmitted(localCredentials, localUser.ID)

	// Okta deactivates its user through SCIM. The local-auth user is outside the provider, and unaffected.
	provisioned := provisionThroughSCIM(t, client, "00u-okta", oktaUser.Email, false)
	if provisioned.UserID != oktaUser.ID {
		t.Fatalf("SCIM bound user %d, want %d", provisioned.UserID, oktaUser.ID)
	}
	assertDenied(oktaCredentials, types2.UserStatusDisabled)
	assertAdmitted(localCredentials, localUser.ID)

	if err := setActiveThroughSCIM(t, client, provisioned.ID, true); err != nil {
		t.Fatalf("failed to reactivate the Okta user: %v", err)
	}
	assertAdmitted(oktaCredentials, oktaUser.ID)

	// A principal that acts for Obot itself is unaffected.
	if u, err := authenticate(httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/me", nil)); err != nil || u.GetName() != "anonymous" {
		t.Fatalf("anonymous request = %v, %v; want admitted as anonymous", u, err)
	}

	// A deleted user stays deleted: every credential is denied, and the user cannot be reactivated. Okta deactivates
	// the user first, because a user it still provisions cannot be deleted.
	if err := setActiveThroughSCIM(t, client, provisioned.ID, false); err != nil {
		t.Fatalf("failed to deactivate the Okta user: %v", err)
	}
	if err := client.DeleteUser(ctx, fmt.Sprint(oktaUser.ID)); err != nil {
		t.Fatalf("failed to delete the Okta user: %v", err)
	}
	assertDenied(oktaCredentials[1:], types2.UserStatusDeleted)
	if err := setActiveThroughSCIM(t, client, provisioned.ID, true); err == nil {
		t.Fatal("reactivated a deleted user")
	}
	// Signing in again creates a new, empty account, as it does today.
	if again := signIn(provider, "00u-okta"); again.ID == oktaUser.ID {
		t.Fatalf("signing in after deletion reused user %d", oktaUser.ID)
	}
}

// findDenied finds a denial in an error from the authenticator chain, whose unions aggregate their members' errors.
func findDenied(err error) (*gatewayclient.UserAccessDeniedError, bool) {
	if denied, ok := errors.AsType[*gatewayclient.UserAccessDeniedError](err); ok {
		return denied, true
	}
	if aggregate, ok := errors.AsType[utilerrors.Aggregate](err); ok {
		for _, err := range aggregate.Errors() {
			if denied, ok := findDenied(err); ok {
				return denied, true
			}
		}
	}
	return nil, false
}
