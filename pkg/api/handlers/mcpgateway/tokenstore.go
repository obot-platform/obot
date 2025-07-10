package mcpgateway

import (
	"context"
	"errors"
	"strings"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type GlobalTokenStore interface {
	ForMCPID(mcpID string) nmcp.TokenStorage
}

func NewGlobalTokenStore(gatewayClient *gateway.Client) GlobalTokenStore {
	return &globalTokenStore{
		gatewayClient: gatewayClient,
	}
}

type globalTokenStore struct {
	gatewayClient *gateway.Client
}

func (g *globalTokenStore) ForMCPID(mcpID string) nmcp.TokenStorage {
	return &tokenStore{
		gatewayClient: g.gatewayClient,
		mcpID:         mcpID,
	}
}

type tokenStore struct {
	gatewayClient *gateway.Client
	mcpID         string
}

func (t *tokenStore) GetTokenConfig(ctx context.Context, _ string) (*oauth2.Config, *oauth2.Token, error) {
	mcpToken, err := t.gatewayClient.GetMCPOAuthToken(ctx, t.mcpID)
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
		AccessToken: mcpToken.AccessToken,
		TokenType:   mcpToken.TokenType,
		ExpiresIn:   mcpToken.ExpiresIn,
		Expiry:      mcpToken.Expiry,
	}, nil
}

func (t *tokenStore) SetTokenConfig(ctx context.Context, _ string, config *oauth2.Config, token *oauth2.Token) error {
	return t.gatewayClient.ReplaceMCPOAuthToken(ctx, t.mcpID, "", "", "", config, token)
}
