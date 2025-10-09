package oauth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
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
func (f *MCPOAuthHandlerFactory) CheckForMCPAuth(ctx context.Context, mcpServer v1.MCPServer, mcpServerConfig mcp.ServerConfig, userID, mcpID, oauthAppAuthRequestID string) (string, error) {
	// Handle composite servers
	if mcpServerConfig.Runtime == types.RuntimeComposite {
		return f.checkForCompositeMCPAuth(ctx, mcpServer, userID, mcpID, oauthAppAuthRequestID)
	}

	if mcpServerConfig.Runtime != types.RuntimeRemote {
		// OAuth is only support for remote MCP servers.
		return "", nil
	}

	oauthHandler := f.newMCPOAuthHandler(userID, mcpID, oauthAppAuthRequestID)
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)
		_, err := f.mcpSessionManager.ClientForMCPServerWithOptions(ctx, "Obot OAuth Check", mcpServer, mcpServerConfig, nmcp.ClientOption{
			ClientName:       "Obot MCP OAuth",
			OAuthRedirectURL: fmt.Sprintf("%s/oauth/mcp/callback", f.baseURL),
			OAuthClientName:  "Obot MCP Gateway",
			CallbackHandler:  oauthHandler,
			ClientCredLookup: oauthHandler,
			TokenStorage:     f.tokenStore.ForUserAndMCP(oauthHandler.userID, oauthHandler.mcpID),
		})
		if err != nil {
			errChan <- fmt.Errorf("failed to get client for server %s: %v", mcpServer.Name, err)
		} else {
			errChan <- nil
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

func (f *MCPOAuthHandlerFactory) checkForCompositeMCPAuth(ctx context.Context, mcpServer v1.MCPServer, userID, mcpID, oauthAppAuthRequestID string) (string, error) {
	// Query child servers
	var childServerList v1.MCPServerList
	if err := f.client.List(ctx, &childServerList, &kclient.ListOptions{
		Namespace: mcpServer.Namespace,
	}); err != nil {
		return "", fmt.Errorf("failed to list child servers: %w", err)
	}

	// Filter children by composite-parent label
	var children []v1.MCPServer
	for _, server := range childServerList.Items {
		if server.Labels != nil && server.Labels["composite-parent"] == mcpServer.Name {
			children = append(children, server)
		}
	}

	// Check if any child needs OAuth
	needsOAuth := false
	for _, childServer := range children {
		// Only remote servers can have OAuth
		if childServer.Spec.Manifest.Runtime != types.RuntimeRemote {
			continue
		}

		// Check if this child already has a valid token
		tokenStore := f.tokenStore.ForUserAndMCP(userID, childServer.Name)

		// Try to check for OAuth by creating a handler and checking if it would request auth
		childHandler := f.newMCPOAuthHandler(userID, childServer.Name, oauthAppAuthRequestID)
		childErrChan := make(chan error, 1)
		childConfig := mcp.ServerConfig{
			Runtime: childServer.Spec.Manifest.Runtime,
			URL:     childServer.Spec.Manifest.RemoteConfig.URL,
		}

		go func(server v1.MCPServer, config mcp.ServerConfig) {
			defer close(childErrChan)
			_, err := f.mcpSessionManager.ClientForMCPServerWithOptions(ctx, "Obot OAuth Check", server, config, nmcp.ClientOption{
				ClientName:       "Obot MCP OAuth",
				OAuthRedirectURL: fmt.Sprintf("%s/oauth/mcp/callback", f.baseURL),
				OAuthClientName:  "Obot MCP Gateway",
				CallbackHandler:  childHandler,
				ClientCredLookup: childHandler,
				TokenStorage:     tokenStore,
			})
			childErrChan <- err
		}(childServer, childConfig)

		select {
		case <-childErrChan:
			// If no error, child doesn't need OAuth
			continue
		case <-ctx.Done():
			return "", fmt.Errorf("failed to check child server OAuth: %w", ctx.Err())
		case <-childHandler.URLChan():
			// Child needs OAuth
			needsOAuth = true
			break
		}

		if needsOAuth {
			break
		}
	}

	if !needsOAuth {
		// All children are authenticated
		return "", nil
	}

	// Return URL to composite OAuth page
	return fmt.Sprintf("%s/mcp/composite/%s/%s", f.baseURL, oauthAppAuthRequestID, mcpID), nil
}

type mcpOAuthHandler struct {
	client             kclient.Client
	gptscript          *gptscript.GPTScript
	stateCache         *stateCache
	mcpID              string
	userID             string
	oauthAuthRequestID string
	urlChan            chan string
}

func (f *MCPOAuthHandlerFactory) newMCPOAuthHandler(userID, mcpID, oauthAuthRequestID string) *mcpOAuthHandler {
	return &mcpOAuthHandler{
		client:             f.client,
		gptscript:          f.gptscript,
		stateCache:         f.stateCache,
		userID:             userID,
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
	return state, ch, m.stateCache.store(ctx, m.userID, m.mcpID, m.oauthAuthRequestID, state, verifier, conf, ch)
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
