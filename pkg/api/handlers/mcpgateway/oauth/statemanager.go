package oauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"golang.org/x/oauth2"
)

type stateManager struct {
	gatewayClient         *client.Client
	staticOAuthHTTPClient *http.Client
}

func newStateManager(gatewayClient *client.Client, staticOAuthHTTPClient ...*http.Client) *stateManager {
	manager := &stateManager{gatewayClient: gatewayClient}
	if len(staticOAuthHTTPClient) > 0 {
		manager.staticOAuthHTTPClient = staticOAuthHTTPClient[0]
	}
	return manager
}

func (sm *stateManager) store(ctx context.Context, userID, mcpID, mcpURL, oauthAuthRequestID, catalogEntryName, state, verifier string, conf *oauth2.Config) error {
	return sm.gatewayClient.CreateMCPOAuthPendingState(ctx, userID, mcpID, mcpURL, oauthAuthRequestID, catalogEntryName, state, verifier, conf)
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

	exchangeContext := ctx
	if ps.CatalogEntryName != "" && sm.staticOAuthHTTPClient != nil {
		exchangeContext = context.WithValue(ctx, oauth2.HTTPClient, sm.staticOAuthHTTPClient)
	}
	token, err := conf.Exchange(exchangeContext, code, oauth2.SetAuthURLParam("code_verifier", ps.Verifier))
	if err != nil {
		_ = sm.gatewayClient.DeleteMCPOAuthPendingState(ctx, ps.HashedState)
		return "", "", fmt.Errorf("failed to exchange code: %w", err)
	}

	if err := sm.gatewayClient.CommitMCPOAuthPendingStateToken(ctx, ps, ps.OAuthAuthRequestID, conf, token); err != nil {
		return "", "", err
	}

	return ps.OAuthAuthRequestID, ps.MCPID, nil
}
