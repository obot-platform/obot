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

type Client struct {
	*nmcp.Client

	jwt *jwt.Token
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

		clientSessions.Delete(clientScope)
		go func() {
			time.Sleep(time.Minute)
			c.Close(false)
		}()
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

		headers.Set("Authorization", "Bearer "+token)
	}

	url := server.URL
	if !isOAuthCheck && server.UserID != "system" {
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
		Client: c,
		jwt:    jwtToken,
	}

	res, ok := clientSessions.LoadOrStore(clientScope, result)
	if ok {
		existing := res.(*Client)
		if existing.hasValidToken() {
			result.Close(false)
			return existing, nil
		}

		// Swap the existing client with the new one and close the old one.
		clientSessions.Swap(clientScope, result)
		go func() {
			time.Sleep(time.Minute)
			existing.Close(false)
		}()
	}

	return result, nil
}

func copyListIntoMap(m map[string]string, list []string) {
	for _, s := range list {
		k, v, ok := strings.Cut(s, "=")
		if ok {
			m[k] = v
		}
	}
}
