package mcpgateway

import (
	"context"
	"fmt"
	"io"
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
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/utils"
	"golang.org/x/oauth2"
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

func (a compositeComponentAuth) OAuthHandler(ctx context.Context, _ mmmcpconfig.Server) (auth.OAuthHandler, error) {
	bearer, ok := strings.CutPrefix(component.RequestHeadersFromContext(ctx).Get("Authorization"), "Bearer ")
	if !ok || bearer == "" {
		return nil, nil
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
	_, _ = io.Copy(io.Discard, resp.Body)
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
		ids := make([]string, 0, len(serverConfig.Components))
		for _, component := range serverConfig.Components {
			ids = append(ids, component.ConnectID())
		}

		revision, err := req.GatewayClient.MCPOAuthDiscoveryRevision(req.Context(), req.User.GetUID(), ids)
		if err != nil {
			return nil, fmt.Errorf("read component OAuth discovery revision: %w", err)
		}

		// Consult shared state on each request so revocation also invalidates other replicas.
		serverConfig.ConfigHash = utils.Digest([]string{serverConfig.ConfigHash, revision})
	}

	cfg := mcp.MMMCPConfig(serverConfig, nil)
	cfg.DeferAuthorization = authz.IsClaudeEnvironment(req.User)

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
