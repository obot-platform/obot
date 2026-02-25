package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const pendingStateTTL = 10 * time.Minute

type stateObj struct {
	verifier, userID, mcpID, mcpURL, oauthAuthRequestID, authURL string
	conf                                                         *oauth2.Config
	ch                                                           chan<- nmcp.CallbackPayload
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
	authURL, err := pendingAuthURL(conf, state, verifier, mcpURL)
	if err != nil {
		return fmt.Errorf("failed to build pending auth url: %w", err)
	}

	if err = sm.gatewayClient.ReplaceMCPOAuthToken(ctx, userID, mcpID, mcpURL, oauthAuthRequestID, state, verifier, authURL, time.Now().Add(pendingStateTTL), conf, &oauth2.Token{}); err != nil {
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
		authURL:            authURL,
		ch:                 ch,
	}
	sm.lock.Unlock()
	return nil
}

func (sm *stateCache) activePendingAuthURL(ctx context.Context, userID, mcpID, mcpURL, oauthAuthRequestID string) (string, error) {
	token, err := sm.gatewayClient.GetActivePendingMCPOAuthToken(ctx, userID, mcpID, mcpURL, oauthAuthRequestID)
	if err != nil {
		return "", err
	}

	return token.PendingAuthURL, nil
}

func (sm *stateCache) setPendingAuthURL(ctx context.Context, state, authURL string) error {
	sm.lock.Lock()
	if cached, ok := sm.cache[state]; ok {
		cached.authURL = authURL
		sm.cache[state] = cached
	}
	sm.lock.Unlock()

	return sm.gatewayClient.SetPendingAuthURLByState(ctx, state, authURL)
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
		if !token.PendingExpiresAt.IsZero() && !token.PendingExpiresAt.After(time.Now()) {
			return "", "", fmt.Errorf("failed to get oauth state: %w", gorm.ErrRecordNotFound)
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

	return oauthAuthRequestID, mcpID, sm.gatewayClient.ReplaceMCPOAuthToken(ctx, userID, mcpID, mcpURL, "", "", "", "", time.Time{}, conf, token)
}

func pendingAuthURL(conf *oauth2.Config, state, verifier, mcpURL string) (string, error) {
	if conf == nil {
		return "", errors.New("oauth config is required")
	}

	authEndpoint, err := url.Parse(conf.Endpoint.AuthURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse authorization endpoint: %w", err)
	}

	authCodeURLOpts := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier)}
	if mcpURL != "" && authEndpoint.Host != "login.microsoftonline.com" {
		authCodeURLOpts = append(authCodeURLOpts, oauth2.SetAuthURLParam("resource", mcpURL))
	}

	return conf.AuthCodeURL(state, authCodeURLOpts...), nil
}
