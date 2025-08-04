package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/hash"
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

func (sm *SessionManager) ClientForMCPServerWithOptions(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*Client, error) {
	mcpServerName := mcpServer.Spec.Manifest.Name
	if mcpServerName == "" {
		mcpServerName = mcpServer.Name
	}

	return sm.clientForServerWithOptions(ctx, mcpServerName, serverConfig, opts...)
}

func (sm *SessionManager) ClientForMCPServer(ctx context.Context, userID string, mcpServer v1.MCPServer, serverConfig ServerConfig) (*Client, error) {
	mcpServerName := mcpServer.Spec.Manifest.Name
	if mcpServerName == "" {
		mcpServerName = mcpServer.Name
	}

	return sm.ClientForServer(ctx, userID, mcpServerName, mcpServer.Name, serverConfig)
}

func (sm *SessionManager) ClientForServer(ctx context.Context, userID, mcpServerName, mcpServerID string, serverConfig ServerConfig) (*Client, error) {
	clientName := "Obot MCP Gateway"
	if strings.HasPrefix(serverConfig.URL, fmt.Sprintf("%s/mcp-connect/", sm.baseURL)) {
		// If the URL points back to us, then this is Obot chat. Ensure the client name reflects that.
		clientName = "Obot Chat"
	}

	return sm.clientForServerWithOptions(ctx, mcpServerName, serverConfig, nmcp.ClientOption{
		ClientName:   clientName,
		TokenStorage: sm.tokenStorage.ForUserAndMCP(userID, mcpServerID),
	})
}

func (sm *SessionManager) clientForServerWithOptions(ctx context.Context, mcpServerName string, serverConfig ServerConfig, opts ...nmcp.ClientOption) (*Client, error) {
	config, err := sm.transformServerConfig(ctx, mcpServerName, serverConfig)
	if err != nil {
		return nil, err
	}

	session, err := sm.loadSession(config, mcpServerName, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return session, nil
}

func (sm *SessionManager) loadSession(server ServerConfig, serverName string, clientOpts ...nmcp.ClientOption) (*Client, error) {
	server.AllowedTools = nil

	id := hash.Digest(server)
	sm.lock.Lock()
	existing, ok := sm.sessions[id]
	if sm.sessionCtx == nil {
		sm.sessionCtx, sm.cancel = context.WithCancel(context.Background())
	}
	sm.lock.Unlock()

	if ok {
		return existing, nil
	}

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

	sm.lock.Lock()
	defer sm.lock.Unlock()

	if existing, ok = sm.sessions[id]; ok {
		c.Session.Close()
		return existing, nil
	}

	if sm.sessions == nil {
		sm.sessions = make(map[string]*Client, 1)
	}
	sm.sessions[id] = result
	return result, nil
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
