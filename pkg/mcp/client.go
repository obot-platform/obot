package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Client struct {
	*nmcp.Client
	ID     string
	Config ServerConfig
}

func (c *Client) Capabilities() nmcp.ServerCapabilities {
	return c.Session.InitializeResult.Capabilities
}

func (sm *SessionManager) ClientForMCPServerWithOptions(ctx context.Context, clientScope string, mcpServer v1.MCPServer, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*Client, error) {
	mcpServerName := mcpServer.Spec.Manifest.Name
	if mcpServerName == "" {
		mcpServerName = mcpServer.Name
	}

	return sm.clientForServerWithOptions(ctx, clientScope, mcpServerName, serverConfig, opts...)
}

func (sm *SessionManager) ClientForMCPServer(ctx context.Context, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) (*Client, error) {
	return sm.clientForMCPServerWithClientScope(ctx, "default", userID, mcpServer, serverConfig)
}

func (sm *SessionManager) clientForMCPServerWithClientScope(ctx context.Context, clientScope, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) (*Client, error) {
	mcpServerName := mcpServer.Spec.Manifest.Name
	if mcpServerName == "" {
		mcpServerName = mcpServer.Name
	}

	return sm.clientForServerWithScope(ctx, clientScope, userID, mcpServerName, mcpServer.Name, serverConfig)
}

func (sm *SessionManager) ClientForServer(ctx context.Context, userID, mcpServerName, mcpServerID string, serverConfig ServerConfig) (*Client, error) {
	return sm.clientForServerWithScope(ctx, "default", userID, mcpServerName, mcpServerID, serverConfig)
}

func (sm *SessionManager) clientForServerWithScope(ctx context.Context, clientScope, userID, mcpServerName, mcpServerID string, serverConfig ServerConfig) (*Client, error) {
	clientName := "Obot MCP Gateway"
	if strings.HasPrefix(serverConfig.URL, fmt.Sprintf("%s/mcp-connect/", sm.baseURL)) {
		// If the URL points back to us, then this is Obot chat. Ensure the client name reflects that.
		clientName = "Obot Chat"
	}

	return sm.clientForServerWithOptions(ctx, clientScope, mcpServerName, serverConfig, nmcp.ClientOption{
		ClientName:   clientName,
		TokenStorage: sm.tokenStorage.ForUserAndMCP(userID, mcpServerID),
	})
}

func (sm *SessionManager) clientForServerWithOptions(ctx context.Context, clientScope, mcpServerName string, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*Client, error) {
	config, err := sm.transformServerConfig(ctx, mcpServerName, serverConfig)
	if err != nil {
		return nil, err
	}

	session, err := sm.loadSession(config, clientScope, mcpServerName, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return session, nil
}

func (sm *SessionManager) loadSession(server ServerConfig, clientScope, serverName string, clientOpts ...nmcp.ClientOption) (*Client, error) {
	id := deploymentID(server)
	sessions, _ := sm.sessions.LoadOrStore(id, &sync.Map{})

	clientSessions, ok := sessions.(*sync.Map)
	if !ok || clientSessions == nil {
		// Shouldn't happen, but handle it anyway
		clientSessions = &sync.Map{}
		sm.sessions.Store(id, clientSessions)
	}

	existing, ok := clientSessions.Load(clientScope)
	if ok && existing != nil {
		return existing.(*Client), nil
	}

	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.sessionCtx, sm.cancel = context.WithCancel(context.Background())
	}
	sm.contextLock.Unlock()

	c, err := nmcp.NewClient(sm.sessionCtx, serverName, nmcp.Server{
		Env:     splitIntoMap(server.Env),
		Command: server.Command,
		Args:    server.Args,
		BaseURL: server.URL,
		Headers: splitIntoMap(server.Headers),
	}, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	result := &Client{
		ID:     id,
		Client: c,
		Config: server,
	}

	res, ok := clientSessions.LoadOrStore(clientScope, result)
	if ok {
		c.Session.Close()
		return res.(*Client), nil
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
