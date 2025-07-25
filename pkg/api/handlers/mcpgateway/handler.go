package mcpgateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"
	"github.com/gptscript-ai/gptscript/pkg/mvl"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = mvl.Package()

type Handler struct {
	tokenService      *jwt.TokenService
	mcpSessionManager *mcp.SessionManager
	webhookHelper     *mcp.WebhookHelper
	sessions          *sessionStoreFactory
	pendingRequests   *nmcp.PendingRequests
	tokenStore        mcp.GlobalTokenStore
	baseURL           string
}

func NewHandler(tokenService *jwt.TokenService, storageClient kclient.Client, mcpSessionManager *mcp.SessionManager, webhookHelper *mcp.WebhookHelper, globalTokenStore mcp.GlobalTokenStore, baseURL string) *Handler {
	return &Handler{
		tokenService:      tokenService,
		mcpSessionManager: mcpSessionManager,
		webhookHelper:     webhookHelper,
		sessions: &sessionStoreFactory{
			client: storageClient,
		},
		pendingRequests: &nmcp.PendingRequests{},
		tokenStore:      globalTokenStore,
		baseURL:         baseURL,
	}
}

func (h *Handler) StreamableHTTP(req api.Context) error {
	sessionID := req.Request.Header.Get("Mcp-Session-Id")
	mcpID := req.PathValue("mcp_id")

	var (
		mcpServer       v1.MCPServer
		mcpServerConfig mcp.ServerConfig
		err             error
	)

	if strings.HasPrefix(mcpID, system.MCPServerInstancePrefix) {
		mcpServer, mcpServerConfig, err = handlers.ServerFromMCPServerInstance(req, mcpID)
	} else {
		mcpServer, mcpServerConfig, err = handlers.ServerForActionWithID(req, mcpID)
	}

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
		handler:       h,
		mcpID:         mcpID,
		client:        req.Storage,
		gatewayClient: req.GatewayClient,
		gptClient:     req.GPTClient,
		resp:          req.ResponseWriter,
		serverConfig:  mcpServerConfig,
		mcpServer:     mcpServer,
		req:           req.Request,
		userID:        req.User.GetUID(),
	}
	nmcp.NewHTTPServer(nil, handler, nmcp.HTTPServerOptions{SessionStore: h.sessions.NewStore(handler)}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

type messageHandler struct {
	handler       *Handler
	mcpID         string
	client        kclient.Client
	gatewayClient *gateway.Client
	gptClient     *gptscript.GPTScript
	resp          http.ResponseWriter
	mcpServer     v1.MCPServer
	serverConfig  mcp.ServerConfig
	req           *http.Request
	userID        string
}

func (m *messageHandler) OnMessage(ctx context.Context, msg nmcp.Message) {
	auditLog := &gatewaytypes.MCPAuditLog{
		UserID:                    m.userID,
		MCPID:                     m.mcpID,
		MCPServerDisplayName:      m.mcpServer.Spec.Manifest.Name,
		MCPServerCatalogEntryName: m.mcpServer.Spec.MCPServerCatalogEntryName,
		ClientName:                msg.Session.InitializeRequest.ClientInfo.Name,
		ClientVersion:             msg.Session.InitializeRequest.ClientInfo.Version,
		ClientIP:                  m.getClientIP(),
		CallType:                  msg.Method,
		CallIdentifier:            m.extractCallIdentifier(msg),
		SessionID:                 msg.Session.ID(),
		UserAgent:                 m.req.UserAgent(),
		RequestHeaders:            m.captureHeaders(m.req.Header),
	}
	auditLog.RequestID, _ = msg.ID.(string)

	// Go through webhook validations.
	webhooks, err := m.handler.webhookHelper.GetWebhooksForMCPServer(ctx, m.gptClient, m.mcpServer, msg.Method, auditLog.CallIdentifier)
	if err != nil {
		log.Errorf("Failed to get webhooks for server %s: %v", m.mcpServer.Name, err)
		err = msg.Reply(ctx, &nmcp.RPCError{
			Code:    -32603,
			Message: fmt.Sprintf("failed to get webhooks for server %s: %v", m.mcpServer.Name, err),
		})
		return
	}

	signatures := make(map[string]string)
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		log.Errorf("Failed to marshal message: %v", err)
		msg.SendError(ctx, &nmcp.RPCError{
			Code:    -32603,
			Message: fmt.Sprintf("failed to marshal message: %v", err),
		})
		return
	}

	auditLog.WebhookStatuses = make([]gatewaytypes.MCPWebhookStatus, 0, len(webhooks))
	var (
		webhookStatus string
		rpcError      *nmcp.RPCError
	)
	for i, webhook := range webhooks {
		webhookStatus, rpcError = fireWebhook(ctx, httpClient, body, m.mcpID, m.userID, webhook.URL, webhook.Secret, signatures)
		auditLog.WebhookStatuses = append(auditLog.WebhookStatuses, gatewaytypes.MCPWebhookStatus{
			URL:    webhook.URL,
			Status: webhookStatus,
		})
		if rpcError != nil {
			auditLog.WebhookStatuses[i] = gatewaytypes.MCPWebhookStatus{
				URL:     webhook.URL,
				Status:  webhookStatus,
				Message: rpcError.Message,
			}
			msg.SendError(ctx, rpcError)
			err = rpcError

			m.insertAuditLog(auditLog)

			return
		}
	}

	if m.handler.pendingRequests.Notify(msg) {
		// Insert the audit log for this request. The message handler will update it with its fields.
		m.insertAuditLog(auditLog)
		// This is a response to a pending request.
		// We don't forward it to the client, just return.
		return
	}

	// Capture audit log information
	auditLog.CreatedAt = time.Now()

	// Capture request body if available
	if msg.Params != nil {
		if requestBody, err := json.Marshal(msg.Params); err == nil {
			auditLog.RequestBody = requestBody
		}
	}

	// If an unauthorized error occurs, send the proper status code.
	var (
		client *gmcp.Client
		result any
	)
	defer func() {
		// Complete audit log
		auditLog.ProcessingTimeMs = time.Since(auditLog.CreatedAt).Milliseconds()
		auditLog.ResponseHeaders = m.captureHeaders(m.resp.Header())

		if err != nil {
			auditLog.Error = err.Error()
			auditLog.ResponseStatus = http.StatusInternalServerError

			var oauthErr nmcp.AuthRequiredErr
			if errors.As(err, &oauthErr) {
				auditLog.ResponseStatus = http.StatusUnauthorized
				m.resp.Header().Set(
					"WWW-Authenticate",
					fmt.Sprintf(`Bearer error="invalid_token", error_description="The access token is invalid or expired. Please re-authenticate and try again.", resource_metadata="%s/.well-known/oauth-protected-resource%s"`, m.handler.baseURL, m.req.URL.Path),
				)
				http.Error(m.resp, fmt.Sprintf("Unauthorized: %v", oauthErr), http.StatusUnauthorized)
				m.insertAuditLog(auditLog)
				return
			}

			if rpcError := (*nmcp.RPCError)(nil); errors.As(err, &rpcError) {
				msg.SendError(ctx, rpcError)
				m.insertAuditLog(auditLog)
				return
			}

			msg.SendError(ctx, &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to send message to server %s: %v", m.mcpServer.Name, err),
			})
		} else {
			auditLog.ResponseStatus = http.StatusOK
			// Capture response body if available
			if result != nil {
				if responseBody, err := json.Marshal(result); err == nil {
					auditLog.ResponseBody = responseBody
				}
			}
		}

		m.insertAuditLog(auditLog)
	}()

	client, err = m.handler.mcpSessionManager.ClientForMCPServerWithOptions(ctx, m.mcpServer, m.serverConfig, m.clientMessageHandlerAsClientOption(m.handler.tokenStore.ForUserAndMCP(m.userID, m.mcpID), msg.Session))
	if err != nil {
		log.Errorf("Failed to get client for server %s: %v", m.mcpServer.Name, err)
		return
	}

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
		go func() {
			msg.Session.Wait()

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
		}()

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

// Helper methods for audit logging

func (m *messageHandler) getClientIP() string {
	// Check X-Forwarded-For header first
	if forwarded := m.req.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Take the first IP in the list
		if idx := strings.Index(forwarded, ","); idx != -1 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return strings.TrimSpace(forwarded)
	}

	// Check X-Real-IP header
	if realIP := m.req.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	// Fall back to RemoteAddr
	if host, _, err := net.SplitHostPort(m.req.RemoteAddr); err == nil {
		return host
	}

	return m.req.RemoteAddr
}

func (m *messageHandler) extractCallIdentifier(msg nmcp.Message) string {
	switch msg.Method {
	case "resources/read":
		return gjson.GetBytes(msg.Params, "uri").String()
	case "tools/call", "prompts/get":
		return gjson.GetBytes(msg.Params, "name").String()
	default:
		return ""
	}
}

func (m *messageHandler) captureHeaders(headers http.Header) json.RawMessage {
	// Create a filtered version of headers (removing sensitive information)
	filteredHeaders := make(map[string][]string)
	for k, v := range headers {
		// Skip sensitive headers
		if strings.EqualFold(k, "Authorization") ||
			strings.EqualFold(k, "Cookie") ||
			strings.EqualFold(k, "X-Auth-Token") {
			continue
		}
		filteredHeaders[k] = v
	}

	if data, err := json.Marshal(filteredHeaders); err == nil {
		return data
	}
	return nil
}

func (m *messageHandler) insertAuditLog(auditLog *gatewaytypes.MCPAuditLog) {
	// Insert audit log asynchronously to avoid blocking the response
	go func() {
		// Use a background context with timeout to avoid blocking
		auditCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := m.gatewayClient.InsertMCPAuditLog(auditCtx, auditLog); err != nil {
			// Log the error but don't fail the request
			log.Errorf("Failed to insert MCP audit log: %v", err)
		}
	}()
}

func (m *messageHandler) updateAuditLog(auditLog *gatewaytypes.MCPAuditLog) {
	// Insert audit log asynchronously to avoid blocking the response
	go func() {
		// Use a background context with timeout to avoid blocking
		auditCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := m.gatewayClient.UpdateMCPAuditLogByRequestID(auditCtx, auditLog); err != nil {
			// Log the error but don't fail the request
			log.Errorf("Failed to insert MCP audit log: %v", err)
		}
	}()
}

func fireWebhook(ctx context.Context, httpClient *http.Client, body []byte, mcpID, userID, url, secret string, signatures map[string]string) (string, *nmcp.RPCError) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", &nmcp.RPCError{
			Code:    -32603,
			Message: fmt.Sprintf("failed to construct request to webhook %s: %v", url, err),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	req.Header.Set("X-Obot-Mcp-Server-Id", mcpID)
	req.Header.Set("X-Obot-User-Id", userID)

	if secret != "" {
		sig := signatures[secret]
		if sig == "" {
			h := hmac.New(sha256.New, []byte(secret))
			h.Write(body)
			sig = fmt.Sprintf("sha256=%x", h.Sum(nil))
			signatures[secret] = sig
		}

		req.Header.Set("X-Obot-Signature-256", sig)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", &nmcp.RPCError{
			Code:    -32603,
			Message: fmt.Sprintf("failed to send request to webhook %s: %v", url, err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return resp.Status, &nmcp.RPCError{
			Code:    -32000,
			Message: fmt.Sprintf("webhook %s returned status code %d: %v", url, resp.StatusCode, string(respBody)),
		}
	}

	return resp.Status, nil
}
