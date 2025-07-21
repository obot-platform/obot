package oauth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type MCPOAuthHandlerFactory struct {
	baseURL           string
	mcpSessionManager *mcp.SessionManager
	client            kclient.Client
	gptscript         *gptscript.GPTScript
	stateCache        *stateCache
	tokenStore        mcp.GlobalTokenStore
}

func NewMCPOAuthHandlerFactory(baseURL string, sessionManager *mcp.SessionManager, client kclient.Client, gptClient *gptscript.GPTScript, gatewayClient *client.Client, globalTokenStore mcp.GlobalTokenStore) *MCPOAuthHandlerFactory {
	return &MCPOAuthHandlerFactory{
		baseURL:           baseURL,
		mcpSessionManager: sessionManager,
		client:            client,
		gptscript:         gptClient,
		stateCache:        newStateCache(gatewayClient),
		tokenStore:        globalTokenStore,
	}
}
func (f *MCPOAuthHandlerFactory) CheckForMCPAuth(ctx context.Context, mcpServer v1.MCPServer, mcpServerConfig mcp.ServerConfig, mcpID, oauthAppAuthRequestID string) (string, error) {
	// Give the server config a scope that makes sense.
	// Clients used in the proxy will set the scope that comes with the server config, but we need to ensure we get a different client here
	// because the client we use here needs the CallbackHandler and ClientCredLookup set.
	mcpServerConfig.Scope = mcpID

	oauthHandler := f.newMCPOAuthHandler(mcpID, oauthAppAuthRequestID)
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)
		_, err := f.mcpSessionManager.ClientForServerWithOptions(ctx, mcpServer, mcpServerConfig, nmcp.ClientOption{
			OAuthRedirectURL: fmt.Sprintf("%s/oauth/mcp/callback", f.baseURL),
			OAuthClientName:  "Obot MCP Gateway",
			CallbackHandler:  oauthHandler,
			ClientCredLookup: oauthHandler,
			TokenStorage:     f.tokenStore.ForMCPID(oauthHandler.mcpID),
		})
		if err != nil {
			errChan <- fmt.Errorf("failed to get client for server %s: %v", mcpServer.Name, err)
		} else {
			errChan <- nil
		}

		// We only need this client for checking for OAuth. Close it, now that we're done.
		if err = f.mcpSessionManager.ShutdownServer(context.Background(), mcpServerConfig); err != nil {
			log.Errorf("failed to shutdown server after authentication %s: %v", mcpServer.Name, err)
		}
	}()

	select {
	case err := <-errChan:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("failed to check for MCP server OAuth: %w", ctx.Err())
	case u := <-oauthHandler.URLChan():
		return u, nil
	}
}

type mcpOAuthHandler struct {
	client             kclient.Client
	gptscript          *gptscript.GPTScript
	stateCache         *stateCache
	mcpID              string
	oauthAuthRequestID string
	urlChan            chan string
}

func (f *MCPOAuthHandlerFactory) newMCPOAuthHandler(mcpID, oauthAuthRequestID string) *mcpOAuthHandler {
	return &mcpOAuthHandler{
		client:             f.client,
		gptscript:          f.gptscript,
		stateCache:         f.stateCache,
		mcpID:              mcpID,
		oauthAuthRequestID: oauthAuthRequestID,
		urlChan:            make(chan string, 1),
	}
}

func (m *mcpOAuthHandler) URLChan() <-chan string {
	return m.urlChan
}

func (m *mcpOAuthHandler) HandleAuthURL(ctx context.Context, _ string, authURL string) (bool, error) {
	select {
	case m.urlChan <- authURL:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	default:
		return false, nil
	}
}

func (m *mcpOAuthHandler) NewState(ctx context.Context, conf *oauth2.Config, verifier string) (string, <-chan nmcp.CallbackPayload, error) {
	state := strings.ToLower(rand.Text())

	ch := make(chan nmcp.CallbackPayload)
	return state, ch, m.stateCache.store(ctx, m.mcpID, m.oauthAuthRequestID, state, verifier, conf, ch)
}

func (m *mcpOAuthHandler) Lookup(ctx context.Context, authServerURL string) (string, string, error) {
	var oauthApps v1.OAuthAppList
	if err := m.client.List(ctx, &oauthApps, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.manifest.authorizationServerURL": authServerURL,
		}),
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return "", "", err
	}

	if len(oauthApps.Items) != 1 {
		return "", "", fmt.Errorf("expected exactly one oauth app for authorization server %s, found %d", authServerURL, len(oauthApps.Items))
	}

	app := oauthApps.Items[0]

	var clientSecret string
	cred, err := m.gptscript.RevealCredential(ctx, []string{app.Name}, app.Spec.Manifest.Alias)
	if err != nil {
		var errNotFound gptscript.ErrNotFound
		if errors.As(err, &errNotFound) {
			if app.Spec.Manifest.ClientSecret != "" {
				clientSecret = app.Spec.Manifest.ClientSecret
			}
		} else {
			return "", "", err
		}
	} else {
		clientSecret = cred.Env["CLIENT_SECRET"]
	}

	return app.Spec.Manifest.ClientID, clientSecret, nil
}
