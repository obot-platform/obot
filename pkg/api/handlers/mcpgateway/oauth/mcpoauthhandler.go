package oauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"

	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/oauth2"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type MCPOAuthHandlerFactory struct {
	baseURL                   string
	mcpSessionManager         *mcp.SessionManager
	client                    kclient.Client
	stateMgr                  *stateManager
	tokenStore                mcp.GlobalTokenStore
	secretBindingAllowedLabel string
	cimdDocumentURL           string
}

func NewMCPOAuthHandlerFactory(baseURL string, sessionManager *mcp.SessionManager, client kclient.Client, gatewayClient *client.Client, globalTokenStore mcp.GlobalTokenStore, secretBindingAllowedLabel string, forceDynamicClient bool) *MCPOAuthHandlerFactory {
	f := &MCPOAuthHandlerFactory{
		baseURL:                   baseURL,
		mcpSessionManager:         sessionManager,
		client:                    client,
		stateMgr:                  newStateManager(gatewayClient),
		tokenStore:                globalTokenStore,
		secretBindingAllowedLabel: secretBindingAllowedLabel,
	}

	if !forceDynamicClient && strings.HasPrefix(baseURL, "https://") {
		f.cimdDocumentURL = system.OAuthClientIDMetadataURL(baseURL)
	}

	return f
}

func (f *MCPOAuthHandlerFactory) CheckForMCPAuth(req api.Context, mcpServer v1.MCPServer, mcpServerConfig mcp.ServerConfig, userID, mcpID, oauthAppAuthRequestID string) (string, error) {
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		var componentServers v1.MCPServerList
		if err := f.client.List(req.Context(), &componentServers,
			kclient.InNamespace(mcpServer.Namespace),
			kclient.MatchingFields{"spec.compositeName": mcpServer.Name},
		); err != nil {
			return "", fmt.Errorf("failed to list component servers")
		}

		// Precompute disabled component set for quick lookup (by catalog entry ID only)
		var compositeConfig types.CompositeRuntimeConfig
		if mcpServer.Spec.Manifest.CompositeConfig != nil {
			compositeConfig = *mcpServer.Spec.Manifest.CompositeConfig
		}

		disabled := make(map[string]bool, len(compositeConfig.ComponentServers))
		for _, comp := range compositeConfig.ComponentServers {
			disabled[comp.CatalogEntryID] = comp.Disabled
		}

		for _, componentServer := range componentServers.Items {
			// Skip disabled components defined in the composite server config using O(1) lookups
			if disabled[componentServer.Spec.MCPServerCatalogEntryName] ||
				componentServer.Spec.Manifest.Runtime != types.RuntimeRemote {
				continue
			}

			_, componentConfig, err := f.mcpSessionManager.ServerForAction(req.Context(), componentServer.Name, req.User.GetUID())
			if err != nil {
				continue
			}

			u, err := f.CheckForMCPAuth(req, componentServer, componentConfig, userID, componentServer.Name, oauthAppAuthRequestID)
			if err != nil {
				if req.Context().Err() != nil {
					return "", fmt.Errorf("failed to check component server OAuth: %w", req.Context().Err())
				}
				return "", fmt.Errorf("failed to check component server %s OAuth: %w", componentServer.Name, err)
			}

			if u != "" {
				// At least one component requires OAuth
				log.Infof("Composite MCP server requires component OAuth authentication: compositeMCPID=%s componentMCPID=%s", mcpID, componentServer.Name)
				if oauthAppAuthRequestID != "" {
					return fmt.Sprintf("%s/auth/mcp/composite/%s?oauth_auth_request=%s", f.baseURL, mcpID, oauthAppAuthRequestID), nil
				}

				return fmt.Sprintf("%s/auth/mcp/composite/%s", f.baseURL, mcpID), nil
			}
		}

		// No component requires OAuth
		log.Infof("Composite MCP server passed OAuth check with no pending component authentication: compositeMCPID=%s", mcpID)
		return "", nil
	} else if mcpServerConfig.Runtime != types.RuntimeRemote {
		// Not a remote or composite server, no OAuth required
		return "", nil
	}

	if mcpServerConfig.TunnelName == "" {
		if err := mcp.ValidateRemoteMCPURL(req.Context(), mcpServerConfig.URL, f.mcpSessionManager.RemoteMCPURLValidationConfig()); err != nil {
			return "", err
		}
	}

	// Remote server, check for OAuth directly
	oauthHandler := f.newMCPOAuthHandler(req.GatewayClient, userID, mcpID, mcpServerConfig.URL, oauthAppAuthRequestID)
	staticOAuthPending, err := f.staticOAuthPending(req.Context(), mcpServer, oauthHandler)
	if err != nil {
		return "", err
	}
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)

		blockingConfig := f.mcpSessionManager.RemoteMCPURLValidationConfig()
		httpClientOptions := nmcp.HTTPClientOptions{
			OAuthRedirectURL:              system.MCPOAuthCallbackURL(f.baseURL),
			OAuthClientName:               "Obot MCP Gateway",
			OAuthClientIDMetadataDocument: f.cimdDocumentURL,
			CallbackHandler:               oauthHandler,
			ClientCredLookup:              oauthHandler,
			TokenStorage:                  f.tokenStore.ForUserAndMCP(oauthHandler.userID, oauthHandler.mcpID),
			BlockLoopback:                 !blockingConfig.AllowLocalhostMCP,
			BlockPrivateIP:                !blockingConfig.AllowPrivateIPMCP,
			BlockLinkLocal:                !blockingConfig.AllowLinkLocalMCP,
		}
		_, err := f.mcpSessionManager.ClientForMCPServerForOAuthCheck(req.Context(), mcpServerConfig, nmcp.ClientOption{
			ClientName:        "Obot MCP OAuth",
			HTTPClientOptions: httpClientOptions,
		})
		if err != nil {
			errChan <- fmt.Errorf("failed to get client for server %s: %v", mcpServer.Name, err)
		} else {
			f.mcpSessionManager.CloseClient(mcpServerConfig, "Obot OAuth Check")
			errChan <- nil
		}
	}()

	select {
	case err := <-errChan:
		if err != nil || !staticOAuthPending {
			return "", err
		}
		return f.staticOAuthURL(req.Context(), mcpServerConfig, oauthHandler)
	case <-req.Context().Done():
		return "", fmt.Errorf("failed to check for MCP server OAuth: %w", req.Context().Err())
	case u := <-oauthHandler.URLChan():
		log.Infof("Remote MCP server requires OAuth authentication: mcpID=%s", mcpID)
		return u, nil
	}
}

func (f *MCPOAuthHandlerFactory) staticOAuthPending(ctx context.Context, mcpServer v1.MCPServer, oauthHandler *mcpOAuthHandler) (bool, error) {
	if !mcp.RequiresStaticOAuth(mcpServer) {
		return false, nil
	}

	conf, token, err := f.tokenStore.ForUserAndMCP(oauthHandler.userID, oauthHandler.mcpID).GetTokenConfig(ctx, oauthHandler.mcpURL)
	if err != nil {
		return false, fmt.Errorf("failed to check stored OAuth token for MCP server %s: %w", mcpServer.Name, err)
	}
	return conf == nil || token == nil || token.AccessToken == "", nil
}

func (f *MCPOAuthHandlerFactory) staticOAuthURL(ctx context.Context, serverConfig mcp.ServerConfig, oauthHandler *mcpOAuthHandler) (string, error) {
	metadata, err := f.mcpSessionManager.GetOAuthMetadata(ctx, serverConfig,
		"Obot MCP Gateway", system.MCPOAuthCallbackURL(f.baseURL), true)
	if err != nil {
		return "", fmt.Errorf("failed to discover OAuth metadata for static OAuth server: %w", err)
	}

	callbackURL := system.MCPOAuthCallbackURL(f.baseURL)
	authorizationServer, registration, err := staticOAuthMetadata(metadata, callbackURL)
	if err != nil {
		return "", err
	}

	clientID, clientSecret, err := oauthHandler.Lookup(ctx, metadata.AuthorizationServerMetadataURL)
	if err != nil {
		return "", err
	}
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  callbackURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:   authorizationServer.AuthorizationEndpoint,
			TokenURL:  authorizationServer.TokenEndpoint,
			AuthStyle: staticOAuthAuthStyle(registration.TokenEndpointAuthMethod),
		},
	}
	if registration.Scope != "" {
		conf.Scopes = strings.Fields(registration.Scope)
	}

	authURL, _, _, err := nmcp.GetOAuthAuthorizationURL(ctx, oauthHandler, conf, authorizationServer.AuthorizationEndpoint, serverConfig.URL)
	if err != nil {
		return "", err
	}
	log.Infof("Remote MCP server requires configured static OAuth authentication: mcpID=%s", oauthHandler.mcpID)
	return authURL, nil
}

func staticOAuthMetadata(metadata nmcp.OAuthMetadata, redirectURL string) (nmcp.AuthorizationServerMetadata, nmcp.ClientRegistrationMetadata, error) {
	var authorizationServer nmcp.AuthorizationServerMetadata
	if len(metadata.AuthorizationServerMetadata) > 0 {
		if err := json.Unmarshal(metadata.AuthorizationServerMetadata, &authorizationServer); err != nil {
			return authorizationServer, nmcp.ClientRegistrationMetadata{}, fmt.Errorf("failed to parse authorization server metadata: %w", err)
		}
	}
	if authorizationServer.AuthorizationEndpoint == "" || authorizationServer.TokenEndpoint == "" {
		return authorizationServer, nmcp.ClientRegistrationMetadata{}, fmt.Errorf("static OAuth is required but authorization server metadata was not found")
	}

	var registration nmcp.ClientRegistrationMetadata
	if len(metadata.ClientRegistration) > 0 {
		if err := json.Unmarshal(metadata.ClientRegistration, &registration); err != nil {
			return authorizationServer, registration, fmt.Errorf("failed to parse OAuth client registration metadata: %w", err)
		}
	}

	return authorizationServer, nmcp.AuthServerMetadataToClientRegistration(authorizationServer,
		"Obot MCP Gateway", redirectURL, registration.Scope), nil
}

func staticOAuthAuthStyle(method string) oauth2.AuthStyle {
	switch method {
	case "client_secret_basic":
		return oauth2.AuthStyleInHeader
	case "client_secret_post":
		return oauth2.AuthStyleInParams
	default:
		return oauth2.AuthStyleAutoDetect
	}
}

type mcpOAuthHandler struct {
	client             kclient.Client
	gatewayClient      *client.Client
	stateMgr           *stateManager
	mcpID              string
	mcpURL             string
	userID             string
	oauthAuthRequestID string
	urlChan            chan string
}

func (f *MCPOAuthHandlerFactory) newMCPOAuthHandler(gatewayClient *client.Client, userID, mcpID, mcpURL, oauthAuthRequestID string) *mcpOAuthHandler {
	return &mcpOAuthHandler{
		client:             f.client,
		gatewayClient:      gatewayClient,
		stateMgr:           f.stateMgr,
		userID:             userID,
		mcpID:              mcpID,
		mcpURL:             mcpURL,
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

	// The channel is required by the nanobot CallbackHandler interface but is not used
	// in the Obot flow. The auth URL is handled via HandleAuthURL/URLChan, and the
	// callback arrives via a separate HTTP endpoint (oauthCallback) which looks up
	// the pending state from the DB directly.
	ch := make(chan nmcp.CallbackPayload)
	return state, ch, m.stateMgr.store(ctx, m.userID, m.mcpID, m.mcpURL, m.oauthAuthRequestID, state, verifier, conf)
}

func (m *mcpOAuthHandler) Lookup(ctx context.Context, _ string) (string, string, error) {
	if m.mcpID != "" {
		var server v1.MCPServer
		if err := m.client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: m.mcpID}, &server); err == nil {
			// If the server was created from a catalog entry, look up OAuth credentials by catalog entry name
			if server.Spec.MCPServerCatalogEntryName != "" {
				credName := system.MCPOAuthCredentialName(server.Spec.MCPServerCatalogEntryName)
				cred, err := m.gatewayClient.RevealCredential(ctx, []string{credName}, "oauth")
				if err == nil {
					clientID := cred.Secrets["CLIENT_ID"]
					clientSecret := cred.Secrets["CLIENT_SECRET"]
					if clientID != "" && clientSecret != "" {
						return clientID, clientSecret, nil
					}
				}
			}
		}
	}

	return "", "", fmt.Errorf("no credentials found for MCP server %s", m.mcpID)
}
