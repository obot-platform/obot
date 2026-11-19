package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const (
	// sharedGrantRefreshTimeout bounds waiting for, and holding, the refresh lock of a vMCP's agent OAuth grant.
	sharedGrantRefreshTimeout = time.Minute
	// sharedGrantSaveAttempts is how many times a refreshed grant is saved before the refresh is given up. The
	// provider may have rotated the refresh token, so a refresh that is not saved can leave the grant unusable.
	sharedGrantSaveAttempts = 3
)

var (
	// ErrOAuthGrantInvalid reports that a vMCP's agent OAuth grant no longer works: it was revoked, its target
	// changed, or the provider refused to refresh it. An administrator must authorize it again; until then no agent
	// that uses it can reach the component, and none connects to it without the grant.
	ErrOAuthGrantInvalid = errors.New("the vMCP's agent OAuth grant is no longer valid and must be authorized again")
	// ErrOAuthGrantUnavailable reports that a vMCP's agent OAuth grant could not be refreshed for a reason that may
	// pass, such as the provider being unreachable or failing. The grant is left valid.
	ErrOAuthGrantUnavailable = errors.New("the vMCP's agent OAuth grant could not be refreshed")

	errAgentOAuthGrantWrite = errors.New("a vMCP's agent OAuth grant is written only through its administrator's authorization")
)

type GlobalTokenStore interface {
	ForUserAndMCP(userID, mcpID, mcpURL string) TokenStorage
}

type TokenStorage interface {
	GetTokenConfig(context.Context) (*oauth2.Config, *oauth2.Token, error)
	SetTokenConfig(context.Context, *oauth2.Config, *oauth2.Token) error
	RefreshTokenConfig(context.Context, *oauth2.Config, *oauth2.Token, *oauth2.Token) error
	DeleteTokenConfig(context.Context) error
	TokenSource(context.Context) (oauth2.TokenSource, error)
}

type globalTokenStore struct {
	gatewayClient *gateway.Client
}

type tokenStore struct {
	gatewayClient         *gateway.Client
	userID, mcpID, mcpURL string
}

// storageBackedTokenSource implements the oauth2.TokenSource interface to store new tokens in the TokenStorage.
type storageBackedTokenSource struct {
	lock         sync.Mutex
	tokenStorage TokenStorage
	conf         *oauth2.Config
	tok          *oauth2.Token
	// Keep the original grant identity even when a refresh replaces the token response.
	grantToken *oauth2.Token
	// Retain rotated tokens across failed saves instead of refreshing with a consumed token.
	source oauth2.TokenSource
}

// sharedGrantRefresher refreshes a vMCP's agent OAuth grant for one connection, under the grant's refresh lock. It reads
// the grant, and the client it was issued to, again under the lock, so a grant replaced meanwhile is refreshed with
// its own client.
type sharedGrantRefresher struct {
	store      *tokenStore
	httpClient *http.Client
}

func NewGlobalTokenStore(gatewayClient *gateway.Client) GlobalTokenStore {
	return &globalTokenStore{
		gatewayClient: gatewayClient,
	}
}

func (g *globalTokenStore) ForUserAndMCP(userID, mcpID, mcpURL string) TokenStorage {
	return &tokenStore{
		gatewayClient: g.gatewayClient,
		mcpID:         mcpID,
		userID:        userID,
		mcpURL:        mcpURL,
	}
}

func (t *tokenStore) GetTokenConfig(ctx context.Context) (*oauth2.Config, *oauth2.Token, error) {
	mcpToken, err := t.gatewayClient.GetMCPOAuthToken(ctx, t.userID, t.mcpID, t.mcpURL)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// A grant for another resource must not supply credentials or scopes to this connection.
	if mcpToken.URL != t.mcpURL {
		return nil, nil, nil
	}

	token := grantToken(mcpToken).WithExtra(map[string]any{"obot_grant_id": mcpToken.GrantID})
	return grantOAuthConfig(mcpToken), token, nil
}

func (t *tokenStore) RefreshTokenConfig(ctx context.Context, config *oauth2.Config, previous, token *oauth2.Token) error {
	grantID, _ := previous.Extra("obot_grant_id").(string)

	return t.gatewayClient.RefreshMCPOAuthToken(ctx, t.userID, t.mcpID, t.mcpURL, grantID, config, token)
}

func (t *tokenStore) SetTokenConfig(ctx context.Context, config *oauth2.Config, token *oauth2.Token) error {
	if principal.IsAgentOAuthGrantOwner(t.userID) {
		// A vMCP's grant is stored only by its administrator's callback, bound to who authorized it.
		return errAgentOAuthGrantWrite
	}
	return t.gatewayClient.ReplaceMCPOAuthToken(ctx, t.userID, t.mcpID, t.mcpURL, "", config, token)
}

func (t *tokenStore) DeleteTokenConfig(ctx context.Context) error {
	if principal.IsAgentOAuthGrantOwner(t.userID) {
		// A vMCP's grant is deleted only with its component or vMCP, or when an administrator revokes it.
		return errAgentOAuthGrantWrite
	}
	return t.gatewayClient.DeleteMCPOAuthTokenForURL(ctx, t.userID, t.mcpID, t.mcpURL)
}

func (t *tokenStore) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	if principal.IsAgentOAuthGrantOwner(t.userID) {
		return t.sharedGrantTokenSource(ctx)
	}
	config, token, err := t.GetTokenConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get token config: %w", err)
	}

	if config == nil || token == nil {
		return nil, nil
	}
	// Refreshes reuse the HTTP client the caller put in ctx with withOAuthHTTPClient, so
	// they follow the same egress policy and network path as the rest of the connection.
	httpClient, _ := ctx.Value(oauth2.HTTPClient).(*http.Client)
	return newStorageBackedTokenSource(t, config, token, httpClient), nil
}

// sharedGrantTokenSource returns the token source of a vMCP's agent OAuth grant, which many agents, on every replica,
// use at once. A grant marked invalid has none: ErrOAuthGrantInvalid.
//
// Its refresh is not the per-connection one of a user's token, which would let every caller refresh the same
// refresh token, and every caller but the first be refused when the provider rotates it. Instead, a caller that finds
// the token expired takes the grant's refresh lock (gateway.Client.WithMCPOAuthRefreshLock), reads the grant again,
// and uses the token it finds if another caller refreshed it meanwhile; only otherwise does it refresh, and it saves
// the result before releasing the lock.
//
// The store is only asked for the grant selected for an agent, so a grant that is gone was revoked, and one for another
// target no longer matches its component: both are ErrOAuthGrantInvalid, as is a grant marked invalid.
func (t *tokenStore) sharedGrantTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	grant, err := t.gatewayClient.GetMCPOAuthToken(ctx, t.userID, t.mcpID, t.mcpURL)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOAuthGrantInvalid
	} else if err != nil {
		return nil, fmt.Errorf("%w: read grant: %v", ErrOAuthGrantUnavailable, err)
	}
	if !t.usableGrant(grant) {
		return nil, ErrOAuthGrantInvalid
	}
	// Refreshes use the HTTP client the caller put in ctx, as a user's do.
	httpClient, _ := ctx.Value(oauth2.HTTPClient).(*http.Client)
	if httpClient == nil {
		httpClient = strictEgressClient
	}
	return oauth2.ReuseTokenSource(grantToken(grant), &sharedGrantRefresher{
		store:      t,
		httpClient: httpClient,
	}), nil
}

func (r *sharedGrantRefresher) Token() (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sharedGrantRefreshTimeout)
	defer cancel()

	t := r.store
	var (
		result *oauth2.Token
		// refused is why the grant gives no token. fn reports it here rather than returning it, because an error
		// returned from fn rolls back the lock's transaction, and with it the grant being marked invalid.
		refused error
	)
	err := t.gatewayClient.WithMCPOAuthRefreshLock(ctx, t.userID, t.mcpID, func(ctx context.Context, locked *gateway.MCPOAuthRefresh) error {
		current, err := locked.Token(ctx, t.mcpURL)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Revoked while the connection used it.
			refused = ErrOAuthGrantInvalid
			return nil
		} else if err != nil {
			return fmt.Errorf("%w: read grant: %v", ErrOAuthGrantUnavailable, err)
		}
		if !t.usableGrant(current) {
			refused = ErrOAuthGrantInvalid
			return nil
		}

		conf := grantOAuthConfig(current)
		token := grantToken(current)
		if token.Valid() {
			// Another caller, on this replica or another, refreshed the grant while this one waited.
			result = token
			return nil
		}
		if token.RefreshToken == "" {
			r.invalidate(locked, current, "the access token expired and the grant has no refresh token")
			refused = ErrOAuthGrantInvalid
			return nil
		}

		refreshed, err := conf.TokenSource(withOAuthHTTPClient(ctx, r.httpClient), token).Token()
		if err != nil {
			if RefusedRefresh(err) {
				r.invalidate(locked, current, "the provider refused to refresh the grant (invalid_grant)")
				refused = ErrOAuthGrantInvalid
				return nil
			}
			// Anything else, such as a provider outage, may pass, so the grant stays valid. Only the status of the
			// refusal is reported: its body is the provider's.
			refused = fmt.Errorf("%w: %s", ErrOAuthGrantUnavailable, refreshFailure(err))
			return nil
		}

		for attempt := 1; ; attempt++ {
			err = locked.Save(ctx, t.mcpURL, current.GrantID, conf, refreshed)
			if err == nil || attempt == sharedGrantSaveAttempts || ctx.Err() != nil {
				break
			}
		}
		if err != nil {
			// The provider may have rotated the refresh token, so this connection keeps the token it got, and later
			// callers may find the grant refused; nothing better can be done without the database.
			slog.Error("Failed to save a refreshed vMCP agent OAuth grant", "owner", t.userID, "mcpID", t.mcpID, "error", err)
		}
		result = refreshed
		return nil
	})
	switch {
	case err != nil:
		return nil, err
	case refused != nil:
		return nil, refused
	}
	return result, nil
}

// invalidate marks the grant invalid for everyone who uses it, logging rather than failing if it cannot.
func (r *sharedGrantRefresher) invalidate(locked *gateway.MCPOAuthRefresh, grant *gatewaytypes.MCPOAuthToken, reason string) {
	t := r.store
	marked, err := locked.Invalidate(grant.GrantID, reason)
	if err != nil {
		slog.Error("Failed to mark a vMCP agent OAuth grant invalid", "owner", t.userID, "mcpID", t.mcpID, "error", err)
		return
	}
	if marked {
		slog.Warn("Marked a vMCP agent OAuth grant invalid; an administrator must authorize it again", "owner", t.userID, "mcpID", t.mcpID, "reason", reason)
	}
}

// RefusedRefresh reports whether err is the provider refusing to refresh a token with invalid_grant: the account was
// revoked, or the refresh token expired. Only authorizing the account again fixes it.
func RefusedRefresh(err error) bool {
	retrieveErr, ok := errors.AsType[*oauth2.RetrieveError](err)
	return ok && retrieveErr.ErrorCode == "invalid_grant"
}

// refreshFailure describes a failed refresh without the provider's response body or any token.
func refreshFailure(err error) string {
	if retrieveErr, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		if retrieveErr.Response != nil {
			return fmt.Sprintf("token endpoint answered %d %s", retrieveErr.Response.StatusCode, retrieveErr.ErrorCode)
		}
		return "token endpoint refused the refresh " + retrieveErr.ErrorCode
	}
	return "token endpoint unreachable"
}

// usableGrant reports whether grant, a vMCP's agent OAuth grant, is for the URL this store's connection reaches and is not
// marked invalid.
func (t *tokenStore) usableGrant(grant *gatewaytypes.MCPOAuthToken) bool {
	return grant.URL == t.mcpURL && grant.InvalidatedAt == nil
}

// grantToken is the OAuth token a stored token holds.
func grantToken(token *gatewaytypes.MCPOAuthToken) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresIn:    token.ExpiresIn,
		Expiry:       token.Expiry,
	}
}

// grantOAuthConfig is the OAuth client configuration a stored token was issued to.
func grantOAuthConfig(token *gatewaytypes.MCPOAuthToken) *oauth2.Config {
	conf := &oauth2.Config{
		ClientID:     token.ClientID,
		ClientSecret: token.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   token.AuthURL,
			TokenURL:  token.TokenURL,
			AuthStyle: token.AuthStyle,
		},
		RedirectURL: token.RedirectURL,
	}
	if token.Scopes != "" {
		conf.Scopes = strings.Split(token.Scopes, " ")
	}
	return conf
}

func newStorageBackedTokenSource(tokenStorage TokenStorage, conf *oauth2.Config, tok *oauth2.Token, httpClient *http.Client) oauth2.TokenSource {
	if httpClient == nil {
		httpClient = strictEgressClient
	}

	ctx := withOAuthHTTPClient(context.Background(), httpClient)

	return oauth2.ReuseTokenSource(tok, &storageBackedTokenSource{
		tokenStorage: tokenStorage,
		conf:         conf,
		tok:          tok,
		grantToken:   tok,
		source:       conf.TokenSource(ctx, tok),
	})
}

func (ts *storageBackedTokenSource) Token() (*oauth2.Token, error) {
	tok, err := ts.source.Token()
	if err != nil {
		return nil, err
	}

	ts.lock.Lock()
	defer ts.lock.Unlock()

	if tok.AccessToken != ts.tok.AccessToken || tok.RefreshToken != ts.tok.RefreshToken || tok.Expiry.Unix() != ts.tok.Expiry.Unix() {
		if ts.tokenStorage != nil {
			if err = ts.tokenStorage.RefreshTokenConfig(context.Background(), ts.conf, ts.grantToken, tok); err != nil {
				return nil, fmt.Errorf("failed to store token: %w", err)
			}
		}

		ts.tok = tok
	}

	return ts.tok, nil
}
