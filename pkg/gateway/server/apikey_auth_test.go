package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
)

func TestAPIKeyAuthenticatorCarriesAuditAttribution(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	keyOwner, err := client.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		ProviderUsername: "alice",
		ProviderUserID:   "alice",
		Email:            "alice@example.com",
	}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAPIKey(t.Context(), keyOwner.ID, "CLI token", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer "+created.Key)

	response, ok, err := NewAPIKeyAuthenticator(client, false).AuthenticateRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || response == nil || response.User == nil {
		t.Fatal("expected API key to authenticate")
	}
	wantObotGroups := types2.RoleBasic.RoleGroups()
	if !slices.Equal(response.User.GetExtra()["obot_groups"], wantObotGroups) {
		t.Fatalf("Obot groups extra = %v, want role groups %v", response.User.GetExtra()["obot_groups"], wantObotGroups)
	}
	if got := response.User.GetExtra()[principal.APIKeyRawNameExtra]; len(got) != 1 || got[0] != "CLI token" {
		t.Fatalf("raw key name = %v, want CLI token", got)
	}
	attribution, ok := principal.APIKeyAttributionFromUser(response.User)
	if !ok || attribution.ID != created.ID || attribution.Name != "CLI token" {
		t.Fatalf("attribution = %#v, %v; want key %d named CLI token", attribution, ok, created.ID)
	}
}

func TestAPIKeyAuthenticatorCarriesMaskedAttributionForUnnamedKey(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	keyOwner, err := client.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		ProviderUsername: "alice-unnamed",
		ProviderUserID:   "alice-unnamed",
		Email:            "alice-unnamed@example.com",
	}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAPIKey(t.Context(), keyOwner.ID, "", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer "+created.Key)

	response, ok, err := NewAPIKeyAuthenticator(client, false).AuthenticateRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || response == nil || response.User == nil {
		t.Fatal("expected API key to authenticate")
	}
	attribution, ok := principal.APIKeyAttributionFromUser(response.User)
	wantName := fmt.Sprintf("ok1-%d-%d-*****", keyOwner.ID, created.ID)
	if got := response.User.GetExtra()[principal.APIKeyRawNameExtra]; len(got) != 1 || got[0] != "" {
		t.Fatalf("raw name for unnamed key = %v, want empty", got)
	}
	if !ok || attribution.ID != created.ID || attribution.Name != wantName {
		t.Fatalf("attribution = %#v, %v; want key %d named %q", attribution, ok, created.ID, wantName)
	}
}

func TestOnlyClaudeCredentialsEnableDeferredMCPAuthorization(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	owner, err := client.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		ProviderUsername: "claude-user", ProviderUserID: "claude-user", Email: "claude@example.com",
	}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	// Matching display metadata is not sufficient to impersonate a runner credential.
	external, err := client.CreateAPIKey(t.Context(), owner.ID, "Claude session acr-test", "Self-hosted Claude session", nil, gatewaytypes.APIKeyScopes{MCPServerIDs: []string{"vmcp1"}})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := client.EnsureClaudeCredential(t.Context(), owner.ID, "acr-test", []string{"vmcp1"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "external",
			key:  external.Key,
		},
		{
			name: "runner",
			key:  runner.Secrets["key"],
			want: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/mcp-connect/vmcp1", nil)
			request.Header.Set("Authorization", "Bearer "+test.key)
			response, ok, err := NewAPIKeyAuthenticator(client, false).AuthenticateRequest(request)
			if err != nil || !ok {
				t.Fatalf("authenticate: %v, %v", ok, err)
			}
			if got := slices.Contains(response.User.GetGroups(), types2.GroupClaudeEnvironment); got != test.want {
				t.Fatalf("Claude environment group = %v, want %v", got, test.want)
			}
		})
	}
}

// X-Obot-Api-Key authenticates only LLM proxy requests: the LLM proxy strips it before
// forwarding, but on other routes, such as MCP connections, it would reach an upstream server.
func TestAPIKeyAuthenticatorReadsObotAPIKeyHeaderOnlyOnTheLLMProxy(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	keyOwner, err := client.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		ProviderUsername: "alice",
		ProviderUserID:   "alice",
		Email:            "alice@example.com",
	}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAPIKey(t.Context(), keyOwner.ID, "Claude Code", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	authenticate := func(path, bearer string) bool {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", bearer)
		req.Header.Set(HeaderObotAPIKey, created.Key)
		_, ok, err := NewAPIKeyAuthenticator(client, false).AuthenticateRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	for _, bearer := range []string{"Bearer sk-ant-oat01-subscription", "Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJmaXh0dXJlIn0.c2ln"} {
		if !authenticate("/api/llm-proxy/v1/responses", bearer) {
			t.Error("X-Obot-Api-Key didn't authenticate an LLM proxy request")
		}
		if authenticate("/mcp-connect/ms1example", bearer) {
			t.Error("X-Obot-Api-Key authenticated an MCP connection")
		}
	}
}

func TestAPIKeyAuthenticatorRecordsTheUserKind(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	keyOwner, err := client.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		ProviderUsername: "kim",
		ProviderUserID:   "kim",
		Email:            "kim@example.com",
	}, "", types2.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateAPIKey(t.Context(), keyOwner.ID, "CLI token", "", nil, gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer "+created.Key)

	response, ok, err := NewAPIKeyAuthenticator(client, false).AuthenticateRequest(req)
	if err != nil || !ok || response == nil || response.User == nil {
		t.Fatalf("authenticate = %v, %v; want the key to authenticate", ok, err)
	}
	if got, ok := principal.FromUserInfo(response.User); !ok || got != principal.User(keyOwner.ID) {
		t.Fatalf("FromUserInfo() = %#v, %v; want %#v", got, ok, principal.User(keyOwner.ID))
	}
}

// TestAPIKeyAuthenticatorDoesNotAuthenticateAgentKeys checks that agent keys authenticate no one while agent identities
// are disabled: neither with their own prefix, nor presented with a user key's prefix as the key of the user with the
// agent's ID. With agent identities enabled, the latter still authenticates no one.
func TestAPIKeyAuthenticatorDoesNotAuthenticateAgentKeys(t *testing.T) {
	_, client := newTokenRequestTestServer(t)
	ctx := t.Context()
	user, err := client.EnsureIdentityWithRole(ctx, &gatewaytypes.Identity{
		ProviderUsername: "nina",
		ProviderUserID:   "nina",
		Email:            "nina@example.com",
	}, "", types2.RoleAdmin, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"})
	if err != nil {
		t.Fatal(err)
	}
	for agent.ID < user.ID {
		if agent, err = client.CreateAgent(ctx, gatewaytypes.Agent{DisplayName: "nightly build"}); err != nil {
			t.Fatal(err)
		}
	}
	if agent.ID != user.ID {
		t.Fatalf("agent ID %d, want the user's ID %d", agent.ID, user.ID)
	}
	created, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "nightly", MCPServerIDs: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	// The key itself is valid, so a refusal below is the authenticator's.
	if _, err := client.ValidateAPIKey(ctx, created.Key); err != nil {
		t.Fatalf("failed to validate the agent key: %v", err)
	}
	_, _, keyID, secret, err := gatewayclient.ParseAPIKey(created.Key)
	if err != nil {
		t.Fatal(err)
	}
	asUserKey := fmt.Sprintf("%s-%d-%d-%s", "ok1", user.ID, keyID, secret)

	for _, tt := range []struct {
		name   string
		path   string
		header string
		key    string
		// declinedWhenEnabled is set for a key that is declined even with agent identities enabled.
		declinedWhenEnabled bool
	}{
		{
			name:   "Authorization",
			path:   "/api/me",
			header: "Authorization",
			key:    "Bearer " + created.Key,
		},
		{
			name:   "X-API-Key",
			path:   "/api/me",
			header: "X-API-Key",
			key:    created.Key,
		},
		{
			name:   "X-Obot-Api-Key on the LLM proxy",
			path:   "/api/llm-proxy/v1/messages",
			header: HeaderObotAPIKey,
			key:    created.Key,
		},
		{
			name:   "Authorization on the LLM proxy",
			path:   "/api/llm-proxy/v1/messages",
			header: "Authorization",
			key:    "Bearer " + created.Key,
		},
		{
			name:                "agent key with a user key's prefix",
			path:                "/api/me",
			header:              "Authorization",
			key:                 "Bearer " + asUserKey,
			declinedWhenEnabled: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.Header.Set(tt.header, tt.key)
			response, ok, err := NewAPIKeyAuthenticator(client, false).AuthenticateRequest(req)
			if response != nil || ok || err != nil {
				t.Fatalf("authenticate = %v, %v, %v; want the agent key declined", response, ok, err)
			}

			response, ok, err = NewAPIKeyAuthenticator(client, true).AuthenticateRequest(req)
			if tt.declinedWhenEnabled {
				if response != nil || ok || err != nil {
					t.Fatalf("authenticate with agent identities enabled = %v, %v, %v; want the key declined", response, ok, err)
				}
				return
			}
			if err != nil || !ok {
				t.Fatalf("authenticate with agent identities enabled = %v, %v; want the agent", ok, err)
			}
			if got, ok := principal.FromUserInfo(response.User); !ok || got != principal.Agent(agent.ID) {
				t.Fatalf("authenticated as %#v, %v; want agent %d", got, ok, agent.ID)
			}
		})
	}
}
