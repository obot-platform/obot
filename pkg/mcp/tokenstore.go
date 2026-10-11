package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type GlobalTokenStore interface {
	ForUserAndMCP(userID, mcpID, mcpURL string) TokenStorage
}

type TokenStorage interface {
	GetTokenConfig(context.Context) (*oauth2.Config, *oauth2.Token, error)
	SetTokenConfig(context.Context, *oauth2.Config, *oauth2.Token) error
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
	// httpClient performs the refresh request to the server-supplied token endpoint.
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

	conf := &oauth2.Config{
		ClientID:     mcpToken.ClientID,
		ClientSecret: mcpToken.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   mcpToken.AuthURL,
			TokenURL:  mcpToken.TokenURL,
			AuthStyle: mcpToken.AuthStyle,
		},
		RedirectURL: mcpToken.RedirectURL,
	}
	if mcpToken.Scopes != "" {
		conf.Scopes = strings.Split(mcpToken.Scopes, " ")
	}

	return conf, &oauth2.Token{
		AccessToken:  mcpToken.AccessToken,
		RefreshToken: mcpToken.RefreshToken,
		TokenType:    mcpToken.TokenType,
		ExpiresIn:    mcpToken.ExpiresIn,
		Expiry:       mcpToken.Expiry,
	}, nil
}

func (t *tokenStore) SetTokenConfig(ctx context.Context, config *oauth2.Config, token *oauth2.Token) error {
	return t.gatewayClient.ReplaceMCPOAuthToken(ctx, t.userID, t.mcpID, t.mcpURL, "", config, token)
}

func (t *tokenStore) DeleteTokenConfig(ctx context.Context) error {
	return t.gatewayClient.DeleteMCPOAuthTokenForURL(ctx, t.userID, t.mcpID, t.mcpURL)
}

func (t *tokenStore) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
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

func newStorageBackedTokenSource(tokenStorage TokenStorage, conf *oauth2.Config, tok *oauth2.Token, httpClient *http.Client) oauth2.TokenSource {
	if httpClient == nil {
		httpClient = strictEgressClient
	}
	return oauth2.ReuseTokenSource(tok, &storageBackedTokenSource{
		tokenStorage: tokenStorage,
		conf:         conf,
		tok:          tok,
		httpClient:   httpClient,
	})
}

func (ts *storageBackedTokenSource) Token() (*oauth2.Token, error) {
	ctx := withOAuthHTTPClient(context.Background(), ts.httpClient)
	tok, err := ts.conf.TokenSource(ctx, ts.tok).Token()
	if err != nil {
		return nil, err
	}

	ts.lock.Lock()
	defer ts.lock.Unlock()

	if tok.AccessToken != ts.tok.AccessToken || tok.RefreshToken != ts.tok.RefreshToken || tok.Expiry.Unix() != ts.tok.Expiry.Unix() {
		ts.tok = tok

		if ts.tokenStorage != nil {
			if err = ts.tokenStorage.SetTokenConfig(context.Background(), ts.conf, ts.tok); err != nil {
				return nil, fmt.Errorf("failed to store token: %w", err)
			}
		}
	}

	return ts.tok, nil
}
