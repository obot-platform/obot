package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
)

const oauthCheckClientScope = "Obot OAuth Check"

type clientTokenService interface {
	NewToken(context.Context, persistent.TokenContext) (*jwt.Token, string, error)
}

type Client struct {
	*nmcp.Client

	jwt        *jwt.Token
	serverName string
	cacheID    string
	retireOnce sync.Once
}

func (c *Client) retire() {
	c.retireOnce.Do(func() {
		c.Client.Close(true)
	})
}

func (c *Client) hasValidToken() bool {
	if c.jwt != nil {
		expiration, err := c.jwt.Claims.GetExpirationTime()
		return err == nil && (expiration == nil || expiration.After(time.Now().Add(5*time.Minute)))
	}
	return false
}

func (sm *SessionManager) ClientForMCPServerForOAuthCheck(ctx context.Context, serverConfig ServerConfig, opt nmcp.ClientOption) (*Client, error) {
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

	return sm.clientForServerWithOptions(ctx, clientScope, serverConfig, nmcp.ClientOption{
		ClientName: clientName,
	})
}

func (sm *SessionManager) clientForServerWithOptions(ctx context.Context, clientScope string, serverConfig ServerConfig, opt nmcp.ClientOption) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	return sm.loadSession(ctx, serverConfig, clientScope, opt)
}

func (sm *SessionManager) loadSession(ctx context.Context, server ServerConfig, clientScope string, clientOpts nmcp.ClientOption) (*Client, error) {
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

		if clientSessions.CompareAndDelete(clientScope, c) {
			sm.deferClientRetirement(c)
		}
	}

	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.sessionCtx, sm.cancel = context.WithCancel(context.Background())
	}
	sm.contextLock.Unlock()

	headers := make(headerMap, len(server.PassthroughHeaderNames)+len(server.Headers))
	copyHeaders(headers, server.PassthroughHeaderNames, server.PassthroughHeaderValues)
	copyListIntoMap(headers, server.Headers)

	var jwtToken *jwt.Token
	// If the token storage is not set, then this is a client we use in our API.
	// This needs authentication for it to work.
	if clientOpts.TokenStorage == nil {
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

		headers.Set("Authorization", "Bearer "+token)
	}

	url := server.URL
	if !isOAuthCheck {
		url = sm.TransformObotHostname(system.MCPConnectURL(sm.baseURL, server.MCPServerName))
	}

	c, err := nmcp.NewClient(sm.sessionCtx, server.MCPServerDisplayName, nmcp.Server{
		BaseURL: url,
		Headers: headers,
	}, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	result := &Client{
		Client:     c,
		jwt:        jwtToken,
		serverName: server.MCPServerName,
		cacheID:    clientScope,
	}

	for {
		res, loaded := clientSessions.LoadOrStore(clientScope, result)
		if !loaded {
			return result, nil
		}

		existing := res.(*Client)
		if existing.hasValidToken() {
			result.retire()
			return existing, nil
		}

		// Replace only the client we observed. A concurrent caller may have already
		// installed a newer client, in which case retry against the current value.
		if clientSessions.CompareAndSwap(clientScope, existing, result) {
			sm.deferClientRetirement(existing)
			return result, nil
		}
	}
}

func (sm *SessionManager) deferClientRetirement(client *Client) {
	if sm.clientRetirementDelay <= 0 {
		client.retire()
		return
	}

	entry := &retiredClient{client: client}

	sm.retiredClientsLock.Lock()
	if sm.retiredClients == nil {
		sm.retiredClients = map[string]*retiredClient{}
	}

	previous := sm.retiredClients[client.serverName]
	sm.retiredClients[client.serverName] = entry
	entry.timer = time.AfterFunc(sm.clientRetirementDelay, func() {
		sm.expireRetiredClient(client.serverName, entry)
	})
	sm.retiredClientsLock.Unlock()

	// Keep at most one client in the grace period for each MCP server. This
	// bounds the remote process fan-out to the current session plus one retiring
	// session even during a reconnect burst.
	if previous != nil {
		previous.timer.Stop()
		previous.client.retire()
	}
}

func (sm *SessionManager) expireRetiredClient(serverName string, target *retiredClient) {
	sm.retiredClientsLock.Lock()
	if sm.retiredClients[serverName] != target {
		sm.retiredClientsLock.Unlock()
		return
	}
	delete(sm.retiredClients, serverName)
	sm.retiredClientsLock.Unlock()

	target.client.retire()
}

func (sm *SessionManager) flushRetiredClients(serverName, cacheID string) {
	var retiredClients []*retiredClient

	sm.retiredClientsLock.Lock()
	for name, retired := range sm.retiredClients {
		if serverName != "" && name != serverName {
			continue
		}
		if cacheID != "" && retired.client.cacheID != cacheID {
			continue
		}

		retiredClients = append(retiredClients, retired)
		delete(sm.retiredClients, name)
	}
	sm.retiredClientsLock.Unlock()

	for _, retired := range retiredClients {
		retired.timer.Stop()
		retired.client.retire()
	}
}

func copyListIntoMap(m map[string]string, list []string) {
	for _, s := range list {
		k, v, ok := strings.Cut(s, "=")
		if ok {
			m[k] = v
		}
	}
}
