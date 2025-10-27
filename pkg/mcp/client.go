package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
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

func (sm *SessionManager) ClientForMCPServerWithOptions(ctx context.Context, userID, clientScope string, mcpServer v1.MCPServer, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*Client, error) {
	mcpServerDisplayName := mcpServer.Spec.Manifest.Name
	if mcpServerDisplayName == "" {
		mcpServerDisplayName = mcpServer.Name
	}

	return sm.clientForServerWithOptions(ctx, userID, clientScope, mcpServerDisplayName, mcpServer.Name, serverConfig, opts...)
}

func (sm *SessionManager) ClientForMCPServer(ctx context.Context, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) (*Client, error) {
	return sm.clientForMCPServerWithClientScope(ctx, "default", userID, mcpServer, serverConfig)
}

func (sm *SessionManager) clientForMCPServerWithClientScope(ctx context.Context, clientScope, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) (*Client, error) {
	mcpServerDisplayName := mcpServer.Spec.Manifest.Name
	if mcpServerDisplayName == "" {
		mcpServerDisplayName = mcpServer.Name
	}

	return sm.clientForServerWithScope(ctx, clientScope, userID, mcpServerDisplayName, mcpServer.Name, serverConfig)
}

func (sm *SessionManager) ClientForServer(ctx context.Context, userID, mcpServerDisplayName, mcpServerName string, serverConfig ServerConfig) (*Client, error) {
	return sm.clientForServerWithScope(ctx, "default", userID, mcpServerDisplayName, mcpServerName, serverConfig)
}

func (sm *SessionManager) clientForServerWithScope(ctx context.Context, clientScope, userID, mcpServerDisplayName, mcpServerName string, serverConfig ServerConfig) (*Client, error) {
	clientName := "Obot MCP Gateway"
	var tokenStorage nmcp.TokenStorage
	if (serverConfig.Runtime == types.RuntimeRemote || serverConfig.Runtime == types.RuntimeComposite) && strings.HasPrefix(serverConfig.URL, fmt.Sprintf("%s/mcp-connect/", sm.baseURL)) {
		// If the URL points back to us (mcp-connect), then this is Obot chat. Ensure the client name reflects that.
		clientName = "Obot Chat"
	} else {
		tokenStorage = sm.tokenStorage.ForUserAndMCP(userID, mcpServerName)
	}

	return sm.clientForServerWithOptions(ctx, userID, clientScope, mcpServerDisplayName, mcpServerName, serverConfig, nmcp.ClientOption{
		ClientName:   clientName,
		TokenStorage: tokenStorage,
	})
}

func (sm *SessionManager) clientForServerWithOptions(ctx context.Context, userID, clientScope, mcpServerDisplayName, mcpServerName string, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*Client, error) {
	config, err := sm.transformServerConfig(ctx, userID, mcpServerDisplayName, mcpServerName, serverConfig)
	if err != nil {
		return nil, err
	}

	session, err := sm.loadSession(config, clientScope, mcpServerDisplayName, opts...)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (sm *SessionManager) loadSession(server ServerConfig, clientScope, mcpServerDisplayName string, clientOpts ...nmcp.ClientOption) (*Client, error) {
	id := clientID(server)
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

	c, err := nmcp.NewClient(sm.sessionCtx, mcpServerDisplayName, nmcp.Server{
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
		c.Session.Close(true)
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
