package mcpgateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/mvl"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var log = mvl.Package()

type Handler struct {
	gptscript         *gptscript.GPTScript
	mcpSessionManager *mcp.SessionManager
	sessions          nmcp.SessionStore
	pendingRequests   *nmcp.PendingRequests
}

func NewHandler(gptClient *gptscript.GPTScript, mcpSessionManager *mcp.SessionManager) *Handler {
	return &Handler{
		gptscript:         gptClient,
		mcpSessionManager: mcpSessionManager,
		sessions:          nmcp.NewInMemorySessionStore(),
		pendingRequests:   &nmcp.PendingRequests{},
	}
}

func (h *Handler) StreamableHTTP(req api.Context) error {
	mcpServer, mcpServerConfig, err := handlers.ServerForAction(req, h.gptscript)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the MCP server is not found, remove the session.
			id := req.Request.Header.Get("Mcp-Session-Id")
			if id != "" {
				session, found := h.sessions.LoadAndDelete(id)
				if found {
					session.Close()
				}
			}
		}
		return fmt.Errorf("failed to get mcp server config: %w", err)
	}

	nmcp.NewHTTPServer(nil, &messageHandler{
		handler:         h,
		serverConfig:    mcpServerConfig,
		mcpServer:       mcpServer,
		pendingRequests: h.pendingRequests,
	}, nmcp.HTTPServerOptions{SessionStore: h.sessions}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

type messageHandler struct {
	handler         *Handler
	mcpServer       v1.MCPServer
	serverConfig    mcp.ServerConfig
	pendingRequests *nmcp.PendingRequests
}

func (m *messageHandler) OnMessage(ctx context.Context, msg nmcp.Message) {
	if m.pendingRequests.Notify(msg) {
		// This is a response to a pending request.
		// We don't forward it to the client, just return.
		return
	}

	m.serverConfig.Scope = msg.Session.ID()
	client, err := m.handler.mcpSessionManager.ClientForServer(ctx, m.mcpServer, m.serverConfig, clientMessageHandlerAsClientOption(msg.Session, m.pendingRequests))
	if err != nil {
		log.Errorf("Failed to get client for server %s: %v", m.mcpServer.Name, err)
		return
	}

	var result any
	switch msg.Method {
	case "notifications/initialized":
		// This method is special because it is handled automatically by the client.
		// So, we don't forward this one, just respond with a success.
		if err = msg.Reply(ctx, nmcp.Notification{}); err != nil {
			log.Errorf("failed to reply to notifications/initialized: %v", err)
		}
		return
	case "ping":
		result = nmcp.PingResult{}
	case "initialize":
		id := msg.Session.ID()
		context.AfterFunc(ctx, func() {
			if err := m.handler.mcpSessionManager.CloseClient(context.Background(), m.serverConfig); err != nil {
				log.Errorf("Failed to shutdown server %s: %v", m.mcpServer.Name, err)
			}
			m.handler.sessions.LoadAndDelete(id)
		})

		if client.Session.InitializeResult != nil {
			if err = msg.Reply(ctx, client.Session.InitializeResult); err != nil {
				log.Errorf("Failed to reply to server %s: %v", m.mcpServer.Name, err)
				msg.SendError(ctx, &nmcp.RPCError{
					Code:    -32603,
					Message: fmt.Sprintf("failed to reply to server %s: %v", m.mcpServer.Name, err),
				})
			}
			return
		}

		result = nmcp.InitializeResult{}
	case "resources/read":
		result = nmcp.ReadResourceResult{}
	case "resources/list":
		result = nmcp.ListResourcesResult{}
	case "resources/templates/list":
		result = nmcp.ListResourceTemplatesResult{}
	case "prompts/list":
		result = nmcp.ListPromptsResult{}
	case "prompts/get":
		result = nmcp.GetPromptResult{}
	case "tools/list":
		result = nmcp.ListToolsResult{}
	case "tools/call":
		result = nmcp.CallToolResult{}
	case "notifications/progress", "notifications/roots/list_changed", "notifications/cancelled", "logging/setLevel":
		// These methods don't require a result.
		result = nmcp.Notification{}
	default:
		log.Errorf("Unknown method for server message: %s", msg.Method)
		msg.SendError(ctx, &nmcp.RPCError{
			Code:    -32601,
			Message: "Method not allowed",
		})
		return
	}

	if err = client.Session.Exchange(ctx, msg.Method, &msg, &result); err != nil {
		log.Errorf("Failed to send %s message to server %s: %v", msg.Method, m.mcpServer.Name, err)
		if rpcError := (*nmcp.RPCError)(nil); errors.As(err, &rpcError) {
			msg.SendError(ctx, rpcError)
			return
		}

		msg.SendError(ctx, &nmcp.RPCError{
			Code:    -32603,
			Message: fmt.Sprintf("failed to send message to server %s: %v", m.mcpServer.Name, err),
		})
		return
	}

	if err = msg.Reply(ctx, result); err != nil {
		log.Errorf("Failed to reply to server %s: %v", m.mcpServer.Name, err)
		msg.SendError(ctx, &nmcp.RPCError{
			Code:    -32603,
			Message: fmt.Sprintf("failed to reply to server %s: %v", m.mcpServer.Name, err),
		})
		return
	}
}
