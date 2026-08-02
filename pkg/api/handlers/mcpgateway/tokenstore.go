package mcpgateway

import (
	"context"
	"errors"
	"strings"
	"sync"

	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

func NewGlobalTokenStore(gatewayClient *gateway.Client) mcp.GlobalTokenStore {
	return &globalTokenStore{
		gatewayClient: gatewayClient,
	}
}

type globalTokenStore struct {
	gatewayClient *gateway.Client
}

func (g *globalTokenStore) ForUserAndMCP(userID, mcpID string) nmcp.TokenStorage {
	return &tokenStore{
		gatewayClient: g.gatewayClient,
		mcpID:         mcpID,
		userID:        userID,
	}
}

type tokenStore struct {
	gatewayClient *gateway.Client
	userID, mcpID string
	mu            sync.Mutex
	catalogEntry  map[string]catalogCredentialFence
}

type catalogCredentialFence struct {
	entryName  string
	generation string
}

func (t *tokenStore) GetTokenConfig(ctx context.Context, mcpURL string) (*oauth2.Config, *oauth2.Token, error) {
	mcpToken, err := t.gatewayClient.GetMCPOAuthToken(ctx, t.userID, t.mcpID, mcpURL)
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
	catalogEntryName := mcpToken.CatalogEntryName
	if catalogEntryName == "" {
		catalogEntryName, err = t.gatewayClient.CatalogEntryForCurrentOAuthCredential(ctx, t.userID, t.mcpID, mcpURL, conf)
		if err != nil {
			return nil, nil, err
		}
	} else if err := t.gatewayClient.ValidateCatalogOAuthToken(ctx, t.mcpID, mcpURL, catalogEntryName, mcpToken.CatalogCredentialGeneration, conf); err != nil {
		return nil, nil, err
	}
	t.mu.Lock()
	if t.catalogEntry == nil {
		t.catalogEntry = map[string]catalogCredentialFence{}
	}
	t.catalogEntry[mcpURL] = catalogCredentialFence{entryName: catalogEntryName, generation: mcpToken.CatalogCredentialGeneration}
	t.mu.Unlock()

	return conf, &oauth2.Token{
		AccessToken:  mcpToken.AccessToken,
		RefreshToken: mcpToken.RefreshToken,
		TokenType:    mcpToken.TokenType,
		ExpiresIn:    mcpToken.ExpiresIn,
		Expiry:       mcpToken.Expiry,
	}, nil
}

func (t *tokenStore) SetTokenConfig(ctx context.Context, mcpURL string, config *oauth2.Config, token *oauth2.Token) error {
	t.mu.Lock()
	fence, captured := t.catalogEntry[mcpURL]
	t.mu.Unlock()
	if !captured {
		var err error
		fence.entryName, err = t.gatewayClient.CatalogEntryForCurrentOAuthCredential(ctx, t.userID, t.mcpID, mcpURL, config)
		if err != nil {
			return err
		}
	}
	return t.gatewayClient.ReplaceMCPOAuthTokenWithCatalogCredentialGenerationFence(ctx, t.userID, t.mcpID, mcpURL, "", fence.entryName, fence.generation, config, token)
}

func (t *tokenStore) DeleteTokenConfig(ctx context.Context, mcpURL string) error {
	return t.gatewayClient.DeleteMCPOAuthTokenForURL(ctx, t.userID, t.mcpID, mcpURL)
}
