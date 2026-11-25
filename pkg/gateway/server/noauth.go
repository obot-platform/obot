package server

import (
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/union"
)

// WithNoAuth ends the authenticator chain of a server that does not authenticate callers. A request that credentials
// authenticates is that principal's, and every other request is the "nobody" owner's (authn.NoAuth), as before, with
// one exception: agents' credentials fail closed.
//
// An agent authenticates there as it does with authentication enabled, with its API key (oak1-) or a token the token
// service issued it, as its agent and with the agent's restrictions. A credential the agent was refused (an unknown,
// revoked, expired or malformed key or token, or one of a disabled or deleted agent) must never become the "nobody"
// owner, who has every right. credentials is a union.New chain, which goes on to the next authenticator after an
// error, so the agents' authenticators run after it, in a part of the chain that stops at its first error and ahead of
// authn.NoAuth:
//   - AgentAPIKeyAuthenticator authenticates a request that presents an agent API key in any header that
//     APIKeyAuthenticator reads, or refuses it.
//   - tokens.AgentTokens refuses a token that claims to be an agent's and that credentials did not authenticate. The
//     token service in credentials authenticates a valid one first; its refusal of one would otherwise fall through.
//
// Users' API keys (ok1-) are not checked without authentication, and a request with one, valid or not, is the
// "nobody" owner's, as any other request is.
func WithNoAuth(credentials authenticator.Request, gatewayClient *client.Client, tokens *persistent.TokenService) authenticator.Request {
	return union.New(credentials, union.NewFailOnError(
		NewAgentAPIKeyAuthenticator(gatewayClient),
		tokens.AgentTokens(),
		authn.NewNoAuth(gatewayClient),
	))
}
