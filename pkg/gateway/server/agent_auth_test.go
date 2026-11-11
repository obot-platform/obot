package server

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authn"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// createAgentWithUsersID creates an agent with the same numeric ID as user. Users and agents are numbered apart, so
// agents are created until one has the user's ID.
func createAgentWithUsersID(t *testing.T, client *gatewayclient.Client, user *gatewaytypes.User) *gatewaytypes.Agent {
	t.Helper()

	agent, err := client.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	for agent.ID < user.ID {
		if agent, err = client.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "nightly build"}); err != nil {
			t.Fatalf("failed to create agent: %v", err)
		}
	}
	if agent.ID != user.ID {
		t.Fatalf("agent ID %d, want the user's ID %d", agent.ID, user.ID)
	}
	return agent
}

func authenticateAPIKey(t *testing.T, authenticator *APIKeyAuthenticator, key string) (user.Info, bool, error) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp-connect/vmcp1test", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	response, ok, err := authenticator.AuthenticateRequest(req)
	if !ok || err != nil {
		if response != nil {
			t.Fatalf("authenticate = %v, %v, %v; want no principal with a refusal", response, ok, err)
		}
		return nil, ok, err
	}
	return response.User, true, nil
}

// TestAPIKeyAuthenticatorBuildsTheAgentPrincipal checks the principal of an agent's key: the agent's encoded ID as
// UID and name, the agent kind and status, its display name, the key's attribution and MCP scope, and only the groups
// the key's scopes grant. It carries no user privilege: no role group, GroupAPI, email, obot_groups or
// auth_provider_groups.
func TestAPIKeyAuthenticatorBuildsTheAgentPrincipal(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name       string
		request    gatewaytypes.AgentAPIKeyCreateRequest
		wantGroups []string
		wantName   func(created *gatewaytypes.APIKeyCreateResponse) string
	}{
		{
			name: "every scope",
			request: gatewaytypes.AgentAPIKeyCreateRequest{
				Name:              "nightly",
				CanAccessSkills:   true,
				CanAccessLLMProxy: true,
				MCPServerIDs:      []string{"*"},
			},
			wantGroups: []string{types2.GroupSkills, types2.GroupLLM, types2.GroupMCP, types2.GroupAuthenticated},
			wantName:   func(*gatewaytypes.APIKeyCreateResponse) string { return "nightly" },
		},
		{
			name: "LLM proxy only, unnamed",
			request: gatewaytypes.AgentAPIKeyCreateRequest{
				CanAccessLLMProxy: true,
			},
			wantGroups: []string{types2.GroupLLM, types2.GroupAuthenticated},
			wantName: func(created *gatewaytypes.APIKeyCreateResponse) string {
				return fmt.Sprintf("oak1-%d-%d-*****", agent.ID, created.ID)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			created, err := client.CreateAgentAPIKey(ctx, agent.ID, tt.request)
			if err != nil {
				t.Fatal(err)
			}
			info, ok, err := authenticateAPIKey(t, NewAPIKeyAuthenticator(client, true), created.Key)
			if err != nil || !ok {
				t.Fatalf("authenticate = %v, %v; want the agent key authenticated", ok, err)
			}

			wantUID := fmt.Sprintf("agent:%d", agent.ID)
			if info.GetUID() != wantUID || info.GetName() != wantUID {
				t.Errorf("UID, name = %q, %q; want both %q", info.GetUID(), info.GetName(), wantUID)
			}
			if got, ok := principal.FromUserInfo(info); !ok || got != principal.Agent(agent.ID) {
				t.Errorf("FromUserInfo() = %#v, %v; want agent %d", got, ok, agent.ID)
			}
			if status, recorded := principal.UserStatus(info); !recorded || status != types2.UserStatusActive {
				t.Errorf("status = %q, %v; want active", status, recorded)
			}
			if !slices.Equal(info.GetGroups(), tt.wantGroups) {
				t.Errorf("groups = %v, want %v", info.GetGroups(), tt.wantGroups)
			}
			extra := info.GetExtra()
			if got := extra["authorized_mcp_ids"]; !slices.Equal(got, tt.request.MCPServerIDs) {
				t.Errorf("authorized MCP IDs = %v, want %v", got, tt.request.MCPServerIDs)
			}
			for _, key := range []string{"email", "obot_groups", "auth_provider_groups", "auth_provider_name", "auth_provider_namespace"} {
				if _, ok := extra[key]; ok {
					t.Errorf("extra has %s = %v, which only users have", key, extra[key])
				}
			}
			attribution, ok := principal.APIKeyAttributionFromUser(info)
			if !ok || attribution.ID != created.ID || attribution.Name != tt.wantName(created) {
				t.Errorf("attribution = %#v, %v; want key %d named %q", attribution, ok, created.ID, tt.wantName(created))
			}
		})
	}
}

// TestAPIKeyAuthenticatorNeverGrantsAgentsAPIAccess checks that an agent's key never gets GroupAPI, a role group, or
// device scans, even should its stored scopes claim API or device scan access, which an agent's key cannot be created
// with.
func TestAPIKeyAuthenticatorNeverGrantsAgentsAPIAccess(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.WithContext(ctx).Model(&gatewaytypes.APIKey{}).Where("id = ?", created.ID).Updates(map[string]any{
		"can_access_api":          true,
		"can_access_device_scans": true,
	}).Error; err != nil {
		t.Fatalf("failed to widen the stored key: %v", err)
	}

	info, ok, err := authenticateAPIKey(t, NewAPIKeyAuthenticator(client, true), created.Key)
	if err != nil || !ok {
		t.Fatalf("authenticate = %v, %v; want the agent key authenticated", ok, err)
	}
	if want := []string{types2.GroupLLM, types2.GroupAuthenticated}; !slices.Equal(info.GetGroups(), want) {
		t.Fatalf("groups = %v, want only %v", info.GetGroups(), want)
	}
}

// TestAPIKeysOfAUserAndAnAgentWithTheSameIDAreNeverConfused checks that a user's key authenticates the user, an
// agent's key the agent with the same numeric ID, and neither key authenticates the other when presented with the
// other's prefix. A change to one's lifecycle leaves the other alone.
func TestAPIKeysOfAUserAndAnAgentWithTheSameIDAreNeverConfused(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	user := createAPIKeyLifecycleTestUser(t, client, "olga")
	agent := createAgentWithUsersID(t, client, user)

	userKey, err := client.CreateAPIKey(ctx, user.ID, "olga", "", nil, gatewaytypes.APIKeyScopes{CanAccessAPI: true})
	if err != nil {
		t.Fatal(err)
	}
	agentKey, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _, userKeyID, userSecret, err := gatewayclient.ParseAPIKey(userKey.Key)
	if err != nil {
		t.Fatal(err)
	}
	_, _, agentKeyID, agentSecret, err := gatewayclient.ParseAPIKey(agentKey.Key)
	if err != nil {
		t.Fatal(err)
	}

	authenticator := NewAPIKeyAuthenticator(client, true)
	assertAuthenticatesAs := func(key string, want principal.Principal) {
		t.Helper()
		info, ok, err := authenticateAPIKey(t, authenticator, key)
		if err != nil || !ok {
			t.Fatalf("authenticate = %v, %v; want %s", ok, err, want)
		}
		if got, ok := principal.FromUserInfo(info); !ok || got != want {
			t.Fatalf("authenticated as %#v, %v; want %#v", got, ok, want)
		}
	}
	assertDeclined := func(key string) {
		t.Helper()
		if info, ok, err := authenticateAPIKey(t, authenticator, key); ok || err != nil {
			t.Fatalf("authenticate = %v, %v, %v; want the key declined", info, ok, err)
		}
	}

	assertAuthenticatesAs(userKey.Key, principal.User(user.ID))
	assertAuthenticatesAs(agentKey.Key, principal.Agent(agent.ID))
	// Each key presented with the other prefix names the other owner, which does not own it.
	assertDeclined(fmt.Sprintf("oak1-%d-%d-%s", user.ID, userKeyID, userSecret))
	assertDeclined(fmt.Sprintf("ok1-%d-%d-%s", agent.ID, agentKeyID, agentSecret))

	// Disabling the agent leaves the user alone.
	if _, err := client.DisableAgent(ctx, agent.ID, "rotating keys"); err != nil {
		t.Fatal(err)
	}
	assertAuthenticatesAs(userKey.Key, principal.User(user.ID))
	_, _, err = authenticateAPIKey(t, authenticator, agentKey.Key)
	if denied, ok := errors.AsType[*gatewayclient.AgentAccessDeniedError](err); !ok || denied.AgentID != agent.ID {
		t.Fatalf("authenticate the disabled agent's key = %v; want agent %d denied", err, agent.ID)
	}
	if _, err := client.EnableAgent(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}

	// Deleting the user leaves the agent alone.
	if err := client.DeleteUser(ctx, fmt.Sprint(user.ID)); err != nil {
		t.Fatal(err)
	}
	assertAuthenticatesAs(agentKey.Key, principal.Agent(agent.ID))
	_, _, err = authenticateAPIKey(t, authenticator, userKey.Key)
	if denied, ok := errors.AsType[*gatewayclient.UserAccessDeniedError](err); !ok || denied.UserID != user.ID {
		t.Fatalf("authenticate the deleted user's key = %v; want user %d denied", err, user.ID)
	}
}

// TestAPIKeyAuthenticatorDeniesInactiveAgents checks that disabling an agent denies its key from the next request on,
// whether the key's validation is cached or not, that enabling it restores the key, and that deleting it revokes the
// key.
func TestAPIKeyAuthenticatorDeniesInactiveAgents(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	authenticator := NewAPIKeyAuthenticator(client, true)
	assertAdmitted := func(key string) {
		t.Helper()
		if _, ok, err := authenticateAPIKey(t, authenticator, key); err != nil || !ok {
			t.Fatalf("authenticate = %v, %v; want the agent admitted", ok, err)
		}
	}
	assertDenied := func(key string, want types2.UserStatus) {
		t.Helper()
		_, _, err := authenticateAPIKey(t, authenticator, key)
		if denied, ok := errors.AsType[*gatewayclient.AgentAccessDeniedError](err); !ok || denied.AgentID != agent.ID || denied.Status != want {
			t.Fatalf("authenticate = %v; want agent %d denied as %s", err, agent.ID, want)
		}
	}

	// The first use caches the key's validation, so the next ones are cache hits.
	assertAdmitted(created.Key)
	if _, err := client.DisableAgent(ctx, agent.ID, "compromised"); err != nil {
		t.Fatal(err)
	}
	assertDenied(created.Key, types2.UserStatusDisabled)

	// A key that has not been validated yet is denied as well. A disabled agent can be given one, to prepare a
	// rotation.
	uncached, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	assertDenied(uncached.Key, types2.UserStatusDisabled)

	if _, err := client.EnableAgent(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	assertAdmitted(created.Key)
	assertAdmitted(uncached.Key)

	// Deleting the agent revokes its keys, which then authenticate no one.
	if err := client.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{created.Key, uncached.Key} {
		if info, ok, err := authenticateAPIKey(t, authenticator, key); ok || err != nil {
			t.Fatalf("authenticate a deleted agent's key = %v, %v, %v; want it declined", info, ok, err)
		}
	}
}

func TestAPIKeyAuthenticatorFailsWhenItCannotReadTheAgent(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	agent, err := client.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAgentAPIKey(t.Context(), agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	authenticator := NewAPIKeyAuthenticator(client, true)

	if err := client.Close(); err != nil {
		t.Fatalf("failed to close gateway client: %v", err)
	}
	_, ok, err := authenticateAPIKey(t, authenticator, created.Key)
	if lookupErr, isLookup := errors.AsType[*gatewayclient.AgentAccessLookupError](err); ok || !isLookup || lookupErr.AgentID != agent.ID {
		t.Fatalf("authenticate with an unreadable key = %v, %v; want a lookup failure for agent %d", ok, err, agent.ID)
	}
	if _, isUser := errors.AsType[*gatewayclient.UserAccessLookupError](err); isUser {
		t.Fatalf("authenticate with an unreadable agent key = %v; want it reported for the agent, not a user", err)
	}
}

func callAPIKeyWebhook(t *testing.T, s *Server, client *gatewayclient.Client, key string, req apiKeyAuthRequest) apiKeyAuthResponse {
	t.Helper()

	apiContext, recorder := newTokenRequestAPIContext(t, client, http.MethodPost, "/api/api-keys/auth", req, nil)
	apiContext.Storage = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.VMCP{
		Name:      "vmcp1test",
		Namespace: system.DefaultNamespace,
	}).Build()
	apiContext.Request.Header.Set("Authorization", "Bearer "+key)
	if err := s.authenticateAPIKey(apiContext); err != nil {
		t.Fatalf("failed to authenticate API key: %v", err)
	}
	var response apiKeyAuthResponse
	decodeTokenRequestResponse(t, recorder, &response)
	return response
}

// TestAPIKeyWebhookAnswersForAgents checks the webhook's answer for an agent's key: the agent's encoded ID as the
// subject, its display name, and no username or email. A user with the same numeric ID is answered for as the user.
// A disabled agent's key, and an MCP server outside the key's scope, are refused.
func TestAPIKeyWebhookAnswersForAgents(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	s.enableAgentIdentities = true
	ctx := t.Context()
	user := createAPIKeyLifecycleTestUser(t, client, "pia")
	agent := createAgentWithUsersID(t, client, user)

	userKey, err := client.CreateAPIKey(ctx, user.ID, "pia", "", nil, gatewaytypes.APIKeyScopes{MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	agentKey, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	llmOnlyKey, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}

	response := callAPIKeyWebhook(t, s, client, agentKey.Key, apiKeyAuthRequest{MCPID: "vmcp1test"})
	if !response.Allowed || response.Subject != fmt.Sprintf("agent:%d", agent.ID) || response.Name != "nightly build" || response.PreferredUsername != "" || response.Email != "" {
		t.Fatalf("webhook response for the agent's key = %+v; want agent:%d named nightly build, with no username or email", response, agent.ID)
	}
	if !slices.Equal(response.Scopes.MCPServerIDs, []string{"*"}) || response.Scopes.CanAccessAPI {
		t.Fatalf("webhook scopes for the agent's key = %+v", response.Scopes)
	}

	response = callAPIKeyWebhook(t, s, client, userKey.Key, apiKeyAuthRequest{MCPID: "vmcp1test"})
	if !response.Allowed || response.Subject != fmt.Sprint(user.ID) || response.Email != user.Email {
		t.Fatalf("webhook response for the user's key = %+v; want user %d", response, user.ID)
	}

	if response := callAPIKeyWebhook(t, s, client, llmOnlyKey.Key, apiKeyAuthRequest{MCPID: "vmcp1test"}); response.Allowed || response.Subject != "" {
		t.Fatalf("webhook response for an agent key without MCP scope = %+v; want denied", response)
	}

	if _, err := client.DisableAgent(ctx, agent.ID, "compromised"); err != nil {
		t.Fatal(err)
	}
	for _, req := range []apiKeyAuthRequest{
		{
			MCPID: "vmcp1test",
		},
		{
			ValidateOnly: true,
		},
	} {
		if response := callAPIKeyWebhook(t, s, client, agentKey.Key, req); response.Allowed || response.Subject != "" || response.Reason != "agent is not active" {
			t.Fatalf("webhook response for a disabled agent's key = %+v; want denied as not active", response)
		}
	}
	if response := callAPIKeyWebhook(t, s, client, userKey.Key, apiKeyAuthRequest{ValidateOnly: true}); !response.Allowed {
		t.Fatalf("webhook response for the user's key = %+v; want allowed while the agent is disabled", response)
	}
}

// TestAPIKeyWebhookNeverReportsAgentsAPIAccess checks that the webhook reports an agent's key with the scopes the
// authenticator honors, without API or device scan access, even should its stored scopes claim them.
func TestAPIKeyWebhookNeverReportsAgentsAPIAccess(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	s.enableAgentIdentities = true
	ctx := t.Context()
	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.WithContext(ctx).Model(&gatewaytypes.APIKey{}).Where("id = ?", created.ID).Updates(map[string]any{
		"can_access_api":          true,
		"can_access_device_scans": true,
	}).Error; err != nil {
		t.Fatalf("failed to widen the stored key: %v", err)
	}

	response := callAPIKeyWebhook(t, s, client, created.Key, apiKeyAuthRequest{ValidateOnly: true})
	want := gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}}
	if !response.Allowed || response.Scopes.CanAccessAPI || response.Scopes.CanAccessDeviceScans ||
		response.Scopes.CanAccessLLMProxy != want.CanAccessLLMProxy || !slices.Equal(response.Scopes.MCPServerIDs, want.MCPServerIDs) {
		t.Fatalf("webhook response for a widened agent key = %+v; want allowed with scopes %+v", response, want)
	}
}

// TestAPIKeyWebhookFailsWhenItCannotReadTheAgent checks that the webhook answers a failure to read a key's agent with a
// server error, not as an agent that does not exist.
func TestAPIKeyWebhookFailsWhenItCannotReadTheAgent(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	s.enableAgentIdentities = true
	ctx := t.Context()
	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	// Reads of the agents table fail. The key itself is read through the api_keys table, so it still validates.
	if err := s.db.WithContext(ctx).Callback().Query().Before("gorm:query").Register("test:fail_agent_reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "agents" {
			_ = tx.AddError(errors.New("agents are unreadable"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	apiContext, recorder := newTokenRequestAPIContext(t, client, http.MethodPost, "/api/api-keys/auth", apiKeyAuthRequest{ValidateOnly: true}, nil)
	apiContext.Request.Header.Set("Authorization", "Bearer "+created.Key)
	err = s.authenticateAPIKey(apiContext)
	if httpErr, ok := errors.AsType[*types2.ErrHTTP](err); !ok || httpErr.Code != http.StatusInternalServerError {
		t.Fatalf("webhook with an unreadable agent = %v (body %q); want a server error", err, recorder.Body.String())
	}
}

// agentChain composes the authenticators of agents' credentials as the server does, with agent identities enabled.
func agentChain(t *testing.T, client *gatewayclient.Client) (*authn.AdmissionCheck, *persistent.TokenService) {
	t.Helper()

	_, jwk, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.JWKCredentialContext,
		Name:    system.JWKCredentialContext,
		Secrets: map[string]string{
			"JWK_KEY": base64.StdEncoding.EncodeToString(jwk),
		},
	}); err != nil {
		t.Fatalf("failed to store JWK: %v", err)
	}
	tokenService, err := persistent.NewTokenService("https://obot.example.com", client, true)
	if err != nil {
		t.Fatal(err)
	}
	return authn.NewAdmissionCheck(union.NewFailOnError(
		union.New(
			NewAPIKeyAuthenticator(client, true),
			tokenService,
		),
		authn.Anonymous{},
	), nil), tokenService
}

// TestAgentLifecycleAcrossTheAuthenticatorChain checks, across the authenticator chain as the server composes it,
// that an agent and a user with the same numeric ID each authenticate as themselves through API keys and loopback
// tokens, and that disabling or deleting the agent denies it on every path from the next request on, including with
// a loopback token issued before the change, without affecting the user.
func TestAgentLifecycleAcrossTheAuthenticatorChain(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	chain, tokenService := agentChain(t, client)
	person := createAPIKeyLifecycleTestUser(t, client, "quinn")
	agent := createAgentWithUsersID(t, client, person)

	agentKey, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "nightly", MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	userKey, err := client.CreateAPIKey(ctx, person.ID, "quinn", "", nil, gatewaytypes.APIKeyScopes{MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}

	// Loopback tokens as a composite vMCP issues them to reach its components, for each principal.
	loopback := func(subject string, keyID uint) string {
		t.Helper()
		now := time.Now().Add(-time.Second)
		_, token, err := tokenService.NewToken(ctx, persistent.TokenContext{
			Audience:   "https://obot.example.com/mcp-connect/ms1component",
			IssuedAt:   persistent.NewTime(now),
			ExpiresAt:  persistent.NewTime(now.Add(10 * time.Minute)),
			UserID:     subject,
			UserGroups: []string{types2.GroupMCP, types2.GroupCompositeMCP, types2.GroupAuthenticated},
			MCPID:      "ms1component",
			APIKeyID:   fmt.Sprint(keyID),
			APIKeyName: "nightly",
		})
		if err != nil {
			t.Fatalf("failed to issue a loopback token for %s: %v", subject, err)
		}
		return token
	}
	agentToken := loopback(principal.Agent(agent.ID).String(), agentKey.ID)
	userToken := loopback(principal.User(person.ID).String(), userKey.ID)

	authenticate := func(token string) (user.Info, error) {
		t.Helper()
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp-connect/ms1component", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, ok, err := chain.AuthenticateRequest(req)
		if err != nil {
			return nil, err
		}
		if !ok {
			t.Fatal("the chain declined a request, which the anonymous authenticator must prevent")
		}
		return resp.User, nil
	}
	assertAuthenticatesAs := func(name, token string, want principal.Principal) user.Info {
		t.Helper()
		info, err := authenticate(token)
		if err != nil {
			t.Fatalf("%s: %v; want %s admitted", name, err, want)
		}
		if got, ok := principal.FromUserInfo(info); !ok || got != want {
			t.Fatalf("%s: authenticated as %#v, %v (UID %q); want %#v", name, got, ok, info.GetUID(), want)
		}
		return info
	}
	assertAgentDenied := func(name, token string, want types2.UserStatus) {
		t.Helper()
		_, err := authenticate(token)
		if denied, ok := findAgentDenied(err); !ok || denied.AgentID != agent.ID || denied.Status != want {
			t.Fatalf("%s: %v; want agent %d denied as %s", name, err, agent.ID, want)
		}
	}

	assertAuthenticatesAs("user key", userKey.Key, principal.User(person.ID))
	assertAuthenticatesAs("user loopback token", userToken, principal.User(person.ID))
	assertAuthenticatesAs("agent key", agentKey.Key, principal.Agent(agent.ID))
	info := assertAuthenticatesAs("agent loopback token", agentToken, principal.Agent(agent.ID))
	// The loopback principal is built as the key's is: the agent's encoded ID and the originating key, and nothing only
	// users have.
	if info.GetName() != info.GetUID() {
		t.Fatalf("agent loopback principal = %#v", info)
	}
	if attribution, ok := principal.APIKeyAttributionFromUser(info); !ok || attribution.ID != agentKey.ID {
		t.Fatalf("agent loopback attribution = %#v, %v; want key %d", attribution, ok, agentKey.ID)
	}
	for _, key := range []string{"email", "obot_groups", "auth_provider_groups"} {
		if _, ok := info.GetExtra()[key]; ok {
			t.Fatalf("agent loopback principal has %s, which only users have", key)
		}
	}

	if _, err := client.DisableAgent(ctx, agent.ID, "compromised"); err != nil {
		t.Fatal(err)
	}
	assertAgentDenied("disabled agent key", agentKey.Key, types2.UserStatusDisabled)
	assertAgentDenied("disabled agent loopback token issued before", agentToken, types2.UserStatusDisabled)
	assertAuthenticatesAs("user key with the agent disabled", userKey.Key, principal.User(person.ID))
	assertAuthenticatesAs("user loopback token with the agent disabled", userToken, principal.User(person.ID))
	// No new token is issued to the disabled agent, including by background work.
	if _, _, err := tokenService.NewToken(ctx, persistent.TokenContext{
		Audience: "https://obot.example.com/mcp-connect/ms1component",
		UserID:   principal.Agent(agent.ID).String(),
	}); err == nil {
		t.Fatal("issued a token to a disabled agent")
	}

	if _, err := client.EnableAgent(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	assertAuthenticatesAs("re-enabled agent key", agentKey.Key, principal.Agent(agent.ID))
	assertAuthenticatesAs("re-enabled agent loopback token", agentToken, principal.Agent(agent.ID))

	// Deleting the agent revokes its key, which then authenticates no one, and denies its loopback token.
	if err := client.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	if info, err := authenticate(agentKey.Key); err != nil || info.GetName() != "anonymous" {
		t.Fatalf("deleted agent key = %v, %v; want it anonymous", info, err)
	}
	assertAgentDenied("deleted agent loopback token", agentToken, types2.UserStatusDeleted)
	assertAuthenticatesAs("user key with the agent deleted", userKey.Key, principal.User(person.ID))
}

// findAgentDenied finds an agent's denial in an error from the authenticator chain, whose unions aggregate their
// members' errors.
func findAgentDenied(err error) (*gatewayclient.AgentAccessDeniedError, bool) {
	if denied, ok := errors.AsType[*gatewayclient.AgentAccessDeniedError](err); ok {
		return denied, true
	}
	if aggregate, ok := errors.AsType[utilerrors.Aggregate](err); ok {
		for _, err := range aggregate.Errors() {
			if denied, ok := findAgentDenied(err); ok {
				return denied, true
			}
		}
	}
	return nil, false
}
