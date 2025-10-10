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

	msgCtx := messageContext{
		userID:       req.User.GetUID(),
		mcpID:        mcpID,
		serverConfig: mcpServerConfig,
		mcpServer:    mcpServer,
		req:          req.Request,
		resp:         req.ResponseWriter,
	}

	// If composite server, load child servers
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		var childServerList v1.MCPServerList
		if err := req.List(&childServerList, kclient.MatchingLabels{
			"composite-parent": mcpServer.Name,
		}); err != nil {
			return fmt.Errorf("failed to list child servers: %w", err)
		}

		msgCtx.childServers = childServerList.Items
		msgCtx.childConfigs = make([]mcp.ServerConfig, len(childServerList.Items))

		// Get server configs for each child
		for i, childServer := range childServerList.Items {
			childConfig, err := handlers.ServerConfigForAction(req, childServer)
			if err != nil {
				return fmt.Errorf("failed to get config for child server %s: %w", childServer.Name, err)
			}
			msgCtx.childConfigs[i] = childConfig
		}
	}

	req.Request = req.WithContext(withMessageContext(req.Context(), msgCtx))

	nmcp.NewHTTPServer(nil, h, nmcp.HTTPServerOptions{SessionStore: h}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

type messageContext struct {
	userID, mcpID string
	mcpServer     v1.MCPServer
	serverConfig  mcp.ServerConfig
	req           *http.Request
	resp          http.ResponseWriter
	// For composite servers (when serverConfig.Runtime == types.RuntimeComposite)
	childServers []v1.MCPServer
	childConfigs []mcp.ServerConfig
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

	// Handle composite servers - connect to all children and aggregate
	log.Warnf("OnMessage for server %s, method %s, runtime: %s, hasChildServers: %v", m.mcpServer.Name, msg.Method, m.serverConfig.Runtime, len(m.childServers) > 0)
	if m.serverConfig.Runtime == types.RuntimeComposite {
		log.Warnf("Handling composite server %s for method %s", m.mcpServer.Name, msg.Method)
		switch msg.Method {
		case methodNotificationsInitialized:
			return
		case methodPing:
			result = nmcp.PingResult{}
		case methodInitialize:
			// Aggregate capabilities from all children
			aggregatedResult := nmcp.InitializeResult{
				ProtocolVersion: "2024-11-05",
				ServerInfo: nmcp.ServerInfo{
					Name:    m.mcpServer.Spec.Manifest.Name,
					Version: "1.0.0",
				},
				Capabilities: nmcp.ServerCapabilities{},
			}

			// Connect to each child and get their capabilities
			for i, childServer := range m.childServers {
				childClient, childErr := h.mcpSessionManager.ClientForMCPServerWithOptions(
					ctx,
					msg.Session.ID()+"-child-"+childServer.Name,
					childServer,
					m.childConfigs[i],
					h.asClientOption(
						msg.Session,
						m.userID,
						childServer.Name,
						childServer.Namespace,
						childServer.Name,
						childServer.Spec.Manifest.Name,
						childServer.Spec.MCPServerCatalogEntryName,
						catalogName,
						powerUserWorkspaceID,
					),
				)
				if childErr != nil {
					log.Errorf("Failed to get client for child server %s: %v", childServer.Name, childErr)
					continue
				}

				// Merge capabilities (OR logic - if any child has a capability, the composite has it)
				if childClient.Session.InitializeResult.Capabilities.Tools != nil {
					aggregatedResult.Capabilities.Tools = childClient.Session.InitializeResult.Capabilities.Tools
				}
				if childClient.Session.InitializeResult.Capabilities.Resources != nil {
					aggregatedResult.Capabilities.Resources = childClient.Session.InitializeResult.Capabilities.Resources
				}
				if childClient.Session.InitializeResult.Capabilities.Prompts != nil {
					aggregatedResult.Capabilities.Prompts = childClient.Session.InitializeResult.Capabilities.Prompts
				}
			}

			if err = msg.Reply(ctx, aggregatedResult); err != nil {
				log.Errorf("Failed to reply: %v", err)
			}
			return
		case methodToolsList:
			// Aggregate tools from all children
			var allTools []nmcp.Tool
			// Build per-component override maps: entryName -> (toolName -> override)
			componentToolMaps := map[string]map[string]types.ToolOverride{}
			hasAnyMappings := false
			if cfg := m.mcpServer.Spec.Manifest.CompositeConfig; cfg != nil {
				for _, comp := range cfg.Components {
					if len(comp.ToolOverrides) == 0 {
						continue
					}
					hasAnyMappings = true
					mp := make(map[string]types.ToolOverride, len(comp.ToolOverrides))
					for _, tm := range comp.ToolOverrides {
						mp[tm.Name] = tm
					}
					componentToolMaps[comp.CatalogEntryName] = mp
				}
			}
			for i, childServer := range m.childServers {
				childClient, childErr := h.mcpSessionManager.ClientForMCPServerWithOptions(
					ctx,
					msg.Session.ID()+"-child-"+childServer.Name,
					childServer,
					m.childConfigs[i],
					h.asClientOption(
						msg.Session,
						m.userID,
						childServer.Name,
						childServer.Namespace,
						childServer.Name,
						childServer.Spec.Manifest.Name,
						childServer.Spec.MCPServerCatalogEntryName,
						catalogName,
						powerUserWorkspaceID,
					),
				)
				if childErr != nil {
					log.Errorf("Failed to get client for child server %s: %v", childServer.Name, childErr)
					continue
				}

				var listResult nmcp.ListToolsResult
				if err := childClient.Session.Exchange(ctx, methodToolsList, &msg, &listResult); err != nil {
					log.Errorf("Failed to list tools for child %s: %v", childServer.Name, err)
					continue
				}

				// Apply mapping (if provided) or default prefixing
				// Always use sanitized manifest name as the component prefix
				componentPrefix := sanitizeToolPrefix(childServer.Spec.Manifest.Name)
				entryKey := childServer.Spec.MCPServerCatalogEntryName
				log.Warnf("Processing tools from child server %s with prefix: %s, entryKey: %s", childServer.Spec.Manifest.Name, componentPrefix, entryKey)
				for _, tool := range listResult.Tools {
					if tmap, ok := componentToolMaps[entryKey]; ok {
						if tm, ok := tmap[tool.Name]; ok {
							log.Warnf("Found override for tool %s: enabled=%v", tool.Name, tm.Enabled)
							if !tm.Enabled {
								log.Warnf("Skipping disabled tool: %s", tool.Name)
								continue
							}
							name := tm.OverrideName
							if name == "" {
								name = tool.Name
							}
							tool.Name = buildCompositedToolName(componentPrefix, sanitizeToolPrefix(name))
							if tm.OverrideDescription != "" {
								tool.Description = tm.OverrideDescription
							}
							if len(tm.ParameterOverrides) > 0 && len(tool.InputSchema) > 0 {
								tool.InputSchema = applyParameterMappings(tool.InputSchema, tm.ParameterOverrides, false)
							}
							allTools = append(allTools, tool)
							continue
						}
					}
					if !hasAnyMappings {
						log.Warnf("No mappings configured, including tool %s with prefix", tool.Name)
						tool.Name = buildCompositedToolName(componentPrefix, sanitizeToolPrefix(tool.Name))
						allTools = append(allTools, tool)
					}
				}
			}

			result = nmcp.ListToolsResult{Tools: allTools}
		case methodToolsCall:
			// Route tool call using mapping (or legacy prefix fallback)
			var params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				err = &nmcp.RPCError{Code: -32602, Message: "Invalid params"}
				return
			}

			// Find which child this tool belongs to
			var (
				targetChild      *v1.MCPServer
				targetConfig     *mcp.ServerConfig
				originalToolName string
				toolMapping      *types.ToolOverride
			)

			// Build list of component prefixes for parsing
			componentPrefixes := make([]string, len(m.childServers))
			for i, childServer := range m.childServers {
				componentPrefixes[i] = sanitizeToolPrefix(childServer.Spec.Manifest.Name)
			}

			// Parse the composited tool name to get component prefix and exposed name
			componentPrefix, exposedName, found := parseCompositedToolName(params.Name, componentPrefixes)
			if !found {
				err = &nmcp.RPCError{Code: -32602, Message: "Tool not found"}
				return
			}

			// Find the child server by matching component prefix
			var hasAnyMappings bool
			var tmap map[string]types.ToolOverride
			if cfg := m.mcpServer.Spec.Manifest.CompositeConfig; cfg != nil {
				for _, comp := range cfg.Components {
					if comp.CatalogEntryName == m.mcpServer.Spec.MCPServerCatalogEntryName && len(comp.ToolOverrides) > 0 {
						m := make(map[string]types.ToolOverride, len(comp.ToolOverrides))
						for _, tm := range comp.ToolOverrides {
							m[tm.Name] = tm
						}
						tmap = m
						hasAnyMappings = true
						break
					}
				}
			}

			for i, childServer := range m.childServers {
				childPrefix := sanitizeToolPrefix(childServer.Spec.Manifest.Name)
				if childPrefix != componentPrefix {
					continue
				}

				// Found the matching child server
				targetChild = &m.childServers[i]
				targetConfig = &m.childConfigs[i]

				// Now determine the original tool name
				if hasAnyMappings {
					for _, tm := range tmap {
						exposed := tm.OverrideName
						if exposed == "" {
							exposed = tm.Name
						}
						if sanitizeToolPrefix(exposed) == exposedName {
							if !tm.Enabled {
								err = &nmcp.RPCError{Code: -32602, Message: "Tool not found"}
								return
							}
							originalToolName = tm.Name
							tmCopy := tm
							toolMapping = &tmCopy
							break
						}
					}
					if originalToolName == "" {
						err = &nmcp.RPCError{Code: -32602, Message: "Tool not found"}
						return
					}
				} else {
					originalToolName = exposedName
				}
				break
			}

			if targetChild == nil {
				err = &nmcp.RPCError{Code: -32602, Message: "Tool not found"}
				return
			}

			// Apply parameter name mappings to arguments (exposed → component)
			if toolMapping != nil && len(toolMapping.ParameterOverrides) > 0 {
				params.Arguments = transformArguments(params.Arguments, toolMapping.ParameterOverrides, true)
			}

			// Connect to target child and call the tool
			childClient, err := h.mcpSessionManager.ClientForMCPServerWithOptions(
				ctx,
				msg.Session.ID()+"-child-"+targetChild.Name,
				*targetChild,
				*targetConfig,
				h.asClientOption(
					msg.Session,
					m.userID,
					targetChild.Name,
					targetChild.Namespace,
					targetChild.Name,
					targetChild.Spec.Manifest.Name,
					targetChild.Spec.MCPServerCatalogEntryName,
					catalogName,
					powerUserWorkspaceID,
				),
			)
			if err != nil {
				log.Errorf("Failed to get client for child server %s: %v", targetChild.Name, err)
				return
			}

			// Update params with original tool name
			params.Name = originalToolName
			modifiedParams, _ := json.Marshal(params)
			msg.Params = modifiedParams

			result = nmcp.CallToolResult{}
			if err = childClient.Session.Exchange(ctx, methodToolsCall, &msg, &result); err != nil {
				log.Errorf("Failed to call tool on child %s: %v", targetChild.Name, err)
				return
			}
		default:
			// For other methods, return empty results for now
			result = nmcp.Notification{}
		}

		// Send the aggregated/routed result
		if err = msg.Reply(ctx, result); err != nil {
			log.Errorf("Failed to reply: %v", err)
		}
		return
	}

	// Non-composite path (original logic)
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

// sanitizeToolPrefix converts a server manifest name to a safe tool prefix
// e.g., "Component 1" -> "component_1"
func sanitizeToolPrefix(name string) string {
	// Convert to lowercase
	prefix := strings.ToLower(name)
	// Replace spaces and special characters with underscores
	prefix = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, prefix)
	// Collapse multiple underscores into one
	for strings.Contains(prefix, "__") {
		prefix = strings.ReplaceAll(prefix, "__", "_")
	}
	// Trim leading/trailing underscores
	prefix = strings.Trim(prefix, "_")
	return prefix
}

// buildCompositedToolName creates the final tool name with component prefix
// e.g., prefix="component_1", exposedName="add_stuff" -> "component_1_add_stuff"
func buildCompositedToolName(componentPrefix, exposedName string) string {
	return componentPrefix + "_" + exposedName
}

// parseCompositedToolName extracts the component prefix and exposed tool name
// e.g., "component_1_add_stuff" -> ("component_1", "add_stuff")
func parseCompositedToolName(compositeName string, componentPrefixes []string) (componentPrefix, exposedName string, found bool) {
	for _, prefix := range componentPrefixes {
		prefixWithSep := prefix + "_"
		if strings.HasPrefix(compositeName, prefixWithSep) {
			return prefix, strings.TrimPrefix(compositeName, prefixWithSep), true
		}
	}
	return "", "", false
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

// transformArguments renames argument keys based on parameter mappings
// If toComponent is true: exposed → component (for incoming tool calls)
// If toComponent is false: component → exposed (for outgoing tool call results, if needed)
func transformArguments(args map[string]interface{}, mappings []types.ParameterOverride, toComponent bool) map[string]interface{} {
	if len(mappings) == 0 {
		return args
	}

	// Build mapping lookup
	paramMap := make(map[string]string)
	for _, pm := range mappings {
		if toComponent {
			// Map override parameter name to original parameter name
			paramMap[pm.OverrideName] = pm.Name
		} else {
			// Map original parameter name to override parameter name
			paramMap[pm.Name] = pm.OverrideName
		}
	}

	// Transform argument keys
	result := make(map[string]interface{}, len(args))
	for key, value := range args {
		if newKey, found := paramMap[key]; found {
			result[newKey] = value
		} else {
			// Keep unmapped arguments as-is
			result[key] = value
		}
	}
	return result
}

// applyParameterMappings transforms parameter names and descriptions in a JSON Schema InputSchema
// If reverse is false: component → exposed (for tools/list - showing schema to clients)
// If reverse is true: exposed → component (for future use transforming incoming schemas)
func applyParameterMappings(inputSchema json.RawMessage, mappings []types.ParameterOverride, reverse bool) json.RawMessage {
	if len(mappings) == 0 || len(inputSchema) == 0 {
		return inputSchema
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(inputSchema, &schema); err != nil {
		log.Errorf("Failed to unmarshal input schema for parameter mapping: %v", err)
		return inputSchema
	}

	// Build mapping lookup
	paramMap := make(map[string]types.ParameterOverride)
	for _, pm := range mappings {
		if reverse {
			// For override → original: key by override name
			paramMap[pm.OverrideName] = pm
		} else {
			// For original → override: key by original name
			paramMap[pm.Name] = pm
		}
	}

	// Transform properties in the schema
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		newProperties := make(map[string]interface{})
		for propName, propDef := range properties {
			if pm, found := paramMap[propName]; found {
				// Rename the property
				var newName string
				if reverse {
					// override → original
					newName = pm.Name
				} else {
					// original → override
					newName = pm.OverrideName
				}

				// Update description if mapping provides one (only for component → exposed)
				if !reverse && pm.OverrideDescription != "" {
					if propMap, ok := propDef.(map[string]interface{}); ok {
						propMap["description"] = pm.OverrideDescription
					}
				}

				newProperties[newName] = propDef
			} else {
				// Keep unmapped properties as-is
				newProperties[propName] = propDef
			}
		}
		schema["properties"] = newProperties
	}

	// Update required array if present
	if required, ok := schema["required"].([]interface{}); ok {
		newRequired := make([]interface{}, 0, len(required))
		for _, req := range required {
			if reqStr, ok := req.(string); ok {
				if pm, found := paramMap[reqStr]; found {
					if reverse {
						// override → original
						newRequired = append(newRequired, pm.Name)
					} else {
						// original → override
						newRequired = append(newRequired, pm.OverrideName)
					}
				} else {
					newRequired = append(newRequired, req)
				}
			} else {
				newRequired = append(newRequired, req)
			}
		}
		schema["required"] = newRequired
	}

	result, err := json.Marshal(schema)
	if err != nil {
		log.Errorf("Failed to marshal transformed schema: %v", err)
		return inputSchema
	}
	return result
}
