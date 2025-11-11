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
	"github.com/obot-platform/obot/apiclient/types"
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

// MCP Method Constants
const (
	methodPing                          = "ping"
	methodInitialize                    = "initialize"
	methodResourcesRead                 = "resources/read"
	methodResourcesList                 = "resources/list"
	methodResourcesTemplatesList        = "resources/templates/list"
	methodPromptsList                   = "prompts/list"
	methodPromptsGet                    = "prompts/get"
	methodToolsList                     = "tools/list"
	methodToolsCall                     = "tools/call"
	methodNotificationsInitialized      = "notifications/initialized"
	methodNotificationsProgress         = "notifications/progress"
	methodNotificationsRootsListChanged = "notifications/roots/list_changed"
	methodNotificationsCancelled        = "notifications/cancelled"
	methodLoggingSetLevel               = "logging/setLevel"
	methodSampling                      = "sampling/createMessage"
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

	mcpID, mcpServer, mcpServerConfig, err := handlers.ServerForActionWithConnectID(
		req,
		req.PathValue("mcp_id"),
		h.mcpSessionManager.TokenService(),
		h.baseURL,
	)
	if err == nil && mcpServer.Spec.Template {
		// Prevent connections to MCP server templates by returning a 404.
		err = apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpserver"}, mcpID)
	}

	ss := newSessionStore(h, mcpID, req.User.GetUID())

	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the MCP server is not found, remove the session.
			if sessionID != "" {
				session, found, err := ss.LoadAndDelete(req.Context(), h, sessionID)
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

	messageCtx := messageContext{
		userID:       req.User.GetUID(),
		mcpID:        mcpID,
		mcpServer:    mcpServer,
		serverConfig: mcpServerConfig,
		req:          req.Request,
		resp:         req.ResponseWriter,
	}
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		// List all component servers for the composite server.
		var componentServerList v1.MCPServerList
		if err := req.List(&componentServerList,
			kclient.InNamespace(mcpServer.Namespace),
			kclient.MatchingFields{
				"spec.compositeName": mcpServer.Name,
			}); err != nil {
			return fmt.Errorf("failed to list component servers for composite server %s: %v", mcpServer.Name, err)
		}

		var componentInstanceList v1.MCPServerInstanceList
		if err := req.List(&componentInstanceList,
			kclient.InNamespace(mcpServer.Namespace),
			kclient.MatchingFields{
				"spec.compositeName": mcpServer.Name,
			}); err != nil {
			return fmt.Errorf("failed to list component instances for composite server %s: %v", mcpServer.Name, err)
		}

		// Precompute disabled component IDs for quick lookup
		var compositeConfig types.CompositeRuntimeConfig
		if mcpServer.Spec.Manifest.CompositeConfig != nil {
			compositeConfig = *mcpServer.Spec.Manifest.CompositeConfig
		}

		disabledComponents := make(map[string]bool, len(compositeConfig.ComponentServers))
		for _, component := range compositeConfig.ComponentServers {
			if id := component.ComponentID(); id != "" {
				disabledComponents[id] = component.Disabled
			}
		}

		componentServers := make([]messageContext, 0, len(componentServerList.Items)+len(componentInstanceList.Items))

		// Add single-user component servers
		for _, componentServer := range componentServerList.Items {
			// Skip if explicitly disabled in composite config
			if disabled, ok := disabledComponents[componentServer.Spec.MCPServerCatalogEntryName]; !ok || disabled {
				log.Debugf("Skipping component server %s not enabled in composite config", componentServer.Name)
				continue
			}

			// Resolve server and config using the higher-level API
			srv, config, err := handlers.ServerForAction(req, componentServer.Name, h.mcpSessionManager.TokenService(), h.baseURL)
			if err != nil {
				// If the component isn't configured or can't be reached, skip it.
				log.Warnf("Failed to get component server %s: %v", componentServer.Name, err)
				continue
			}

			componentServers = append(componentServers, messageContext{
				userID:       req.User.GetUID(),
				mcpID:        srv.Name,
				mcpServer:    srv,
				serverConfig: config,
			})
		}

		// Add multi-user component instances
		for _, componentInstance := range componentInstanceList.Items {
			var multiUserServer v1.MCPServer
			if err := req.Get(&multiUserServer, componentInstance.Spec.MCPServerName); err != nil {
				log.Warnf("Failed to get multi-user server %s for instance %s: %v", componentInstance.Spec.MCPServerName, componentInstance.Name, err)
				continue
			}

			if disabled, ok := disabledComponents[multiUserServer.Name]; !ok || disabled {
				log.Debugf("Skipping component instance %s not enabled in composite config", componentInstance.Name)
				continue
			}

			srv, config, err := handlers.ServerForAction(req, multiUserServer.Name, h.mcpSessionManager.TokenService(), h.baseURL)
			if err != nil {
				log.Warnf("Failed to get multi-user server %s: %v", multiUserServer.Name, err)
				continue
			}

			componentServers = append(componentServers, messageContext{
				userID:       req.User.GetUID(),
				mcpID:        srv.Name,
				mcpServer:    srv,
				serverConfig: config,
			})
		}

		// No composite servers is a valid configuration that returns no capabilities at all.
		messageCtx.compositeContext = newCompositeContext(mcpServer.Spec.Manifest.CompositeConfig, componentServers)
	}

	req.Request = req.WithContext(withMessageContext(req.Context(), messageCtx))

	nmcp.NewHTTPServer(nil, h, nmcp.HTTPServerOptions{SessionStore: ss}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

type messageContext struct {
	compositeContext
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

	if m.mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		h.onCompositeMessage(ctx, msg, m)
		return
	}

	h.onMessage(ctx, msg, m)
}

func (h *Handler) onMessage(ctx context.Context, msg nmcp.Message, m messageContext) {
	// Determine PowerUserWorkspaceID: use server's workspace ID for multi-user servers,
	// or look up catalog entry's workspace ID for single-user servers
	powerUserWorkspaceID := m.mcpServer.Spec.PowerUserWorkspaceID
	if powerUserWorkspaceID == "" && m.mcpServer.Spec.MCPServerCatalogEntryName != "" {
		// This is a single-user server created from a catalog entry, look up the entry
		var entry v1.MCPServerCatalogEntry
		if err := h.storageClient.Get(ctx, kclient.ObjectKey{Namespace: m.mcpServer.Namespace, Name: m.mcpServer.Spec.MCPServerCatalogEntryName}, &entry); err == nil {
			powerUserWorkspaceID = entry.Spec.PowerUserWorkspaceID
		}
	}

	auditLog := gatewaytypes.MCPAuditLog{
		CreatedAt:                 time.Now(),
		UserID:                    m.userID,
		MCPID:                     m.mcpID,
		PowerUserWorkspaceID:      powerUserWorkspaceID,
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
			if auditLog.ResponseStatus < http.StatusBadRequest {
				auditLog.ResponseStatus = http.StatusInternalServerError
			}

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
		m.userID,
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
			powerUserWorkspaceID,
		),
	)
	if err != nil {
		log.Errorf("Failed to get client for server %s: %v", m.mcpServer.Name, err)
		return
	}

	switch msg.Method {
	case methodNotificationsInitialized:
		// This method is special because it is handled automatically by the client.
		// So, we don't forward this one, just respond with a success.
		return
	case methodPing:
		result = nmcp.PingResult{}
	case methodInitialize:
		go func(session *nmcp.Session) {
			session.Wait()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := h.mcpSessionManager.CloseClient(ctx, m.serverConfig, session.ID()); err != nil {
				log.Errorf("Failed to shutdown server %s: %v", m.mcpServer.Name, err)
			}

			if _, _, err = newSessionStore(h, m.mcpID, m.userID).LoadAndDelete(ctx, h, session.ID()); err != nil {
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
	case methodResourcesRead:
		result = nmcp.ReadResourceResult{}
	case methodResourcesList:
		result = nmcp.ListResourcesResult{}
	case methodResourcesTemplatesList:
		result = nmcp.ListResourceTemplatesResult{}
	case methodPromptsList:
		result = nmcp.ListPromptsResult{}
	case methodPromptsGet:
		result = nmcp.GetPromptResult{}
	case methodToolsList:
		result = nmcp.ListToolsResult{}
	case methodToolsCall:
		result = nmcp.CallToolResult{}
	case methodNotificationsProgress, methodNotificationsRootsListChanged, methodNotificationsCancelled, methodLoggingSetLevel:
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

	// Send forward the message to the server and wait for the result
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
	case methodResourcesRead:
		return gjson.GetBytes(msg.Params, "uri").String()
	case methodToolsCall, methodPromptsGet:
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

// Webhook helpers

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
	var rpcErrors []*nmcp.RPCError
	for _, webhook := range webhooks {
		webhookStatus, rpcError := fireWebhook(ctx, httpClient, body, mcpID, userID, webhook.URL, webhook.Secret, signatures)
		if rpcError != nil {
			auditLog.WebhookStatuses = append(auditLog.WebhookStatuses, gatewaytypes.MCPWebhookStatus{
				Type:    webhookType,
				URL:     webhook.URL,
				Status:  webhookStatus,
				Message: rpcError.Message,
			})
			rpcErrors = append(rpcErrors, rpcError)
		} else {
			auditLog.WebhookStatuses = append(auditLog.WebhookStatuses, gatewaytypes.MCPWebhookStatus{
				Type:   webhookType,
				URL:    webhook.URL,
				Status: webhookStatus,
			})
		}
	}

	switch len(rpcErrors) {
	case 0:
		return nil
	case 1:
		return rpcErrors[0]
	default:
		var message strings.Builder
		message.WriteString("failed to fire webhooks: ")
		for _, err := range rpcErrors {
			message.WriteString(err.Message)
			message.WriteString("; ")
		}
		return &nmcp.RPCError{
			Code:    -32603,
			Message: message.String()[:message.Len()-2],
		}
	}
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
			Code:    -32603,
			Message: fmt.Sprintf("webhook %s returned status code %d: %v", url, resp.StatusCode, string(respBody)),
		}
	}

	return resp.Status, nil
}

// Pending request helpers
func (h *Handler) pendingRequestsForSession(sessionID string) *nmcp.PendingRequests {
	obj, _ := h.pendingRequests.LoadOrStore(sessionID, &nmcp.PendingRequests{})
	return obj.(*nmcp.PendingRequests)
}
