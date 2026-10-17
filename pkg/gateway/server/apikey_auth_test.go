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

	response, ok, err := NewAPIKeyAuthenticator(client).AuthenticateRequest(req)
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

	response, ok, err := NewAPIKeyAuthenticator(client).AuthenticateRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || response == nil || response.User == nil {
		t.Fatal("expected API key to authenticate")
	}
	attribution, ok := principal.APIKeyAttributionFromUser(response.User)
	wantName := fmt.Sprintf("ok1-%d-%d-*****", keyOwner.ID, created.ID)
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
			response, ok, err := NewAPIKeyAuthenticator(client).AuthenticateRequest(request)
			if err != nil || !ok {
				t.Fatalf("authenticate: %v, %v", ok, err)
			}
			if got := slices.Contains(response.User.GetGroups(), types2.GroupClaudeEnvironment); got != test.want {
				t.Fatalf("Claude environment group = %v, want %v", got, test.want)
			}
		})
	}
}
