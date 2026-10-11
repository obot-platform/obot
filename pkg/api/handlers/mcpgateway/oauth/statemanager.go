package oauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	"golang.org/x/oauth2"
)

// tokenClientFunc returns the HTTP client for the code exchange with a server-supplied
// token endpoint, going through the named tunnel when there is one.
type tokenClientFunc func(tunnelName string) (*http.Client, error)

type stateManager struct {
	gatewayClient *client.Client
	// tokenClient builds the client for the code exchange. When nil, or when it returns
	// no client, ExchangeOAuthToken falls back to its strict egress client.
	tokenClient tokenClientFunc
}

func newStateManager(gatewayClient *client.Client, tokenClient tokenClientFunc) *stateManager {
	return &stateManager{
		gatewayClient: gatewayClient,
		tokenClient:   tokenClient,
	}
}

func (sm *stateManager) store(ctx context.Context, userID, mcpID, mcpURL, tunnelName, oauthAuthRequestID, state, verifier, resourceURL string, conf *oauth2.Config) error {
	return sm.gatewayClient.CreateMCPOAuthPendingState(ctx, userID, mcpID, mcpURL, tunnelName, oauthAuthRequestID, state, verifier, resourceURL, conf)
}

func (sm *stateManager) createToken(ctx context.Context, state, code, errorStr, errorDescription string) (string, string, error) {
	ps, err := sm.gatewayClient.GetMCPOAuthPendingState(ctx, state)
	if err != nil {
		return "", "", fmt.Errorf("failed to get oauth state: %w", err)
	}

	if errorStr != "" {
		// Clean up the pending state before returning the error
		_ = sm.gatewayClient.DeleteMCPOAuthPendingState(ctx, ps.HashedState)
		return "", "", fmt.Errorf("error returned from oauth server: %s, %s", errorStr, errorDescription)
	}

	conf := &oauth2.Config{
		ClientID:     ps.ClientID,
		ClientSecret: ps.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   ps.AuthURL,
			TokenURL:  ps.TokenURL,
			AuthStyle: ps.AuthStyle,
		},
		RedirectURL: ps.RedirectURL,
	}
	if ps.Scopes != "" {
		conf.Scopes = strings.Split(ps.Scopes, " ")
	}

	exchangeCtx := ctx
	if sm.tokenClient != nil {
		httpClient, err := sm.tokenClient(ps.TunnelName)
		if err != nil {
			_ = sm.gatewayClient.DeleteMCPOAuthPendingState(ctx, ps.HashedState)
			return "", "", fmt.Errorf("failed to create OAuth token client: %w", err)
		}
		if httpClient != nil {
			exchangeCtx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
		}
	}
	token, err := mcp.ExchangeOAuthToken(exchangeCtx, conf, code, ps.Verifier, ps.ResourceURL)
	if err != nil {
		_ = sm.gatewayClient.DeleteMCPOAuthPendingState(ctx, ps.HashedState)
		return "", "", fmt.Errorf("failed to exchange code: %w", err)
	}

	// Save the completed token
	if err := sm.gatewayClient.ReplaceMCPOAuthToken(ctx, ps.UserID, ps.MCPID, ps.URL, ps.OAuthAuthRequestID, conf, token); err != nil {
		return "", "", err
	}

	// Delete the pending state
	_ = sm.gatewayClient.DeleteMCPOAuthPendingState(ctx, ps.HashedState)

	return ps.OAuthAuthRequestID, ps.MCPID, nil
}
