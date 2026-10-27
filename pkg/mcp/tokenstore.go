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

	token := &oauth2.Token{
		AccessToken:  mcpToken.AccessToken,
		RefreshToken: mcpToken.RefreshToken,
		TokenType:    mcpToken.TokenType,
		ExpiresIn:    mcpToken.ExpiresIn,
		Expiry:       mcpToken.Expiry,
	}

	return conf, token.WithExtra(map[string]any{"obot_grant_id": mcpToken.GrantID}), nil
}

func (t *tokenStore) RefreshTokenConfig(ctx context.Context, config *oauth2.Config, previous, token *oauth2.Token) error {
	grantID, _ := previous.Extra("obot_grant_id").(string)

	return t.gatewayClient.RefreshMCPOAuthToken(ctx, t.userID, t.mcpID, t.mcpURL, grantID, config, token)
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
