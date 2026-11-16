package server

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/principal"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	noAuthTestLLMPath = "/api/llm-proxy/openai/v1/chat/completions"
)

// noAuthChain composes the authenticator chain of a server that does not authenticate callers, as pkg/services does,
// from the credentials that chain honors that need no other service: the user decorator of the gateway's token
// reviewer, the token service, and devices' credentials, all ended by WithNoAuth inside the admission check.
func noAuthChain(t *testing.T, s *Server, client *gatewayclient.Client) (*authn.AdmissionCheck, *persistent.TokenService) {
	t.Helper()

	tokenService := newChainTokenService(t, client)
	var credentials authenticator.Request = gatewayclient.NewUserDecorator(NewGatewayTokenReviewer(client, s.dispatcher), client, userLimitUnlimited{})
	credentials = union.New(credentials, tokenService)
	credentials = union.New(credentials, NewDeviceEnrollmentAuthenticator(client))
	credentials = union.New(credentials, NewDeviceAuthenticator(client))
	return authn.NewAdmissionCheck(WithNoAuth(credentials, client, tokenService), nil), tokenService
}

// noAuthTestRequest returns a request to path with headers set.
func noAuthTestRequest(t *testing.T, path string, headers map[string]string) *http.Request {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	return req
}

// authenticateNoAuth runs chain on req. The chain never declines: it ends with the no auth authenticator.
func authenticateNoAuth(t *testing.T, chain *authn.AdmissionCheck, req *http.Request) (user.Info, error) {
	t.Helper()

	response, ok, err := chain.AuthenticateRequest(req)
	if err != nil {
		return nil, err
	}
	if !ok || response == nil || response.User == nil {
		t.Fatalf("the chain declined a request, which the no auth authenticator must prevent")
	}
	return response.User, nil
}

// requireNobody fails unless info is the "nobody" owner that authn.NoAuth authenticates every other request as.
func requireNobody(t *testing.T, info user.Info, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("authenticate = %v; want the nobody owner", err)
	}
	if info.GetName() != "nobody" || principal.ClaimsAgent(info) {
		t.Fatalf("authenticated as %q (UID %q); want the nobody owner", info.GetName(), info.GetUID())
	}
}

// TestNoAuthChainAuthenticatesAgentKeysAsTheAgent checks that without authentication, an agent's API key in any header
// the API key authenticator reads authenticates as its agent, never as the nobody owner, and that the agent is held to
// its allowlist and its key's scopes: it reaches the LLM proxy its key allows, and no admin route.
func TestNoAuthChainAuthenticatesAgentKeysAsTheAgent(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	chain, _ := noAuthChain(t, s, client)
	agent, err := client.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.CreateAgentAPIKey(t.Context(), agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "ci", CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	authorizer := authz.NewAuthorizer(nil, nil, nil, false, nil, nil, nil)

	for _, tt := range []struct {
		name    string
		path    string
		headers map[string]string
	}{
		{
			name: "Authorization",
			path: noAuthTestLLMPath,
			headers: map[string]string{
				"Authorization": "Bearer " + key.Key,
			},
		},
		{
			name: "X-API-Key",
			path: noAuthTestLLMPath,
			headers: map[string]string{
				"X-API-Key": key.Key,
			},
		},
		{
			name: "X-Obot-Api-Key on the LLM proxy, beside another credential",
			path: noAuthTestLLMPath,
			headers: map[string]string{
				"Authorization":  "Bearer sk-ant-oat01-subscription",
				HeaderObotAPIKey: key.Key,
			},
		},
		{
			name: "Authorization on an admin route",
			path: "/api/agents",
			headers: map[string]string{
				"Authorization": "Bearer " + key.Key,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := noAuthTestRequest(t, tt.path, tt.headers)
			info, err := authenticateNoAuth(t, chain, req)
			if err != nil {
				t.Fatalf("authenticate = %v; want the agent", err)
			}
			if got, ok := principal.FromUserInfo(info); !ok || got != principal.Agent(agent.ID) {
				t.Fatalf("authenticated as %#v, %v (name %q); want agent %d", got, ok, info.GetName(), agent.ID)
			}
			if attribution, ok := principal.APIKeyAttributionFromUser(info); !ok || attribution.ID != key.ID {
				t.Fatalf("attribution = %#v, %v; want key %d", attribution, ok, key.ID)
			}

			if got := authorizer.Authorize(httptest.NewRequest(http.MethodPost, noAuthTestLLMPath, nil), info); !got {
				t.Fatal("the agent was refused the LLM proxy its key allows")
			}
			for _, route := range []struct {
				method string
				path   string
			}{
				{
					method: http.MethodGet,
					path:   "/api/agents",
				},
				{
					method: http.MethodPost,
					path:   "/api/agents",
				},
				{
					method: http.MethodGet,
					path:   "/api/users",
				},
				{
					method: http.MethodGet,
					path:   "/api/me",
				},
				{
					method: http.MethodGet,
					path:   "/api/skills",
				},
			} {
				if authorizer.Authorize(httptest.NewRequest(route.method, route.path, nil), info) {
					t.Fatalf("the agent was authorized on %s %s", route.method, route.path)
				}
			}
		})
	}
}

// TestNoAuthChainRefusesAgentKeysItCannotAuthenticate checks that without authentication, a request that presents an
// agent key that authenticates no agent is refused, never authenticated as the nobody owner, whichever header the key
// is in. The key of a disabled agent is refused as with authentication enabled, as not active (403); every other key
// as invalid (401).
func TestNoAuthChainRefusesAgentKeysItCannotAuthenticate(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	chain, _ := noAuthChain(t, s, client)
	person := createAPIKeyLifecycleTestUser(t, client, "ivy")
	userKey, err := client.CreateAPIKey(ctx, person.ID, "ivy", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}

	newAgentKey := func(t *testing.T) (*gatewaytypes.Agent, *gatewaytypes.APIKeyCreateResponse) {
		t.Helper()
		agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
		if err != nil {
			t.Fatal(err)
		}
		key, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "ci", CanAccessLLMProxy: true})
		if err != nil {
			t.Fatal(err)
		}
		return agent, key
	}

	for _, tt := range []struct {
		name string
		// key returns the agent key to present.
		key func(t *testing.T) string
		// headers are the headers to send besides the key, which is sent in keyHeader.
		headers map[string]string
		// keyHeader is the header the key is sent in; the Authorization header when empty.
		keyHeader string
		// disabled is set for the key of a disabled agent, which is refused as not active.
		disabled bool
	}{
		{
			name: "revoked",
			key: func(t *testing.T) string {
				t.Helper()
				agent, key := newAgentKey(t)
				if err := client.RevokeAgentAPIKey(ctx, agent.ID, key.ID); err != nil {
					t.Fatal(err)
				}
				return key.Key
			},
		},
		{
			name: "expired",
			key: func(t *testing.T) string {
				t.Helper()
				_, key := newAgentKey(t)
				if err := s.db.WithContext(ctx).Model(&gatewaytypes.APIKey{}).Where("id = ?", key.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
				return key.Key
			},
		},
		{
			name: "unknown secret",
			key: func(t *testing.T) string {
				t.Helper()
				agent, key := newAgentKey(t)
				return fmt.Sprintf("oak1-%d-%d-wrong", agent.ID, key.ID)
			},
		},
		{
			name: "unknown key",
			key: func(*testing.T) string {
				return "oak1-999-999-secret"
			},
		},
		{
			name: "malformed",
			key: func(*testing.T) string {
				return "oak1-garbage"
			},
		},
		{
			name: "of a disabled agent",
			key: func(t *testing.T) string {
				t.Helper()
				agent, key := newAgentKey(t)
				if _, err := client.DisableAgent(ctx, agent.ID, "compromised"); err != nil {
					t.Fatal(err)
				}
				return key.Key
			},
			disabled: true,
		},
		{
			name: "of a deleted agent",
			key: func(t *testing.T) string {
				t.Helper()
				agent, key := newAgentKey(t)
				if err := client.DeleteAgent(ctx, agent.ID); err != nil {
					t.Fatal(err)
				}
				return key.Key
			},
		},
		{
			name: "revoked, in X-API-Key",
			key: func(t *testing.T) string {
				t.Helper()
				agent, key := newAgentKey(t)
				if err := client.RevokeAgentAPIKey(ctx, agent.ID, key.ID); err != nil {
					t.Fatal(err)
				}
				return key.Key
			},
			keyHeader: "X-API-Key",
		},
		{
			name: "revoked, in X-Obot-Api-Key on the LLM proxy",
			key: func(t *testing.T) string {
				t.Helper()
				agent, key := newAgentKey(t)
				if err := client.RevokeAgentAPIKey(ctx, agent.ID, key.ID); err != nil {
					t.Fatal(err)
				}
				return key.Key
			},
			keyHeader: HeaderObotAPIKey,
		},
		{
			// The API key authenticator reads Authorization first, so it would not authenticate the agent; the
			// request still presents an agent key, and must not become the nobody owner.
			name: "valid, in X-API-Key beside another bearer value",
			key: func(t *testing.T) string {
				t.Helper()
				_, key := newAgentKey(t)
				return key.Key
			},
			headers: map[string]string{
				"Authorization": "Bearer not-a-credential",
			},
			keyHeader: "X-API-Key",
		},
		{
			name: "valid, in X-API-Key beside a user's key",
			key: func(t *testing.T) string {
				t.Helper()
				_, key := newAgentKey(t)
				return key.Key
			},
			headers: map[string]string{
				"Authorization": "Bearer " + userKey.Key,
			},
			keyHeader: "X-API-Key",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{}
			maps.Copy(headers, tt.headers)
			switch key := tt.key(t); tt.keyHeader {
			case "":
				headers["Authorization"] = "Bearer " + key
			default:
				headers[tt.keyHeader] = key
			}
			info, err := authenticateNoAuth(t, chain, noAuthTestRequest(t, noAuthTestLLMPath, headers))
			if err == nil {
				t.Fatalf("authenticated as %q (UID %q); want the request refused", info.GetName(), info.GetUID())
			}
			if tt.disabled {
				if denied, ok := findAgentDenied(err); !ok || denied.Status != types2.UserStatusDisabled {
					t.Fatalf("authenticate = %v; want the agent denied as disabled", err)
				}
				return
			}
			if !errors.Is(err, errInvalidAgentAPIKey) {
				t.Fatalf("authenticate = %v; want the key refused as invalid", err)
			}
		})
	}
}

// TestNoAuthChainLeavesOtherRequestsToNoAuth checks that without authentication, a request with no agent credential is
// authenticated as before: with no credential, a user's API key whether or not it is valid, or any other bearer value,
// as the nobody owner, and with a user's token as that user.
func TestNoAuthChainLeavesOtherRequestsToNoAuth(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	chain, tokenService := noAuthChain(t, s, client)
	person := createAPIKeyLifecycleTestUser(t, client, "jo")
	userKey, err := client.CreateAPIKey(ctx, person.ID, "jo", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	revokedUserKey, err := client.CreateAPIKey(ctx, person.ID, "jo-revoked", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.RevokeAPIKeyByID(ctx, revokedUserKey.ID); err != nil {
		t.Fatal(err)
	}
	// An agent with a key exists, so that its keys are known to the server.
	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "ci", CanAccessLLMProxy: true}); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name    string
		path    string
		headers map[string]string
	}{
		{
			name: "no credential",
			path: "/api/agents",
		},
		{
			name: "a user's key",
			path: noAuthTestLLMPath,
			headers: map[string]string{
				"Authorization": "Bearer " + userKey.Key,
			},
		},
		{
			name: "a user's key in X-API-Key",
			path: noAuthTestLLMPath,
			headers: map[string]string{
				"X-API-Key": userKey.Key,
			},
		},
		{
			name: "a user's revoked key",
			path: noAuthTestLLMPath,
			headers: map[string]string{
				"Authorization": "Bearer " + revokedUserKey.Key,
			},
		},
		{
			name: "an unknown user key",
			path: noAuthTestLLMPath,
			headers: map[string]string{
				"Authorization": "Bearer ok1-999-999-secret",
			},
		},
		{
			name: "another bearer value",
			path: noAuthTestLLMPath,
			headers: map[string]string{
				"Authorization": "Bearer not-a-credential",
			},
		},
		{
			// X-Obot-Api-Key is only read on the LLM proxy, so elsewhere it presents no key, as with authentication
			// enabled.
			name: "an agent key in X-Obot-Api-Key off the LLM proxy",
			path: "/api/agents",
			headers: map[string]string{
				HeaderObotAPIKey: "oak1-999-999-secret",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info, err := authenticateNoAuth(t, chain, noAuthTestRequest(t, tt.path, tt.headers))
			requireNobody(t, info, err)
		})
	}

	_, userToken, err := tokenService.NewToken(ctx, persistent.TokenContext{
		Audience:   "https://obot.example.com/mcp-connect/ms1component",
		ExpiresAt:  persistent.NewTime(time.Now().Add(10 * time.Minute)),
		UserID:     principal.User(person.ID).String(),
		UserGroups: []string{types2.GroupMCP, types2.GroupAuthenticated},
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := authenticateNoAuth(t, chain, noAuthTestRequest(t, "/mcp-connect/ms1component", map[string]string{
		"Authorization": "Bearer " + userToken,
	}))
	if err != nil {
		t.Fatalf("authenticate a user's token = %v; want the user", err)
	}
	if got, ok := principal.FromUserInfo(info); !ok || got != principal.User(person.ID) {
		t.Fatalf("authenticated a user's token as %#v, %v; want user %d", got, ok, person.ID)
	}
}

// TestNoAuthChainAgentTokens checks that without authentication, the token service issues an agent a loopback token,
// which authenticates as the agent, and that a token of a disabled or deleted agent, or an expired one, is refused,
// never authenticated as the nobody owner. No token is issued to a disabled agent.
func TestNoAuthChainAgentTokens(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	chain, tokenService := noAuthChain(t, s, client)
	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	loopback := func(expiresAt time.Time) (string, error) {
		_, token, err := tokenService.NewToken(ctx, persistent.TokenContext{
			Audience:   "https://obot.example.com/mcp-connect/ms1component",
			IssuedAt:   persistent.NewTime(expiresAt.Add(-10 * time.Minute)),
			ExpiresAt:  persistent.NewTime(expiresAt),
			UserID:     principal.Agent(agent.ID).String(),
			UserGroups: []string{types2.GroupMCP, types2.GroupCompositeMCP, types2.GroupAuthenticated},
			MCPID:      "ms1component",
		})
		return token, err
	}
	authenticate := func(token string) (user.Info, error) {
		t.Helper()
		return authenticateNoAuth(t, chain, noAuthTestRequest(t, "/mcp-connect/ms1component", map[string]string{
			"Authorization": "Bearer " + token,
		}))
	}
	assertAgent := func(name, token string) {
		t.Helper()
		info, err := authenticate(token)
		if err != nil {
			t.Fatalf("%s: %v; want the agent", name, err)
		}
		if got, ok := principal.FromUserInfo(info); !ok || got != principal.Agent(agent.ID) {
			t.Fatalf("%s: authenticated as %#v, %v (name %q); want agent %d", name, got, ok, info.GetName(), agent.ID)
		}
	}
	assertRefused := func(name, token string) error {
		t.Helper()
		info, err := authenticate(token)
		if err == nil {
			t.Fatalf("%s: authenticated as %q (UID %q); want the token refused", name, info.GetName(), info.GetUID())
		}
		return err
	}

	token, err := loopback(time.Now().Add(10 * time.Minute))
	if err != nil {
		t.Fatalf("failed to issue a loopback token to an active agent: %v", err)
	}
	assertAgent("loopback token", token)

	expired, err := loopback(time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_ = assertRefused("expired loopback token", expired)

	if _, err := client.DisableAgent(ctx, agent.ID, "compromised"); err != nil {
		t.Fatal(err)
	}
	if denied, ok := findAgentDenied(assertRefused("disabled agent's loopback token", token)); !ok || denied.Status != types2.UserStatusDisabled {
		t.Fatal("the disabled agent's loopback token was not denied as disabled")
	}
	if _, err := loopback(time.Now().Add(10 * time.Minute)); err == nil {
		t.Fatal("issued a token to a disabled agent")
	}

	if _, err := client.EnableAgent(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	assertAgent("re-enabled agent's loopback token", token)

	if err := client.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	if denied, ok := findAgentDenied(assertRefused("deleted agent's loopback token", token)); !ok || denied.Status != types2.UserStatusDeleted {
		t.Fatal("the deleted agent's loopback token was not denied as deleted")
	}
}
