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
	"sync"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/mvl"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/tidwall/gjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = mvl.Package()

type Handler struct {
	storageClient     kclient.Client
	gatewayClient     *gateway.Client
	gptClient         *gptscript.GPTScript
	mcpSessionManager *mcp.SessionManager
	webhookHelper     *mcp.WebhookHelper
	tokenStore        mcp.GlobalTokenStore
	pendingRequests   sync.Map
	mcpSessionCache   sync.Map
	sessionCache      sync.Map
	baseURL           string
}

func NewHandler(storageClient kclient.Client, mcpSessionManager *mcp.SessionManager, webhookHelper *mcp.WebhookHelper, globalTokenStore mcp.GlobalTokenStore, gatewayClient *gateway.Client, gptClient *gptscript.GPTScript, baseURL string) *Handler {
	return &Handler{
		storageClient:     storageClient,
		gatewayClient:     gatewayClient,
		gptClient:         gptClient,
		mcpSessionManager: mcpSessionManager,
		webhookHelper:     webhookHelper,
		tokenStore:        globalTokenStore,
		baseURL:           baseURL,
	}
}

func (h *Handler) StreamableHTTP(req api.Context) error {
	sessionID := req.Request.Header.Get("Mcp-Session-Id")

	mcpID, mcpServer, mcpServerConfig, err := handlers.ServerForActionWithConnectID(req, req.PathValue("mcp_id"))
	if err == nil && mcpServer.Spec.Template {
		// Prevent connections to MCP server templates by returning a 404.
		err = apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpserver"}, mcpID)
	}

	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the MCP server is not found, remove the session.
			if sessionID != "" {
				session, found, err := h.LoadAndDelete(req.Context(), h, sessionID)
				if err != nil {
					return fmt.Errorf("failed to get mcp server config: %w", err)
				}

				if found {
					session.Close(true)
				}
			}
		}

		return fmt.Errorf("failed to get mcp server config: %w", err)
	}

	req.Request = req.WithContext(withMessageContext(req.Context(), messageContext{
		userID:       req.User.GetUID(),
		mcpID:        mcpID,
		serverConfig: mcpServerConfig,
		mcpServer:    mcpServer,
		req:          req.Request,
		resp:         req.ResponseWriter,
	}))

	nmcp.NewHTTPServer(nil, h, nmcp.HTTPServerOptions{SessionStore: h}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

type messageContext struct {
	userID, mcpID string
	mcpServer     v1.MCPServer
	serverConfig  mcp.ServerConfig
	req           *http.Request
	resp          http.ResponseWriter
}

func (h *Handler) OnMessage(ctx context.Context, msg nmcp.Message) {
	if h.pendingRequestsForSession(msg.Session.ID()).Notify(msg) {
		// This is a response to a pending request.
		// We don't forward it to the client, just return.
		return
	}

	m, ok := messageContextFromContext(ctx)
	if !ok {
		log.Errorf("Failed to get message context from context: %v", ctx)
		msg.SendError(ctx, &nmcp.RPCError{
			Code:    -32603,
			Message: "Failed to get message context",
		})
		return
	}

	auditLog := gatewaytypes.MCPAuditLog{
		CreatedAt:                 time.Now(),
		UserID:                    m.userID,
		MCPID:                     m.mcpID,
		MCPServerDisplayName:      m.mcpServer.Spec.Manifest.Name,
		MCPServerCatalogEntryName: m.mcpServer.Spec.MCPServerCatalogEntryName,
		ClientName:                msg.Session.InitializeRequest.ClientInfo.Name,
		ClientVersion:             msg.Session.InitializeRequest.ClientInfo.Version,
		ClientIP:                  getClientIP(m.req),
		CallType:                  msg.Method,
		CallIdentifier:            extractCallIdentifier(msg),
		SessionID:                 msg.Session.ID(),
		UserAgent:                 m.req.UserAgent(),
		RequestHeaders:            captureHeaders(m.req.Header),
	}
	if msg.ID != nil {
		auditLog.RequestID = fmt.Sprintf("%v", msg.ID)
	}

	// Capture request body if available
	if msg.Params != nil {
		if requestBody, err := json.Marshal(msg.Params); err == nil {
			auditLog.RequestBody = requestBody
		}
	}

	// If an unauthorized error occurs, send the proper status code.
	var (
		err    error
		client *mcp.Client
		result any
	)
	defer func() {
		// Complete audit log
		auditLog.ProcessingTimeMs = time.Since(auditLog.CreatedAt).Milliseconds()
		auditLog.ResponseHeaders = captureHeaders(m.resp.Header())

		if err != nil {
			auditLog.Error = err.Error()
			auditLog.ResponseStatus = http.StatusInternalServerError

			var oauthErr nmcp.AuthRequiredErr
			if errors.As(err, &oauthErr) {
				auditLog.ResponseStatus = http.StatusUnauthorized
				m.resp.Header().Set(
					"WWW-Authenticate",
					fmt.Sprintf(`Bearer error="invalid_token", error_description="The access token is invalid or expired. Please re-authenticate and try again.", resource_metadata="%s/.well-known/oauth-protected-resource%s"`, h.baseURL, m.req.URL.Path),
				)
				http.Error(m.resp, fmt.Sprintf("Unauthorized: %v", oauthErr), http.StatusUnauthorized)
				h.gatewayClient.LogMCPAuditEntry(auditLog)
				return
			}

			if rpcError := (*nmcp.RPCError)(nil); errors.As(err, &rpcError) {
				msg.SendError(ctx, rpcError)
			} else {
				msg.SendError(ctx, &nmcp.RPCError{
					Code:    -32603,
					Message: fmt.Sprintf("failed to send %s message to server %s: %v", msg.Method, m.mcpServer.Name, err),
				})
			}
		} else {
			auditLog.ResponseStatus = http.StatusOK
			// Capture response body if available
			if result != nil {
				if responseBody, err := json.Marshal(result); err == nil {
					auditLog.ResponseBody = responseBody
				}
			}
		}

		h.gatewayClient.LogMCPAuditEntry(auditLog)
	}()

	catalogName := m.mcpServer.Spec.MCPCatalogID
	if catalogName == "" {
		catalogName = m.mcpServer.Spec.PowerUserWorkspaceID
	}
	if catalogName == "" && m.mcpServer.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := h.storageClient.Get(ctx, kclient.ObjectKey{Namespace: m.mcpServer.Namespace, Name: m.mcpServer.Spec.MCPServerCatalogEntryName}, &entry); err != nil {
			log.Errorf("Failed to get catalog for server %s: %v", m.mcpServer.Name, err)
			return
		}
		catalogName = entry.Spec.MCPCatalogName
	}

	var webhooks []mcp.Webhook
	webhooks, err = h.webhookHelper.GetWebhooksForMCPServer(ctx, h.gptClient, m.mcpServer.Namespace, m.mcpServer.Name, m.mcpServer.Spec.MCPServerCatalogEntryName, catalogName, auditLog.CallType, auditLog.CallIdentifier)
	if err != nil {
		log.Errorf("Failed to get webhooks for server %s: %v", m.mcpServer.Name, err)
		return
	}

	if err = fireWebhooks(ctx, webhooks, msg, &auditLog, "request", m.userID, m.mcpID); err != nil {
		log.Errorf("Failed to fire webhooks for server %s: %v", m.mcpServer.Name, err)
		auditLog.ResponseStatus = http.StatusFailedDependency
		return
	}

	client, err = h.mcpSessionManager.ClientForMCPServerWithOptions(
		ctx,
		msg.Session.ID(),
		m.mcpServer,
		m.serverConfig,
		h.asClientOption(
			msg.Session,
			m.userID,
			m.mcpID,
			m.mcpServer.Namespace,
			m.mcpServer.Name,
			m.mcpServer.Spec.Manifest.Name,
			m.mcpServer.Spec.MCPServerCatalogEntryName,
			catalogName,
		),
	)
	if err != nil {
		log.Errorf("Failed to get client for server %s: %v", m.mcpServer.Name, err)
		return
	}

	switch msg.Method {
	case "notifications/initialized":
		// This method is special because it is handled automatically by the client.
		// So, we don't forward this one, just respond with a success.
		return
	case "ping":
		result = nmcp.PingResult{}
	case "initialize":
		go func(session *nmcp.Session) {
			session.Wait()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := h.mcpSessionManager.CloseClient(ctx, m.serverConfig, session.ID()); err != nil {
				log.Errorf("Failed to shutdown server %s: %v", m.mcpServer.Name, err)
			}

			if _, _, err = h.LoadAndDelete(ctx, h, session.ID()); err != nil {
				log.Errorf("Failed to delete session %s: %v", session.ID(), err)
			}
		}(msg.Session)

		if client.Session.InitializeResult.ServerInfo != (nmcp.ServerInfo{}) ||
			client.Session.InitializeResult.Capabilities.Tools != nil ||
			client.Session.InitializeResult.Capabilities.Prompts != nil ||
			client.Session.InitializeResult.Capabilities.Resources != nil {
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
		err = &nmcp.RPCError{
			Code:    -32601,
			Message: "Method not allowed",
		}
		return
	}

	if err = client.Session.Exchange(ctx, msg.Method, &msg, &result); err != nil {
		log.Errorf("Failed to send %s message to server %s: %v", msg.Method, m.mcpServer.Name, err)
		return
	}

	b, err := json.Marshal(result)
	if err != nil {
		log.Errorf("Failed to marshal result for server %s: %v", m.mcpServer.Name, err)
		err = &nmcp.RPCError{
			Code:    -32603,
			Message: fmt.Sprintf("failed to marshal result for server %s: %v", m.mcpServer.Name, err),
		}
		return
	}

	msg.Result = b

	if err = fireWebhooks(ctx, webhooks, msg, &auditLog, "response", m.userID, m.mcpID); err != nil {
		log.Errorf("Failed to fire webhooks for server %s: %v", m.mcpServer.Name, err)
		auditLog.ResponseStatus = http.StatusFailedDependency
		return
	}

	if err = msg.Reply(ctx, msg.Result); err != nil {
		log.Errorf("Failed to reply to server %s: %v", m.mcpServer.Name, err)
		err = &nmcp.RPCError{
			Code:    -32603,
			Message: fmt.Sprintf("failed to reply to server %s: %v", m.mcpServer.Name, err),
		}
	}
}

// Helper methods for audit logging

func getClientIP(req *http.Request) string {
	// Check X-Forwarded-For header first
	if forwarded := req.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Take the first IP in the list
		if idx := strings.Index(forwarded, ","); idx != -1 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return strings.TrimSpace(forwarded)
	}

	// Check X-Real-IP header
	if realIP := req.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	// Fall back to RemoteAddr
	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		return host
	}

	return req.RemoteAddr
}

func extractCallIdentifier(msg nmcp.Message) string {
	switch msg.Method {
	case "resources/read":
		return gjson.GetBytes(msg.Params, "uri").String()
	case "tools/call", "prompts/get":
		return gjson.GetBytes(msg.Params, "name").String()
	default:
		return ""
	}
}

func captureHeaders(headers http.Header) json.RawMessage {
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

func fireWebhooks(ctx context.Context, webhooks []mcp.Webhook, msg nmcp.Message, auditLog *gatewaytypes.MCPAuditLog, webhookType, userID, mcpID string) error {
	signatures := make(map[string]string, len(webhooks))

	// Go through webhook validations.
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	auditLog.WebhookStatuses = make([]gatewaytypes.MCPWebhookStatus, 0, len(webhooks))
	var (
		webhookStatus string
		rpcError      *nmcp.RPCError
	)
	for _, webhook := range webhooks {
		webhookStatus, rpcError = fireWebhook(ctx, httpClient, body, mcpID, userID, webhook.URL, webhook.Secret, signatures)
		if rpcError != nil {
			auditLog.WebhookStatuses = append(auditLog.WebhookStatuses, gatewaytypes.MCPWebhookStatus{
				Type:    webhookType,
				URL:     webhook.URL,
				Status:  webhookStatus,
				Message: rpcError.Message,
			})
			return rpcError
		}

		auditLog.WebhookStatuses = append(auditLog.WebhookStatuses, gatewaytypes.MCPWebhookStatus{
			Type:   webhookType,
			URL:    webhook.URL,
			Status: webhookStatus,
		})
	}

	return nil
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
