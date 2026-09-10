package oauth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"golang.org/x/oauth2"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	obotOAuthClientName = "Obot MCP OAuth"
)

type MCPOAuthHandlerFactory struct {
	baseURL                   string
	mcpSessionManager         *mcp.SessionManager
	client                    kclient.Client
	stateMgr                  *stateManager
	tokenStore                mcp.GlobalTokenStore
	secretBindingAllowedLabel string
	cimdDocumentURL           string
	resolveOAuthClient        func(context.Context, kclient.Client, string) (v1.OAuthClient, error)
}

type mcpOAuthHandler struct {
	gatewayClient      *client.Client
	stateMgr           *stateManager
	mcpID              string
	mcpURL             string
	userID             string
	oauthAuthRequestID string
	urlChan            chan string

	// catalogEntryName is the name of the catalog entry to fetch static OAuth credentials for.
	catalogEntryName  string
	credentialContext string
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
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeVMCP {
		componentServers, err := f.componentServersForAuth(req, mcpServer, mcpServerConfig)
		if err != nil {
			return "", err
		}

		// Gate the number of parallel calls.
		limit := make(chan struct{}, 5)
		defer close(limit)
		// Prime the channel here so we can drain at the end
		for range cap(limit) {
			limit <- struct{}{}
		}

		var (
			needsOAuth bool
			checkErr   error
			lock       sync.RWMutex
		)
		for _, componentServer := range componentServers {
			if componentServer.Spec.Manifest.Runtime != types.RuntimeRemote {
				continue
			}

			lock.RLock()
			if needsOAuth || checkErr != nil {
				lock.RUnlock()
				break
			}
			lock.RUnlock()

			<-limit

			go func() {
				defer func() {
					limit <- struct{}{}
				}()

				_, componentConfig, err := f.mcpSessionManager.ServerForAction(req.Context(), componentServer.Name, req.User.GetUID())
				if err != nil {
					return
				}

				u, err := f.CheckForMCPAuth(req, componentServer, componentConfig, userID, componentServer.Name, oauthAppAuthRequestID)
				if err != nil {
					lock.Lock()
					defer lock.Unlock()

					if req.Context().Err() != nil {
						checkErr = fmt.Errorf("failed to check component server OAuth: %w", req.Context().Err())
					} else {
						checkErr = fmt.Errorf("failed to check component server %s OAuth: %w", componentServer.Name, err)
					}
					return
				}

				if u != "" {
					lock.Lock()
					defer lock.Unlock()
					needsOAuth = true
				}
			}()
		}

		// Wait for everything to finish
		for range cap(limit) {
			<-limit
		}

		if checkErr != nil {
			return "", checkErr
		}

		if needsOAuth {
			// At least one component requires OAuth.
			slog.Info("Aggregate MCP server requires component OAuth authentication", "mcpID", mcpID)
			return compositeConsentURL(f.baseURL, mcpID, mcpServer.Spec.VMCPID, oauthAppAuthRequestID), nil
		}

		// No component requires OAuth
		slog.Info("Aggregate MCP server passed OAuth check with no pending component authentication", "mcpID", mcpID)
		return "", nil
	} else if mcpServerConfig.Runtime != types.RuntimeRemote {
		// Not a remote or vMCP server, no OAuth required
		return "", nil
	}

	if mcpServerConfig.TunnelName == "" {
		if err := mcp.ValidateRemoteMCPURL(req.Context(), mcpServerConfig.URL, f.mcpSessionManager.RemoteMCPURLValidationConfig()); err != nil {
			return "", err
		}
	}

	// Remote server, check for OAuth directly
	oauthHandler := f.newMCPOAuthHandler(req.GatewayClient, userID, mcpID, mcpServerConfig.URL, oauthAppAuthRequestID, mcpServerConfig.MCPCatalogEntryName)
	if mcpServer.Spec.VMCPID != "" || mcpServer.Spec.VMCPInstanceID != "" {
		credentialContext, _, err := vmcpconfig.ServerOAuthCredentialReference(req.Context(), f.client, mcpServer)
		if err != nil {
			return "", fmt.Errorf("resolve VMCP OAuth credential: %w", err)
		}
		oauthHandler.credentialContext = credentialContext
	}
	staticOAuthPending, err := f.staticOAuthPending(req.Context(), mcpServer, oauthHandler)
	if err != nil {
		return "", err
	}
	oauthClientName, err := f.downstreamOAuthClientName(req, oauthAppAuthRequestID)
	if err != nil {
		return "", err
	}
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)

		_, err := f.mcpSessionManager.ClientForMCPServerForOAuthCheck(req.Context(), mcpServerConfig, mcp.ClientOption{
			OAuthClientName:               oauthClientName,
			OAuthClientIDMetadataDocument: f.cimdDocumentURL,
			ClientName:                    obotOAuthClientName,
			TokenStorage:                  f.tokenStore.ForUserAndMCP(userID, mcpID, mcpServerConfig.URL),
			CallbackHandler:               oauthHandler,
			ClientLookup:                  oauthHandler,
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
		slog.Info("Remote MCP server requires OAuth authentication", "mcpID", mcpID)
		return u, nil
	}
}

func compositeConsentURL(baseURL, connectID, vmcpID, authRequestID string) string {
	query := url.Values{}
	if authRequestID != "" {
		query.Set("oauth_auth_request", authRequestID)
	}
	// The UI loads metadata by canonical ID, but authenticates the original
	// connection so migrated users with multiple instances keep their selection.
	if vmcpID != "" && vmcpID != connectID {
		query.Set("vmcp_id", vmcpID)
	}
	result := fmt.Sprintf("%s/auth/mcp/composite/%s", baseURL, connectID)
	if len(query) > 0 {
		result += "?" + query.Encode()
	}
	return result
}

func (f *MCPOAuthHandlerFactory) componentServersForAuth(req api.Context, mcpServer v1.MCPServer, mcpServerConfig mcp.ServerConfig) ([]v1.MCPServer, error) {
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeVMCP {
		componentServers := make([]v1.MCPServer, 0, len(mcpServerConfig.Components))
		for _, component := range mcpServerConfig.Components {
			var componentServer v1.MCPServer
			key := kclient.ObjectKey{
				Namespace: mcpServer.Namespace,
				Name:      component.Name,
			}
			var err error
			if req.Storage != nil {
				err = req.Storage.Get(req.Context(), key, &componentServer)
			} else {
				err = f.client.Get(req.Context(), key, &componentServer)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to get vMCP component server %q: %w", component.Name, err)
			}
			componentServers = append(componentServers, componentServer)
		}
		return componentServers, nil
	}

	return nil, nil
}

func (f *MCPOAuthHandlerFactory) downstreamOAuthClientName(req api.Context, oauthAuthRequestID string) (string, error) {
	if oauthAuthRequestID == "" {
		return "", nil
	}
	if f.resolveOAuthClient == nil {
		return "", fmt.Errorf("failed to resolve originating OAuth client: client resolver is not configured")
	}

	var authRequest v1.OAuthAuthRequest
	if err := req.Get(&authRequest, oauthAuthRequestID); err != nil {
		return "", fmt.Errorf("failed to get originating OAuth request %s: %w", oauthAuthRequestID, err)
	}
	if authRequest.Spec.ClientID == "" {
		return "", fmt.Errorf("originating OAuth request %s has no client ID", oauthAuthRequestID)
	}

	clientID := oauthAuthRequestClientID(authRequest)
	oauthClient, err := f.resolveOAuthClient(req.Context(), req.Storage, clientID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve originating OAuth client %s: %w", clientID, err)
	}

	return oauthClient.Spec.Manifest.ClientName, nil
}

func (f *MCPOAuthHandlerFactory) staticOAuthPending(ctx context.Context, mcpServer v1.MCPServer, oauthHandler *mcpOAuthHandler) (bool, error) {
	if !mcp.RequiresStaticOAuth(mcpServer) {
		return false, nil
	}

	conf, token, err := f.tokenStore.ForUserAndMCP(oauthHandler.userID, oauthHandler.mcpID, oauthHandler.mcpURL).GetTokenConfig(ctx)
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

	clientID, clientSecret, err := oauthHandler.Lookup(ctx)
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
			AuthStyle: oauth2.AuthStyleAutoDetect,
		},
	}
	if registration.Scope != "" {
		conf.Scopes = strings.Fields(registration.Scope)
	}

	resourceURL := mcp.ResolveOAuthResourceURL(authorizationServer.AuthorizationEndpoint, metadata.ResourceURL, serverConfig.URL)
	authURL, _, _, err := mcp.GetOAuthAuthorizationURL(ctx, oauthHandler, conf, authorizationServer.AuthorizationEndpoint, resourceURL)
	if err != nil {
		return "", err
	}
	slog.Info("Remote MCP server requires configured static OAuth authentication", "mcpID", oauthHandler.mcpID)
	return authURL, nil
}

func staticOAuthMetadata(metadata mcp.OAuthMetadata, redirectURL string) (mcp.AuthorizationServerMetadata, mcp.ClientRegistrationMetadata, error) {
	var authorizationServer mcp.AuthorizationServerMetadata
	if len(metadata.AuthorizationServerMetadata) > 0 {
		if err := json.Unmarshal(metadata.AuthorizationServerMetadata, &authorizationServer); err != nil {
			return authorizationServer, mcp.ClientRegistrationMetadata{}, fmt.Errorf("failed to parse authorization server metadata: %w", err)
		}
	}
	if authorizationServer.AuthorizationEndpoint == "" || authorizationServer.TokenEndpoint == "" {
		return authorizationServer, mcp.ClientRegistrationMetadata{}, fmt.Errorf("static OAuth is required but authorization server metadata was not found")
	}

	var registration mcp.ClientRegistrationMetadata
	if len(metadata.ClientRegistration) > 0 {
		if err := json.Unmarshal(metadata.ClientRegistration, &registration); err != nil {
			return authorizationServer, registration, fmt.Errorf("failed to parse OAuth client registration metadata: %w", err)
		}
	}

	return authorizationServer, mcp.AuthServerMetadataToClientRegistration(authorizationServer,
		"Obot MCP Gateway", redirectURL, registration.Scope), nil
}

func (f *MCPOAuthHandlerFactory) newMCPOAuthHandler(gatewayClient *client.Client, userID, mcpID, mcpURL, oauthAuthRequestID, catalogEntryName string) *mcpOAuthHandler {
	return &mcpOAuthHandler{
		gatewayClient:      gatewayClient,
		stateMgr:           f.stateMgr,
		userID:             userID,
		mcpID:              mcpID,
		mcpURL:             mcpURL,
		oauthAuthRequestID: oauthAuthRequestID,
		catalogEntryName:   catalogEntryName,
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

func (m *mcpOAuthHandler) NewState(ctx context.Context, conf *oauth2.Config, resourceURL, verifier string) (string, <-chan mcp.CallbackPayload, error) {
	state := strings.ToLower(rand.Text())

	// The channel is required by the nanobot CallbackHandler interface but is not used
	// in the Obot flow. The auth URL is handled via HandleAuthURL/URLChan, and the
	// callback arrives via a separate HTTP endpoint (oauthCallback) which looks up
	// the pending state from the DB directly.
	ch := make(chan mcp.CallbackPayload)
	return state, ch, m.stateMgr.store(ctx, m.userID, m.mcpID, m.mcpURL, m.oauthAuthRequestID, state, verifier, resourceURL, conf)
}

func (m *mcpOAuthHandler) Lookup(ctx context.Context) (string, string, error) {
	// If the server was created from a catalog entry, look up OAuth credentials by catalog entry name
	credentialContext := m.credentialContext
	if credentialContext == "" && m.catalogEntryName != "" {
		credentialContext = system.MCPOAuthCredentialName(m.catalogEntryName)
	}
	if credentialContext != "" {
		cred, err := m.gatewayClient.RevealCredential(ctx, []string{credentialContext}, system.StaticOAuthCredentialName)
		if err == nil {
			clientID := cred.Secrets["CLIENT_ID"]
			clientSecret := cred.Secrets["CLIENT_SECRET"]
			if clientID != "" {
				return clientID, clientSecret, nil
			}
		}
	}

	return "", "", fmt.Errorf("no credentials found for MCP server %s", m.mcpID)
}
