package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gtypes "github.com/gptscript-ai/gptscript/pkg/types"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
)

type Client struct {
	*nmcp.Client
	ID     string
	Config ServerConfig

	jwt *jwt.Token
}

func (c *Client) hasValidToken() bool {
	if c.jwt != nil {
		expiration, err := c.jwt.Claims.GetExpirationTime()
		return err == nil && (expiration == nil || !expiration.Before(time.Now().Add(-5*time.Minute)))
	}
	return false
}

func (sm *SessionManager) ClientForMCPServerForOAuthCheck(ctx context.Context, clientScope string, serverConfig ServerConfig, opt nmcp.ClientOption) (*Client, error) {
	return sm.clientForServerWithOptions(ctx, clientScope, serverConfig, false, opt)
}

func (sm *SessionManager) clientForMCPServer(ctx context.Context, serverConfig ServerConfig) (*Client, error) {
	return sm.clientForMCPServerWithClientScope(ctx, "default", serverConfig)
}

func (sm *SessionManager) clientForMCPServerWithClientScope(ctx context.Context, clientScope string, serverConfig ServerConfig) (*Client, error) {
	return sm.clientForServerWithScope(ctx, clientScope, serverConfig)
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

	return sm.clientForServerWithOptions(ctx, clientScope, serverConfig, true, nmcp.ClientOption{
		ClientName: clientName,
	})
}

func (sm *SessionManager) clientForServerWithOptions(ctx context.Context, clientScope string, serverConfig ServerConfig, transformRemote bool, opt nmcp.ClientOption) (*Client, error) {
	config, err := sm.ensureDeployment(ctx, serverConfig, transformRemote)
	if err != nil {
		return nil, err
	}

	session, err := sm.loadSession(config, clientScope, opt)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (sm *SessionManager) loadSession(server ServerConfig, clientScope string, clientOpts nmcp.ClientOption) (*Client, error) {
	sessions, _ := sm.sessions.LoadOrStore(server.MCPServerName, &sync.Map{})

	clientSessions, ok := sessions.(*sync.Map)
	if !ok || clientSessions == nil {
		// Shouldn't happen, but handle it anyway
		clientSessions = &sync.Map{}
		sm.sessions.Store(server.MCPServerName, clientSessions)
	}

	clientScope = clientID(server) + clientScope

	existing, ok := clientSessions.Load(clientScope)
	if ok && existing != nil {
		c := existing.(*Client)
		if c.hasValidToken() {
			return c, nil
		}

		c.Close(false)
		clientSessions.Delete(clientScope)
	}

	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.sessionCtx, sm.cancel = context.WithCancel(context.Background())
	}
	sm.contextLock.Unlock()

	headers := splitIntoMap(server.Headers)

	var jwtToken *jwt.Token
	// If the token storage is not set, then this is a client we use in our API.
	// This needs authentication for it to work.
	if clientOpts.TokenStorage == nil {
		var (
			token string
			err   error
		)

		now := time.Now().Add(-time.Second)
		// TODO(thedadams): This needs to be fixed before user information headers can be passed to the MCP server.
		jwtToken, token, err = sm.tokenService.NewTokenWithClaims(jwt.MapClaims{
			"aud": gtypes.FirstSet(server.Audiences...),
			"exp": float64(now.Add(time.Hour + 15*time.Minute).Unix()),
			"iat": float64(now.Unix()),
			"sub": server.UserID,
			// "name":       server.UserName,
			// "email":      server.UserEmail,
			// "picture":    server.Picture,
			// "UserGroups": strings.Join(server.UserGroups, ","),
			"MCPID": server.MCPServerName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create JWT token for client: %w", err)
		}

		headers["Authorization"] = "Bearer " + token
	}

	c, err := nmcp.NewClient(sm.sessionCtx, server.MCPServerDisplayName, nmcp.Server{
		Env:     splitIntoMap(server.Env),
		Command: server.Command,
		Args:    server.Args,
		BaseURL: server.URL,
		Headers: headers,
	}, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	result := &Client{
		ID:     clientScope,
		Client: c,
		Config: server,
		jwt:    jwtToken,
	}

	res, ok := clientSessions.LoadOrStore(clientScope, result)
	if ok {
		existing := res.(*Client)
		if existing.hasValidToken() {
			result.Close(true)
			return existing, nil
		}

		// Swap the existing client with the new one and close the old one.
		clientSessions.Swap(clientScope, result)
		existing.Close(false)
	}

	return result, nil
}

func (sm *SessionManager) getClient(id, clientScope string) *Client {
	sessions, _ := sm.sessions.LoadOrStore(id, &sync.Map{})

	clientSessions, ok := sessions.(*sync.Map)
	if !ok || clientSessions == nil {
		// Shouldn't happen, but handle it anyway
		clientSessions = &sync.Map{}
		sm.sessions.Store(id, clientSessions)
	}

	existing, ok := clientSessions.Load(clientScope)
	if ok && existing != nil {
		return existing.(*Client)
	}

	return nil
}

func splitIntoMap(list []string) map[string]string {
	result := make(map[string]string, len(list))
	for _, s := range list {
		k, v, ok := strings.Cut(s, "=")
		if ok {
			result[k] = v
		}
	}
	return result
}
