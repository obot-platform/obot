package server

import (
	"errors"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/pkg/api/authn"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
)

// TestChainsRefuseDelegatedTokens checks, across the authenticator chains as the server composes them, with and
// without authentication, that a token with which an agent acts on behalf of a user from a delegation that does not
// exist authenticates no one: not the user it names as its subject, not the agent, not the anonymous caller and not
// the "nobody" owner. On the routes that accept such tokens the delegated token authenticator refuses it, and on every
// other route the token service does, in either header the token service reads. TestChainsAcceptDelegatedTokensOnly
// OnTheirRoutes covers a valid delegation.
func TestChainsRefuseDelegatedTokens(t *testing.T) {
	s, client := newTokenRequestTestServer(t)
	ctx := t.Context()

	noAuth, tokenService := noAuthChain(t, s, client)
	// The chain with authentication, from the credentials agents and users present without a browser session.
	authenticated, _ := agentChain(t, client)

	person := createAPIKeyLifecycleTestUser(t, client, "ola")
	agent := createAgentWithUsersID(t, client, person)
	key, err := client.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "assistant", CanAccessLLMProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := tokenService.NewDelegatedToken(ctx, agent.ID, person.ID, "adl1test", key.ID, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, chain := range []struct {
		name  string
		chain *authn.AdmissionCheck
	}{
		{
			name:  "without authentication",
			chain: noAuth,
		},
		{
			name:  "with authentication",
			chain: authenticated,
		},
	} {
		for _, header := range []string{"Authorization", "X-API-Key"} {
			for _, path := range []string{noAuthTestLLMPath, "/mcp-connect/vmcp1test", "/api/me", "/api/skills"} {
				t.Run(chain.name+" "+header+" "+path, func(t *testing.T) {
					value := token
					if header == "Authorization" {
						value = "Bearer " + token
					}
					req := noAuthTestRequest(t, path, map[string]string{header: value})
					req.Method = http.MethodGet
					response, ok, err := chain.chain.AuthenticateRequest(req)
					if err == nil {
						name := ""
						if response != nil && response.User != nil {
							name = response.User.GetName()
						}
						t.Fatalf("authenticated as %q, %v; want the token refused", name, ok)
					}
					if ok || response != nil {
						t.Fatalf("got %v, %v alongside the error %v", response, ok, err)
					}
					// Without authentication, the token service refuses it first, and the agent token guard after it.
					if !errors.Is(err, persistent.ErrDelegatedToken) && chain.name == "with authentication" {
						t.Fatalf("error = %v, want the token service's refusal", err)
					}
				})
			}
		}
	}
}
