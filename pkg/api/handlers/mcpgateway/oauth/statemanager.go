package oauth

import (
	"context"
	"fmt"
	"strings"
	"sync"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"golang.org/x/oauth2"
)

type stateObj struct {
	verifier, userID, mcpID, mcpURL, oauthAuthRequestID string
	conf                                                *oauth2.Config
	ch                                                  chan<- nmcp.CallbackPayload
}
type stateCache struct {
	lock          sync.Mutex
	cache         map[string]stateObj
	gatewayClient *client.Client
}

func newStateCache(gatewayClient *client.Client) *stateCache {
	return &stateCache{
		gatewayClient: gatewayClient,
		cache:         make(map[string]stateObj),
	}
}

func (sm *stateCache) store(ctx context.Context, userID, mcpID, mcpURL, oauthAuthRequestID, state, verifier string, conf *oauth2.Config, ch chan<- nmcp.CallbackPayload) error {
	if err := sm.gatewayClient.ReplaceMCPOAuthToken(ctx, userID, mcpID, mcpURL, oauthAuthRequestID, state, verifier, conf, &oauth2.Token{}); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	sm.lock.Lock()
	sm.cache[state] = stateObj{
		conf:               conf,
		verifier:           verifier,
		userID:             userID,
		mcpID:              mcpID,
		mcpURL:             mcpURL,
		oauthAuthRequestID: oauthAuthRequestID,
		ch:                 ch,
	}
	sm.lock.Unlock()
	return nil
}

func (sm *stateCache) createToken(ctx context.Context, state, code, errorStr, errorDescription string) (string, string, error) {
	sm.lock.Lock()
	s, ok := sm.cache[state]
	delete(sm.cache, state)
	sm.lock.Unlock()

	var (
		userID, mcpID, mcpURL, verifier, oauthAuthRequestID string
		conf                                                *oauth2.Config
	)
	if ok {
		defer close(s.ch)

		mcpID = s.mcpID
		mcpURL = s.mcpURL
		userID = s.userID
		oauthAuthRequestID = s.oauthAuthRequestID
		verifier = s.verifier
		conf = s.conf
	} else {
		token, err := sm.gatewayClient.GetMCPOAuthTokenByState(ctx, state)
		if err != nil {
			return "", "", fmt.Errorf("failed to get oauth state: %w", err)
		}

		conf = &oauth2.Config{
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

		oauthAuthRequestID = token.OAuthAuthRequestID
		userID = token.UserID
		mcpID = token.MCPID
		mcpURL = token.URL
		verifier = token.Verifier
	}

	if errorStr != "" {
		return "", "", fmt.Errorf("error returned from oauth server: %s, %s", errorStr, errorDescription)
	}

	token, err := conf.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return "", "", fmt.Errorf("failed to exchange code: %w", err)
	}

	return oauthAuthRequestID, mcpID, sm.gatewayClient.ReplaceMCPOAuthToken(ctx, userID, mcpID, mcpURL, "", "", "", conf, token)
}
