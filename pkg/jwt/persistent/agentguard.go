package persistent

import (
	"errors"
	"net/http"

	"k8s.io/apiserver/pkg/authentication/authenticator"
)

var (
	// errInvalidAgentToken refuses a token that claims to be an agent's and that the token service did not
	// authenticate.
	errInvalidAgentToken = errors.New("invalid agent token")
)

// AgentTokens returns an authenticator that refuses every token that claims to be an agent's (ClaimsAgentToken),
// including one with which an agent acts on behalf of a user. It fails rather than declines, and authenticates
// nothing, so it is only correct after t, which authenticates a valid agent token first.
//
// It is for an authenticator chain that ends in a catch-all, such as the one of a server that does not authenticate
// callers, which ends with every caller as the "nobody" administrator. A token that the token service refused there,
// because it is expired, is not one it issues to agents, or is for a disabled or deleted agent, would otherwise end
// the chain as that administrator, and an agent would be given every right that it was refused. Run it after the token
// service and ahead of the catch-all, in a part of the chain that stops at its first error (union.NewFailOnError). The
// token service's own error, such as the denial of a disabled agent, stays in the chain's errors.
//
// It declines a request with no token, and one whose token does not claim to be an agent's.
func (t *TokenService) AgentTokens() authenticator.Request {
	return authenticator.RequestFunc(func(req *http.Request) (*authenticator.Response, bool, error) {
		if !ClaimsAgentToken(RequestToken(req)) {
			return nil, false, nil
		}
		return nil, false, errInvalidAgentToken
	})
}
