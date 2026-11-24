package mcpgateway

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/obot-platform/mmmcp"
	"github.com/obot-platform/mmmcp/component"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/utils"
	"golang.org/x/oauth2"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	// compositeLoopbackTokenTTL bounds each token minted for a composite's
	// loopback and component connections.
	compositeLoopbackTokenTTL = 10 * time.Minute
)

// Only the owner replica's authenticated request supplies the OAuth lookup identity.
type compositeAPIContextKey struct{}

type compositeTokenService interface {
	DecodeToken(context.Context, string) (*persistent.TokenContext, error)
	NewToken(context.Context, persistent.TokenContext) (*jwt.Token, string, error)
	// DecodeDelegatedToken and NewDelegatedLoopbackToken read and issue the loopback tokens of an agent acting on
	// behalf of a user, which every other decoder refuses.
	DecodeDelegatedToken(context.Context, string) (*persistent.DelegatedToken, error)
	NewDelegatedLoopbackToken(context.Context, persistent.DelegatedToken, persistent.DelegatedLoopback, time.Duration) (string, time.Time, error)
}

// compositeComponentAuth authenticates a composite's component connections.
// Pooled component sessions outlive the frontend request that opened them and
// replay its headers on standalone SSE reconnects, so the composite loopback
// token from that request is re-minted with the same claims as it expires
// rather than passed through.
type compositeComponentAuth struct {
	ctx    context.Context
	tokens compositeTokenService
}

type compositeComponentOAuthHandler struct {
	tokenSource oauth2.TokenSource
}

type compositeLoopbackTokenSource struct {
	ctx          context.Context
	tokens       compositeTokenService
	tokenContext persistent.TokenContext
}

// delegatedLoopbackTokenSource re-mints the loopback token of an agent acting on behalf of a user, from the same
// delegation and minting key and with the same loopback claims as the token it replaces.
type delegatedLoopbackTokenSource struct {
	ctx        context.Context
	tokens     compositeTokenService
	delegation persistent.DelegatedToken
	loopback   persistent.DelegatedLoopback
}

func (a compositeComponentAuth) OAuthHandler(ctx context.Context, _ mmmcpconfig.Server) (auth.OAuthHandler, error) {
	bearer, ok := strings.CutPrefix(component.RequestHeadersFromContext(ctx).Get("Authorization"), "Bearer ")
	if !ok || bearer == "" {
		return nil, nil
	}

	// The loopback of an agent acting on behalf of a user carries its delegation, so that the composite's connections
	// to the vMCP's components act for the agent on behalf of the same user. Only a loopback token is accepted here:
	// never the token the agent itself presents, which is not scoped to a composite.
	if persistent.ClaimsDelegatedToken(bearer) {
		delegated, err := a.tokens.DecodeDelegatedToken(ctx, bearer)
		if err != nil || delegated.Loopback == nil || !slices.Contains(delegated.Loopback.Groups, types.GroupCompositeMCP) {
			return nil, nil
		}
		source := delegatedLoopbackTokenSource{
			ctx:        a.ctx,
			tokens:     a.tokens,
			delegation: *delegated,
			loopback:   *delegated.Loopback,
		}
		source.delegation.Loopback = nil
		return compositeComponentOAuthHandler{
			tokenSource: oauth2.ReuseTokenSource(&oauth2.Token{
				AccessToken: bearer,
				TokenType:   "Bearer",
				Expiry:      delegated.ExpiresAt,
			}, source),
		}, nil
	}

	tokenContext, err := a.tokens.DecodeToken(ctx, bearer)
	if err != nil || !slices.Contains(tokenContext.UserGroups, types.GroupCompositeMCP) {
		return nil, nil
	}

	return compositeComponentOAuthHandler{
		tokenSource: oauth2.ReuseTokenSource(&oauth2.Token{
			AccessToken: bearer,
			TokenType:   "Bearer",
			Expiry:      tokenContext.ExpiresAt.Time,
		}, compositeLoopbackTokenSource{
			ctx:          a.ctx,
			tokens:       a.tokens,
			tokenContext: *tokenContext,
		}),
	}, nil
}

func (h compositeComponentOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return h.tokenSource, nil
}

// Authorize fails so the component's challenge reaches the frontend client,
// which is the only party able to complete the component's authorization.
func (compositeComponentOAuthHandler) Authorize(_ context.Context, _ *http.Request, resp *http.Response) error {
	// A component can stall its error body indefinitely; fail authorization without draining it.
	_ = resp.Body.Close()

	return fmt.Errorf("component requires authorization: %s", resp.Status)
}

func (s compositeLoopbackTokenSource) Token() (*oauth2.Token, error) {
	token, expiresAt, err := newCompositeLoopbackToken(s.ctx, s.tokens, s.tokenContext, time.Now())
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken: token,
		TokenType:   "Bearer",
		Expiry:      expiresAt,
	}, nil
}

func (s delegatedLoopbackTokenSource) Token() (*oauth2.Token, error) {
	token, expiresAt, err := s.tokens.NewDelegatedLoopbackToken(s.ctx, s.delegation, s.loopback, compositeLoopbackTokenTTL)
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken: token,
		TokenType:   "Bearer",
		Expiry:      expiresAt,
	}, nil
}

// compositeLoopbackToken returns the token a composite server's loopback authenticates with, for caller, the caller
// that reached the composite server. For an agent acting on behalf of a user, it is a loopback token from the same
// delegation and minting key (persistent.TokenService.NewDelegatedLoopbackToken), so that the composite and the vMCP's
// components act for the agent on behalf of the same user, and the authenticator checks the delegation again at each
// hop. For any other caller, it is a token with the claims of compositeLoopbackTokenContext.
func compositeLoopbackToken(ctx context.Context, tokens compositeTokenService, caller user.Info, audience, mcpServerName string, authorizedMCPIDs []string, connectionID string, now time.Time) (string, error) {
	if _, ok := principal.OnBehalfOf(caller); ok {
		delegation, loopback, err := delegatedLoopback(caller, audience, mcpServerName, authorizedMCPIDs, connectionID)
		if err != nil {
			return "", err
		}
		token, _, err := tokens.NewDelegatedLoopbackToken(ctx, delegation, loopback, compositeLoopbackTokenTTL)
		return token, err
	}
	token, _, err := newCompositeLoopbackToken(ctx, tokens, compositeLoopbackTokenContext(caller, audience, mcpServerName, authorizedMCPIDs, connectionID, now), now)
	return token, err
}

// delegatedLoopback returns the delegation and the loopback claims of the loopback token of caller, an agent acting on
// behalf of a user: the delegation and minting key that authenticated it, and the composite loopback's claims, as
// compositeLoopbackTokenContext sets them for the agent acting for itself. A caller without a delegation or a minting
// key gets no token.
func delegatedLoopback(caller user.Info, audience, mcpServerName string, authorizedMCPIDs []string, connectionID string) (persistent.DelegatedToken, persistent.DelegatedLoopback, error) {
	acting, ok := principal.DelegatedOwnerOf(caller)
	if !ok {
		return persistent.DelegatedToken{}, persistent.DelegatedLoopback{}, fmt.Errorf("the caller does not act on behalf of a user")
	}
	delegation, _ := agentprofile.DelegationOf(caller)
	key, hasKey := principal.APIKeyAttributionFromUser(caller)
	if delegation.ID == "" || !hasKey {
		return persistent.DelegatedToken{}, persistent.DelegatedLoopback{}, fmt.Errorf("the agent acting on behalf of a user has no delegation or minting key")
	}
	return persistent.DelegatedToken{
		AgentID:      acting.AgentID,
		UserID:       acting.UserID,
		DelegationID: delegation.ID,
		APIKeyID:     key.ID,
	}, persistent.DelegatedLoopback{
		Audience:          audience,
		Groups:            []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
		MCPID:             mcpServerName,
		AuthorizedMCPIDs:  authorizedMCPIDs,
		OAuthConnectionID: connectionID,
		OAuthClientID:     cmp.Or(caller.GetExtra()["oauth_client_id"]...),
		APIKeyName:        key.Name,
		APIKeyRawName:     cmp.Or(caller.GetExtra()[principal.APIKeyRawNameExtra]...),
	}, nil
}

func newCompositeLoopbackToken(ctx context.Context, tokens compositeTokenService, tokenContext persistent.TokenContext, now time.Time) (string, time.Time, error) {
	expiresAt := now.Add(compositeLoopbackTokenTTL)
	tokenContext.IssuedAt = persistent.NewTime(now)
	tokenContext.ExpiresAt = persistent.NewTime(expiresAt)
	_, token, err := tokens.NewToken(ctx, tokenContext)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func compositeConfig(req api.Context, serverConfig mcp.ServerConfig) (*mmmcpconfig.Config, error) {
	if len(serverConfig.Components) > 0 {
		// Components that use the vMCP's preconfigured OAuth use its grants, whoever the caller is, and the others the
		// caller's own tokens, which an agent has none of. Discovery is separated by both, so that authorizing,
		// revoking, reauthorizing or invalidating a grant, as much as a user's own authorization, reconnects the
		// components with the token they now use.
		var (
			ids  []string
			keys []gatewaytypes.MCPOAuthTokenKey
		)
		for _, component := range serverConfig.Components {
			if component.OAuthGrant != nil {
				keys = append(keys, gatewaytypes.MCPOAuthTokenKey{UserID: component.OAuthGrant.Owner, MCPID: component.OAuthGrant.MCPID})
			} else {
				ids = append(ids, component.ConnectID())
			}
		}

		var (
			revision string
			err      error
		)
		if principal.ClaimsAgent(req.User) {
			revision, err = req.GatewayClient.MCPOAuthGrantDiscoveryRevision(req.Context(), keys)
			if delegated, ok := principal.DelegatedOwnerOf(req.User); ok && err == nil {
				// On behalf of a user, the components that run on the pair's connections use the accounts the user
				// connected through the agent, so connecting or replacing one discovers their tools again. A
				// connection of the agent's own has no token of the pair's, and a component with preconfigured OAuth
				// uses the vMCP's grant, never the pair's token.
				var pairRevision string
				pairRevision, err = req.GatewayClient.MCPOAuthDiscoveryRevision(req.Context(), delegated.String(), ids)
				revision = utils.Digest([]string{revision, pairRevision})
			}
		} else if revision, err = req.GatewayClient.MCPOAuthDiscoveryRevision(req.Context(), req.User.GetUID(), ids); err == nil && len(keys) > 0 {
			var grants string
			grants, err = req.GatewayClient.MCPOAuthGrantDiscoveryRevision(req.Context(), keys)
			revision = utils.Digest([]string{revision, grants})
		}
		if err != nil {
			return nil, fmt.Errorf("read component OAuth discovery revision: %w", err)
		}

		// Consult shared state on each request so revocation also invalidates other replicas.
		serverConfig.ConfigHash = utils.Digest([]string{serverConfig.ConfigHash, revision})
	}

	cfg := mcp.MMMCPConfig(serverConfig, nil)
	// Deferred authorization offers sign-in links to the user of a Claude environment. No agent is one, and an agent
	// is never offered a link.
	cfg.DeferAuthorization = authz.IsClaudeEnvironment(req.User) && !principal.ClaimsAgent(req.User)

	return cfg, nil
}

func compositeAuthorizationURLs(lookup func(api.Context) ([]mmmcp.AuthorizationURL, error)) func(context.Context) ([]mmmcp.AuthorizationURL, error) {
	return func(ctx context.Context) ([]mmmcp.AuthorizationURL, error) {
		req, ok := ctx.Value(compositeAPIContextKey{}).(api.Context)
		if !ok || !authz.IsClaudeEnvironment(req.User) || lookup == nil {
			return nil, fmt.Errorf("authorization for this Claude environment is unavailable")
		}
		req.Request = req.WithContext(ctx)
		return lookup(req)
	}
}
