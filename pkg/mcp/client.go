package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
)

const oauthCheckClientScope = "Obot OAuth Check"

type Client struct {
	*gomcp.ClientSession

	jwt *jwt.Token
}

type ClientOption struct {
	OAuthClientName               string
	OAuthRedirectURL              string
	OAuthClientIDMetadataDocument string
	ClientName                    string
	ClientVersion                 string
	TokenStorage                  TokenStorage
	CallbackHandler               CallbackHandler
	ClientLookup                  ClientCredLookup
}

func (c *Client) hasValidToken() bool {
	if c.jwt != nil {
		expiration, err := c.jwt.Claims.GetExpirationTime()
		return err == nil && (expiration == nil || expiration.After(time.Now().Add(5*time.Minute)))
	}
	return false
}

func (sm *SessionManager) ClientForMCPServerForOAuthCheck(ctx context.Context, serverConfig ServerConfig, opt ClientOption) (*Client, error) {
	return sm.clientForServerWithOptions(ctx, oauthCheckClientScope, serverConfig, opt)
}

func (sm *SessionManager) clientForServer(ctx context.Context, serverConfig ServerConfig) (*Client, error) {
	return sm.clientForServerWithScope(ctx, "default", serverConfig)
}

func (sm *SessionManager) clientForServerWithScope(ctx context.Context, clientScope string, serverConfig ServerConfig) (*Client, error) {
	clientName := "Obot MCP Gateway"
	if serverConfig.Runtime == types.RuntimeRemote && strings.HasPrefix(serverConfig.URL, fmt.Sprintf("%s/mcp-connect/", sm.baseURL)) {
		// If the URL points back to us, then this is Obot chat. Ensure the client name reflects that.
		clientName = "Obot Chat"
	}

	return sm.clientForServerWithOptions(ctx, clientScope, serverConfig, ClientOption{
		ClientName: clientName,
	})
}

func (sm *SessionManager) clientForServerWithOptions(ctx context.Context, clientScope string, serverConfig ServerConfig, opt ClientOption) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	return sm.loadSession(ctx, serverConfig, clientScope, opt)
}

func (sm *SessionManager) loadSession(ctx context.Context, server ServerConfig, clientScope string, clientOpts ClientOption) (*Client, error) {
	sessions, _ := sm.sessions.LoadOrStore(server.MCPServerName, &sync.Map{})

	clientSessions, ok := sessions.(*sync.Map)
	if !ok || clientSessions == nil {
		// Shouldn't happen, but handle it anyway
		clientSessions = &sync.Map{}
		sm.sessions.Store(server.MCPServerName, clientSessions)
	}

	isOAuthCheck := clientScope == oauthCheckClientScope
	clientScope = clientID(server, clientScope)

	existing, ok := clientSessions.Load(clientScope)
	if ok && existing != nil {
		c := existing.(*Client)
		if c.hasValidToken() {
			return c, nil
		}

		clientSessions.Delete(clientScope)
		go func() {
			time.Sleep(time.Minute)
			c.Close()
		}()
	}

	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.sessionCtx, sm.cancel = context.WithCancel(context.Background())
	}
	sm.contextLock.Unlock()

	var (
		jwtToken *jwt.Token
		headers  headerMap
	)
	// If the token storage is not set, then this is a client we use in our API.
	// This needs authentication for it to work.
	// If this is a system client, we don't need to authenticate because we are talking directly to the MCP server.
	if clientOpts.TokenStorage == nil && server.UserID != "system" {
		var (
			token string
			err   error
		)

		now := time.Now().Add(-time.Second)
		jwtToken, token, err = sm.tokenService.NewToken(ctx, persistent.TokenContext{
			Audience:   utils.FirstSet(server.Audiences...),
			ExpiresAt:  persistent.NewTime(now.Add(time.Hour + 15*time.Minute)),
			IssuedAt:   persistent.NewTime(now),
			UserID:     server.UserID,
			MCPID:      server.MCPServerName,
			UserGroups: []string{types.GroupMCP, types.GroupAuthenticated},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create JWT token for client: %w", err)
		}

		// Clear the headers because we are talking to Obot directly and the gateway will set the correct headers.
		// We just need the token to talk to Obot.
		headers = headerMap{"Authorization": []string{"Bearer " + token}}
	} else {
		headers = serverConfigHeaders(server)
	}

	var (
		url          = server.URL
		allowedHosts []string
	)
	if isOAuthCheck || server.UserID == "system" {
		if server.TunnelName != "" {
			if sm.tunnelManager == nil {
				return nil, fmt.Errorf("tunnel manager is not configured")
			}

			var err error
			url, err = sm.tunnelManager.BridgeURL(server.TunnelName, url)
			if err != nil {
				return nil, fmt.Errorf("failed to prepare tunneled MCP server URL: %w", err)
			}

			bridgeAuthorizationName, bridgeAuthorizationValue := sm.tunnelManager.BridgeAuthorization()
			headers.Set(bridgeAuthorizationName, bridgeAuthorizationValue)
			allowedHosts = append(allowedHosts, sm.tunnelManager.BridgeHost())
		}
	} else {
		obotBaseURL := sm.TransformObotHostname(sm.baseURL)
		_, obotHostname, _ := strings.Cut(obotBaseURL, "://")
		allowedHosts = append(allowedHosts, obotHostname)
		url = system.MCPConnectURL(obotBaseURL, server.MCPServerName)
	}

	c := gomcp.NewClient(&gomcp.Implementation{
		Name:    clientOpts.ClientName,
		Title:   clientOpts.ClientName,
		Version: clientOpts.ClientVersion,
		// Empty client capabilities means no capabilities are supported.
		// That's OK because this is just used for listing/getting tools, prompts, resources, etc.
	}, &gomcp.ClientOptions{Capabilities: &gomcp.ClientCapabilities{}})

	httpClient, err := sm.HTTPClientForServer(server, allowedHosts, http.Header(headers), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	var oauthHandler auth.OAuthHandler
	if clientOpts.TokenStorage != nil {
		oauthHandler = newOAuth(httpClient, clientOpts.CallbackHandler, clientOpts.ClientLookup, clientOpts.TokenStorage, server.MCPServerName, clientOpts.ClientName, sm.baseURL+"/oauth/mcp/callback", system.OAuthClientIDMetadataURL(sm.baseURL))
	}

	session, err := c.Connect(ctx, &gomcp.StreamableClientTransport{
		Endpoint:             url,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
		OAuthHandler:         oauthHandler,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect MCP client: %w", err)
	}

	result := &Client{
		ClientSession: session,
		jwt:           jwtToken,
	}

	res, ok := clientSessions.LoadOrStore(clientScope, result)
	if ok {
		existing := res.(*Client)
		if existing.hasValidToken() {
			result.Close()
			return existing, nil
		}

		// Swap the existing client with the new one and close the old one.
		clientSessions.Swap(clientScope, result)
		go func() {
			time.Sleep(time.Minute)
			existing.Close()
		}()
	}

	return result, nil
}
