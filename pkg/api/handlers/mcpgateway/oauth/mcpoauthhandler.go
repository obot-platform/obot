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
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
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
func (f *MCPOAuthHandlerFactory) CheckForMCPAuth(req api.Context, mcpServer v1.MCPServer, mcpServerConfig mcp.ServerConfig, userID, mcpID, oauthAppAuthRequestID string) (string, error) {
	if mcpServerConfig.Runtime == types.RuntimeComposite {
		var componentServers v1.MCPServerList
		if err := f.client.List(req.Context(), &componentServers,
			kclient.InNamespace(mcpServer.Namespace),
			kclient.MatchingFields{"spec.compositeName": mcpServer.Name},
		); err != nil {
			return "", fmt.Errorf("failed to list component servers")
		}

		// Precompute disabled component set for quick lookup (by catalog entry ID only)
		var disabled map[string]struct{}
		if mcpServer.Spec.Manifest.CompositeConfig != nil {
			disabled = make(map[string]struct{}, len(mcpServer.Spec.Manifest.CompositeConfig.ComponentServers))
			for _, comp := range mcpServer.Spec.Manifest.CompositeConfig.ComponentServers {
				if !comp.Enabled && comp.CatalogEntryID != "" {
					disabled[comp.CatalogEntryID] = struct{}{}
				}
			}
		}

		for _, componentServer := range componentServers.Items {
			// Skip disabled components defined in the composite server config using O(1) lookups
			if _, ok := disabled[componentServer.Spec.MCPServerCatalogEntryName]; ok {
				continue
			}
			if componentServer.Spec.Manifest.Runtime != types.RuntimeRemote {
				// Only remote component servers can require OAuth
				continue
			}

			componentConfig, err := handlers.ServerConfigForAction(req, componentServer)
			if err != nil {
				continue
			}

			u, err := f.CheckForMCPAuth(req, componentServer, componentConfig, userID, componentServer.Name, oauthAppAuthRequestID)
			if err != nil {
				if req.Context().Err() != nil {
					return "", fmt.Errorf("failed to check component server OAuth: %w", req.Context().Err())
				}
			}

			if u != "" {
				// At least one component requires OAuth
				log.Errorf("OAuth URL for component server %s: %s", componentServer.Name, u)
				if oauthAppAuthRequestID != "" {
					return fmt.Sprintf("%s/auth/mcp/composite/%s?oauth_auth_request=%s", f.baseURL, mcpID, oauthAppAuthRequestID), nil
				}

				return fmt.Sprintf("%s/auth/mcp/composite/%s", f.baseURL, mcpID), nil
			}
		}

		// No component requires OAuth
		return "", nil
	} else if mcpServerConfig.Runtime != types.RuntimeRemote {
		// Not a remote or composite server, no OAuth required
		return "", nil
	}

	// Remote server, check for OAuth directly
	oauthHandler := f.newMCPOAuthHandler(userID, mcpID, oauthAppAuthRequestID)
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)
		_, err := f.mcpSessionManager.ClientForMCPServerWithOptions(req.Context(), userID, "Obot OAuth Check", mcpServer, mcpServerConfig, nmcp.ClientOption{
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
	case <-req.Context().Done():
		return "", fmt.Errorf("failed to check for MCP server OAuth: %w", req.Context().Err())
	case u := <-oauthHandler.URLChan():
		return u, nil
	}
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
