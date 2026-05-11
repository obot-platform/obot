package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const oauthDebuggerPendingStateMarker = "oauth-debugger"

// RegisterOAuthDebuggerClient registers an OAuth client for an MCP server and saves it for later debugger steps.
func (m *MCPHandler) RegisterOAuthDebuggerClient(req api.Context) error {
	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}
	if err := m.validateOAuthDebuggerServer(req, server); err != nil {
		return err
	}

	metadata, authServer, registration, err := m.oauthDebuggerMetadata(server)
	if err != nil {
		return err
	}

	clientID, clientSecret, err := m.lookupStaticOAuthClient(req, server, metadata.AuthorizationServerURL)
	if err != nil && authServer.RegistrationEndpoint == "" {
		return err
	}

	var registered types.OAuthClient
	if clientID == "" || clientSecret == "" {
		if authServer.RegistrationEndpoint == "" {
			return types.NewErrBadRequest("OAuth metadata does not include a dynamic client registration endpoint, must configure static client ID and secret")
		}

		registered, err = registerOAuthDebuggerClient(req.Context(), authServer.RegistrationEndpoint, registration)
		if err != nil {
			return err
		}

		clientID = registered.ClientID
		clientSecret = registered.ClientSecret
		if registered.TokenEndpointAuthMethod != "" {
			registration.TokenEndpointAuthMethod = registered.TokenEndpointAuthMethod
		}
		if registered.Scope != "" {
			registration.Scope = registered.Scope
		}
	}

	if clientID == "" {
		return types.NewErrBadRequest("OAuth client registration did not return client_id")
	}

	state := strings.ToLower(rand.Text())

	conf := oauthDebuggerConfig(clientID, clientSecret, authServer.AuthorizationEndpoint, authServer.TokenEndpoint, registration.TokenEndpointAuthMethod, firstString(registration.RedirectURIs), registration.Scope)
	if err := req.GatewayClient.CreateMCPOAuthPendingState(
		req.Context(),
		req.User.GetUID(),
		server.Name,
		serverConfig.URL,
		oauthDebuggerPendingStateMarker,
		state,
		oauth2.GenerateVerifier(),
		conf,
	); err != nil {
		return err
	}

	return req.Write(map[string]any{
		"state":  state,
		"client": registered,
	})
}

// GetOAuthDebuggerAuthorizationURL creates fresh pending OAuth state and returns the remote authorization URL.
func (m *MCPHandler) GetOAuthDebuggerAuthorizationURL(req api.Context) error {
	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}
	if err := m.validateOAuthDebuggerServer(req, server); err != nil {
		return err
	}

	var input types.OAuthDebuggerAuthorizationURLRequest
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to read request body: %v", err)
	}
	if input.State == "" {
		return types.NewErrBadRequest("state is required")
	}

	storedClient, err := req.GatewayClient.GetMCPOAuthPendingState(req.Context(), input.State)
	if err != nil {
		return err
	}

	if storedClient.UserID != req.User.GetUID() || storedClient.MCPID != server.Name || storedClient.OAuthAuthRequestID != oauthDebuggerPendingStateMarker {
		return types.NewErrNotFound("OAuth debugger client not found")
	}

	conf := oauthDebuggerConfigFromPendingState(storedClient)
	authURL, err := nmcp.AuthCodeURL(conf, storedClient.AuthURL, serverConfig.URL, input.State, storedClient.Verifier)
	if err != nil {
		return err
	}

	return req.Write(types.OAuthDebuggerAuthorizationURL{OAuthURL: authURL, State: input.State})
}

// ExchangeOAuthDebuggerToken exchanges the debugger authorization code and stores the token like the quick MCP OAuth flow.
func (m *MCPHandler) ExchangeOAuthDebuggerToken(req api.Context) error {
	server, serverConfig, err := serverForAction(req)
	if err != nil {
		return err
	}
	if err := m.validateOAuthDebuggerServer(req, server); err != nil {
		return err
	}

	var input types.OAuthDebuggerTokenRequest
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to read request body: %v", err)
	}
	if input.Code == "" {
		return types.NewErrBadRequest("code is required")
	}
	if input.State == "" {
		return types.NewErrBadRequest("state is required")
	}

	pendingState, err := req.GatewayClient.GetMCPOAuthPendingState(req.Context(), input.State)
	if err != nil {
		return err
	}
	if pendingState.UserID != req.User.GetUID() || pendingState.MCPID != server.Name || pendingState.URL != serverConfig.URL || pendingState.OAuthAuthRequestID != oauthDebuggerPendingStateMarker {
		return types.NewErrNotFound("OAuth debugger authorization state not found")
	}

	conf := oauthDebuggerConfigFromPendingState(pendingState)

	token, err := conf.Exchange(req.Context(), input.Code, oauth2.VerifierOption(pendingState.Verifier))
	if err != nil {
		return fmt.Errorf("failed to exchange OAuth code: %w", err)
	}

	if err := req.GatewayClient.ReplaceMCPOAuthToken(req.Context(), req.User.GetUID(), server.Name, serverConfig.URL, "", conf, token); err != nil {
		return err
	}
	_ = req.GatewayClient.DeleteMCPOAuthPendingState(req.Context(), pendingState.HashedState)

	var expiresIn int
	if !token.Expiry.IsZero() {
		expiresIn = int(time.Until(token.Expiry).Seconds())
	}

	return req.Write(types.OAuthToken{
		AccessToken:  halfToken(token.AccessToken),
		RefreshToken: halfToken(token.RefreshToken),
		TokenType:    token.TokenType,
		ExpiresIn:    expiresIn,
	})
}

func halfToken(token string) string {
	if token == "" {
		return ""
	}
	return token[:len(token)/2] + "..."
}

func (m *MCPHandler) validateOAuthDebuggerServer(req api.Context, server v1.MCPServer) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}
	if server.Spec.Manifest.Runtime != types.RuntimeRemote {
		return types.NewErrBadRequest("OAuth debugger only supports remote MCP servers")
	}
	if server.Status.OAuthMetadata == nil {
		return types.NewErrBadRequest("OAuth metadata has not been discovered for this MCP server")
	}
	return nil
}

func (m *MCPHandler) oauthDebuggerMetadata(server v1.MCPServer) (*v1.OAuthMetadata, nmcp.AuthorizationServerMetadata, nmcp.ClientRegistrationMetadata, error) {
	metadata := server.Status.OAuthMetadata
	var authServer nmcp.AuthorizationServerMetadata
	if len(metadata.AuthorizationServerMetadata) > 0 {
		if err := json.Unmarshal(metadata.AuthorizationServerMetadata, &authServer); err != nil {
			return nil, authServer, nmcp.ClientRegistrationMetadata{}, fmt.Errorf("failed to parse OAuth authorization server metadata: %w", err)
		}
	}
	if authServer.AuthorizationEndpoint == "" {
		return nil, authServer, nmcp.ClientRegistrationMetadata{}, types.NewErrBadRequest("OAuth metadata does not include authorization_endpoint")
	}
	if authServer.TokenEndpoint == "" {
		return nil, authServer, nmcp.ClientRegistrationMetadata{}, types.NewErrBadRequest("OAuth metadata does not include token_endpoint")
	}

	var registration nmcp.ClientRegistrationMetadata
	if len(metadata.ClientRegistration) > 0 {
		if err := json.Unmarshal(metadata.ClientRegistration, &registration); err != nil {
			return nil, authServer, registration, fmt.Errorf("failed to parse OAuth client registration metadata: %w", err)
		}
	}

	return metadata, authServer, nmcp.AuthServerMetadataToClientRegistration(authServer, "Obot MCP OAuth Debugger", system.MCPOAuthCallbackURL(m.serverURL), registration.Scope), nil
}

func registerOAuthDebuggerClient(ctx context.Context, registrationEndpoint string, registration nmcp.ClientRegistrationMetadata) (types.OAuthClient, error) {
	b, err := json.Marshal(registration)
	if err != nil {
		return types.OAuthClient{}, err
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, bytes.NewReader(b))
	if err != nil {
		return types.OAuthClient{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := httpClient.Do(request)
	if err != nil {
		return types.OAuthClient{}, fmt.Errorf("failed to register OAuth client: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
		return types.OAuthClient{}, fmt.Errorf("failed to register OAuth client: unexpected status %d: %s", response.StatusCode, string(body))
	}

	var registered types.OAuthClient
	if err := json.NewDecoder(response.Body).Decode(&registered); err != nil {
		return registered, fmt.Errorf("failed to decode OAuth client registration response: %w", err)
	}
	return registered, nil
}

func (m *MCPHandler) lookupStaticOAuthClient(req api.Context, server v1.MCPServer, authServerURL string) (string, string, error) {
	if server.Spec.MCPServerCatalogEntryName != "" {
		credName := system.MCPOAuthCredentialName(server.Spec.MCPServerCatalogEntryName)
		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credName}, "oauth")
		if err == nil && cred.Env["CLIENT_ID"] != "" && cred.Env["CLIENT_SECRET"] != "" {
			return cred.Env["CLIENT_ID"], cred.Env["CLIENT_SECRET"], nil
		}
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return "", "", err
		}
	}

	var oauthApps v1.OAuthAppList
	if err := req.Storage.List(req.Context(), &oauthApps, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.manifest.authorizationServerURL": authServerURL,
		}),
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return "", "", err
	}
	for _, oauthApp := range oauthApps.Items {
		if oauthApp.Spec.Manifest.ClientID != "" && oauthApp.Spec.Manifest.ClientSecret != "" {
			return oauthApp.Spec.Manifest.ClientID, oauthApp.Spec.Manifest.ClientSecret, nil
		}
	}

	return "", "", nil
}

func oauthDebuggerConfig(clientID, clientSecret, authURL, tokenURL, tokenEndpointAuthMethod, redirectURL, scope string) *oauth2.Config {
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   authURL,
			TokenURL:  tokenURL,
			AuthStyle: oauthDebuggerAuthStyle(tokenEndpointAuthMethod),
		},
		RedirectURL: redirectURL,
	}
	if scope != "" {
		conf.Scopes = strings.Split(scope, " ")
	}
	return conf
}

func oauthDebuggerConfigFromPendingState(pendingState *gwtypes.MCPOAuthPendingState) *oauth2.Config {
	conf := &oauth2.Config{
		ClientID:     pendingState.ClientID,
		ClientSecret: pendingState.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   pendingState.AuthURL,
			TokenURL:  pendingState.TokenURL,
			AuthStyle: pendingState.AuthStyle,
		},
		RedirectURL: pendingState.RedirectURL,
	}
	if pendingState.Scopes != "" {
		conf.Scopes = strings.Split(pendingState.Scopes, " ")
	}
	return conf
}

func oauthDebuggerAuthStyle(method string) oauth2.AuthStyle {
	switch method {
	case "client_secret_basic":
		return oauth2.AuthStyleInHeader
	case "client_secret_post":
		return oauth2.AuthStyleInParams
	default:
		return oauth2.AuthStyleAutoDetect
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
