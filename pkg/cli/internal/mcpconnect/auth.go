package mcpconnect

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

type serializedOAuthHandler struct {
	auth.OAuthHandler
	store  *tokenStore
	client *http.Client
}

func newOAuthHandler(_ context.Context, dir, connectURL, redirectURL string, callback *callbackHandler, client *http.Client) (auth.OAuthHandler, error) {
	store := newTokenStore(dir, connectURL)
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		RedirectURL: redirectURL,
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName: "Obot CLI", RedirectURIs: []string{redirectURL}, TokenEndpointAuthMethod: "none",
			GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"},
		}},
		AuthorizationCodeFetcher: callback.fetch,
		RequestRefreshToken:      true,
		Client:                   client,
		NewTokenSource: func(_ context.Context, conf *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			// Authorize already holds the cross-process lock. The SDK uses this
			// source internally for scope tracking; external requests use our
			// shared source below, avoiding recursive locking.
			if err := store.save(conf, token); err != nil {
				return nil, err
			}
			return oauth2.StaticTokenSource(token), nil
		},
	})
	if err != nil {
		return nil, err
	}
	return &serializedOAuthHandler{OAuthHandler: handler, store: store, client: client}, nil
}

func (h *serializedOAuthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	return h.store.load(ctx, h.client)
}

func (h *serializedOAuthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	defer resp.Body.Close()
	unlock, err := h.store.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	tok, err := h.store.tokenLocked(context.WithValue(ctx, oauth2.HTTPClient, h.client))
	if err != nil {
		return err
	}
	if tok.AccessToken != "" {
		probe := &http.Request{Header: make(http.Header)}
		tok.SetAuthHeader(probe)
		if probe.Header.Get("Authorization") != req.Header.Get("Authorization") {
			return nil
		}
	}
	return h.OAuthHandler.Authorize(ctx, req, resp)
}
