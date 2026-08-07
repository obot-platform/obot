package mcp

import (
	"context"
	"errors"
	"strings"

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
}

func NewGlobalTokenStore(gatewayClient *gateway.Client) GlobalTokenStore {
	return &globalTokenStore{
		gatewayClient: gatewayClient,
	}
}

type globalTokenStore struct {
	gatewayClient *gateway.Client
}

func (g *globalTokenStore) ForUserAndMCP(userID, mcpID, mcpURL string) TokenStorage {
	return &tokenStore{
		gatewayClient: g.gatewayClient,
		mcpID:         mcpID,
		userID:        userID,
		mcpURL:        mcpURL,
	}
}

type tokenStore struct {
	gatewayClient         *gateway.Client
	userID, mcpID, mcpURL string
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
