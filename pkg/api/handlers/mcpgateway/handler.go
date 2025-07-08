package mcpgateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/mvl"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = mvl.Package()

type Handler struct {
	gptscript         *gptscript.GPTScript
	mcpSessionManager *mcp.SessionManager
	sessions          *sessionStoreFactory
	pendingRequests   *nmcp.PendingRequests
	tokenStore        GlobalTokenStore
	baseURL           string
}

func NewHandler(storageClient kclient.Client, gptClient *gptscript.GPTScript, mcpSessionManager *mcp.SessionManager, gatewayClient *gateway.Client, baseURL string) *Handler {
	return &Handler{
		gptscript:         gptClient,
		mcpSessionManager: mcpSessionManager,
		sessions: &sessionStoreFactory{
			client:       storageClient,
			sessionCache: sync.Map{},
		},
		pendingRequests: &nmcp.PendingRequests{},
		tokenStore:      NewGlobalTokenStore(gatewayClient),
		baseURL:         baseURL,
	}
}

func (h *Handler) StreamableHTTP(req api.Context) error {
	sessionID := req.Request.Header.Get("Mcp-Session-Id")
	mcpServer, mcpServerConfig, err := handlers.ServerFromMCPServerInstance(req, h.gptscript)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the MCP server is not found, remove the session.
			if sessionID != "" {
				// We don't need to supply a handler here because the server is not using this session.
				session, found, err := h.sessions.NewStore(nil).LoadAndDelete(req.Request, sessionID)
				if err != nil {
					return fmt.Errorf("failed to get mcp server config: %w", err)
				}

				if found {
					session.Close()
				}
			}
		}
		return fmt.Errorf("failed to get mcp server config: %w", err)
	}

	handler := &messageHandler{
		handler:               h,
		mcpServerInstanceName: req.PathValue("mcp_server_instance_id"),
		client:                req.Storage,
		resp:                  req.ResponseWriter,
		serverConfig:          mcpServerConfig,
		mcpServer:             mcpServer,
	}
	nmcp.NewHTTPServer(nil, handler, nmcp.HTTPServerOptions{SessionStore: h.sessions.NewStore(handler)}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

type messageHandler struct {
	handler               *Handler
	mcpServerInstanceName string
	client                kclient.Client
	resp                  http.ResponseWriter
	mcpServer             v1.MCPServer
	serverConfig          mcp.ServerConfig
}

func (m *messageHandler) OnMessage(ctx context.Context, msg nmcp.Message) {
	if m.handler.pendingRequests.Notify(msg) {
		// This is a response to a pending request.
		// We don't forward it to the client, just return.
		return
	}

	// If an unauthorized error occurs, send the proper status code.
	var (
		client *nmcp.Client
		err    error
	)
	defer func() {
		if err != nil {
			var oauthErr nmcp.AuthRequiredErr
			if errors.As(err, &oauthErr) {
				m.resp.Header().Set(
					"WWW-Authenticate",
					fmt.Sprintf(`Bearer error="invalid_token", error_description="The access token is invalid or expired. Please re-authenticate and try again.", resource_metadata="%s/.well-known/oauth-protected-resource/%s"`, m.handler.baseURL, m.mcpServerInstanceName),
				)
				http.Error(m.resp, fmt.Sprintf("Unauthorized: %v", oauthErr), http.StatusUnauthorized)
				return
			}

			if rpcError := (*nmcp.RPCError)(nil); errors.As(err, &rpcError) {
				msg.SendError(ctx, rpcError)
				return
			}

			msg.SendError(ctx, &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to send message to server %s: %v", m.mcpServer.Name, err),
			})
		}
	}()

	client, err = m.handler.mcpSessionManager.ClientForServer(ctx, m.mcpServer, m.serverConfig, m.clientMessageHandlerAsClientOption(m.handler.tokenStore.ForServerInstance(m.mcpServerInstanceName), msg.Session))
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
		sessionID := msg.Session.ID()
		context.AfterFunc(ctx, func() {
			if err := m.handler.mcpSessionManager.CloseClient(context.Background(), m.serverConfig); err != nil {
				log.Errorf("Failed to shutdown server %s: %v", m.mcpServer.Name, err)
			}

			req, err := http.NewRequest(http.MethodDelete, "", nil)
			if err != nil {
				log.Errorf("Failed to create request to delete session %s: %v", sessionID, err)
				return
			}
			req.Header.Set("Mcp-Session-Id", sessionID)

			if _, _, err := m.handler.sessions.NewStore(m).LoadAndDelete(req, sessionID); err != nil {
				log.Errorf("Failed to delete session %s: %v", sessionID, err)
			}
		})

		if client.Session.InitializeResult.ServerInfo.Name != "" || client.Session.InitializeResult.ServerInfo.Version != "" {
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
