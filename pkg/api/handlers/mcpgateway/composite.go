package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	otypes "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *Handler) onCompositeMessage(ctx context.Context, msg nmcp.Message, m messageContext) {
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

	type componentClient struct {
		serverContext
		*mcp.Client
	}
	var (
		err    error
		result any

		// Map of component server prefix to client
		clients = make(map[string]componentClient, len(m.componentServers))
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
				wwwAuthenticateHeader := fmt.Sprintf(`Bearer error="invalid_token", error_description="The access token is invalid or expired. Please re-authenticate and try again.", resource_metadata="%s/.well-known/oauth-protected-resource%s"`, h.baseURL, m.req.URL.Path)
				log.Errorf("OAuth required for composite server %s: %v, wwwAuthenticateHeader: %s", m.mcpServer.Name, oauthErr, wwwAuthenticateHeader)
				auditLog.ResponseStatus = http.StatusUnauthorized
				m.resp.Header().Set(
					"WWW-Authenticate",
					wwwAuthenticateHeader,
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

	for _, componentServer := range m.componentServers {
		var (
			client       *mcp.Client
			componentKey = normalizeName(componentServer.mcpServer.Spec.Manifest.Name)
		)
		client, err = h.mcpSessionManager.ClientForMCPServerWithOptions(
			ctx,
			componentServer.userID,
			msg.Session.ID(),
			componentServer.mcpServer,
			componentServer.serverConfig,
			h.asClientOption(
				msg.Session,
				componentServer.userID,
				componentServer.mcpID,
				componentServer.mcpServer.Namespace,
				componentServer.mcpServer.Name,
				componentServer.mcpServer.Spec.Manifest.Name,
				componentServer.mcpServer.Spec.MCPServerCatalogEntryName,
				catalogName,
				powerUserWorkspaceID,
			),
		)
		if err != nil {
			log.Errorf("Failed to get client for server %s: %v", componentServer.mcpServer.Name, err)
			if componentServer.enabled {
				// Enabled components must initialize successfully
				return
			}
			// Disabled components can fail silently
			continue
		}

		clients[componentKey] = componentClient{
			serverContext: componentServer,
			Client:        client,
		}
	}

	if len(clients) < 1 {
		err = fmt.Errorf("no running component servers found for composite server %s", m.mcpID)
		return
	}

	switch msg.Method {
	case methodNotificationsInitialized:
		// This method is special because it is handled automatically by the client.
		// So, we don't forward this one, just respond with a success.
		return
	case methodPing:
		result = nmcp.PingResult{}
		for _, client := range clients {
			if err = client.Session.Exchange(ctx, msg.Method, &msg, &result); err != nil {
				log.Errorf("Failed to send %s message to server %s: %v", msg.Method, client.mcpID, err)
				err = &nmcp.RPCError{
					Code:    -32603,
					Message: fmt.Sprintf("failed to send %s message to server %s: %v", msg.Method, client.mcpID, err),
				}
				return
			}

		}

		// All components responded, reply with the last result
		if err = msg.Reply(ctx, result); err != nil {
			log.Errorf("Failed to reply to composite server %s: %v", m.mcpID, err)
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to reply to composite server %s: %v", m.mcpID, err),
			}
		}

		return
	case methodInitialize:
		go func(session *nmcp.Session) {
			session.Wait()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			for _, client := range clients {
				if err := h.mcpSessionManager.CloseClient(ctx, client.serverConfig, session.ID()); err != nil {
					log.Errorf("Failed to shutdown server %s: %v", client.mcpServer.Name, err)
				}
			}

			if _, _, err = h.LoadAndDelete(ctx, h, session.ID()); err != nil {
				log.Errorf("Failed to delete session %s: %v", session.ID(), err)
			}
		}(msg.Session)

		// Initialize with each component and merge the results into a composite initialize result
		compositeResult := nmcp.InitializeResult{
			ProtocolVersion: msg.Session.InitializeRequest.ProtocolVersion,
			ServerInfo: nmcp.ServerInfo{
				Name:    m.mcpServer.Spec.Manifest.Name,
				Version: hash.Digest(m.mcpServer.Spec.Manifest)[:7],
			},
		}
		for _, client := range clients {
			componentResult := client.Session.InitializeResult
			if componentResult.ServerInfo != (nmcp.ServerInfo{}) ||
				componentResult.Capabilities.Tools != nil ||
				componentResult.Capabilities.Prompts != nil {
				compositeResult = mergeInitializeResults(compositeResult, componentResult)
				continue
			}

			// Send the message to the server, wait for the result, and merge it into the composite
			if err = client.Session.Exchange(ctx, methodInitialize, &msg, &componentResult); err != nil {
				log.Errorf("Failed to send %s message to server %s: %v", msg.Method, client.mcpID, err)
				return
			}

			compositeResult = mergeInitializeResults(compositeResult, componentResult)
		}

		if err = msg.Reply(ctx, compositeResult); err != nil {
			log.Errorf("Failed to reply to composite server %s: %v", m.mcpID, err)
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to reply to composite server %s: %v", m.mcpID, err),
			}
		}

		result = compositeResult
		return

	case methodPromptsList:
		var compositePrompts []nmcp.Prompt
		for componentKey, client := range clients {
			var result nmcp.ListPromptsResult
			if err = client.Session.Exchange(ctx, methodPromptsList, &msg, &result); err != nil {
				log.Errorf("Failed to send %s message to server %s: %v", msg.Method, client.mcpID, err)
				return
			}
			for _, prompt := range result.Prompts {
				compositePrompts = append(compositePrompts, m.toCompositePrompt(componentKey, prompt))
			}
		}

		compositeResult := nmcp.ListPromptsResult{
			Prompts: compositePrompts,
		}
		if err = msg.Reply(ctx, compositeResult); err != nil {
			log.Errorf("Failed to reply to composite server %s: %v", m.mcpID, err)
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to reply to composite server %s: %v", m.mcpID, err),
			}
		}

		result = compositeResult
		return
	case methodPromptsGet:
		var compositeRequest nmcp.GetPromptRequest
		if err := json.Unmarshal(msg.Params, &compositeRequest); err != nil {
			err = &nmcp.RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Failed to unmarshal get prompt request: %v", err),
			}
			return
		}

		componentKey, _, ok := strings.Cut(compositeRequest.Name, "_")
		if !ok {
			err = &nmcp.RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown prompt: %s", compositeRequest.Name),
			}
			return
		}

		client, ok := clients[componentKey]
		if !ok {
			err = &nmcp.RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown prompt: %s", compositeRequest.Name),
			}
			return
		}

		componentRequest := m.toComponentGetPromptRequest(componentKey, compositeRequest)

		b, err := json.Marshal(componentRequest)
		if err != nil {
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to marshal request for server %s: %v", client.mcpServer.Name, err),
			}
			return
		}

		msg.Params = b
		var componentResult nmcp.GetPromptResult
		if err = client.Session.Exchange(ctx, methodPromptsGet, &msg, &componentResult); err != nil {
			log.Errorf("Failed to send %s message to server %s: %v", msg.Method, client.mcpID, err)
			return
		}

		if err = msg.Reply(ctx, componentResult); err != nil {
			log.Errorf("Failed to reply to composite server %s: %v", m.mcpID, err)
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to reply to composite server %s: %v", m.mcpID, err),
			}
		}

		result = componentResult
		return
	case methodToolsList:
		var compositeTools []nmcp.Tool
		for componentKey, client := range clients {
			var result nmcp.ListToolsResult
			if err = client.Session.Exchange(ctx, methodToolsList, &msg, &result); err != nil {
				log.Errorf("Failed to send %s message to server %s: %v", msg.Method, client.mcpID, err)
				return
			}
			for _, tool := range result.Tools {
				compositeTool, err := m.toCompositeTool(componentKey, tool)
				if err != nil {
					err = fmt.Errorf("failed to override tool %s: %w", tool.Name, err)
					return
				}

				if compositeTool == nil {
					// Tool is disabled by the override, skip it
					continue
				}

				// Prefix the tool name with the component key
				// This lets us lookup the target component server for tool calls
				compositeTools = append(compositeTools, *compositeTool)
			}
		}

		compositeResult := nmcp.ListToolsResult{
			Tools: compositeTools,
		}
		if err = msg.Reply(ctx, compositeResult); err != nil {
			log.Errorf("Failed to reply to composite server %s: %v", m.mcpID, err)
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to reply to composite server %s: %v", m.mcpID, err),
			}
		}

		result = compositeResult
		return
	case methodToolsCall:
		var compositeRequest nmcp.CallToolRequest
		if err := json.Unmarshal(msg.Params, &compositeRequest); err != nil {
			err = &nmcp.RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Failed to unmarshal tool call request: %v", err),
			}
			return
		}

		componentKey, _, ok := strings.Cut(compositeRequest.Name, "_")
		if !ok {
			err = &nmcp.RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown tool: %s", compositeRequest.Name),
			}
			return
		}

		client, ok := clients[componentKey]
		if !ok {
			err = &nmcp.RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown tool: %s", compositeRequest.Name),
			}
			return
		}

		componentRequest, err := m.toComponentCallToolRequest(componentKey, compositeRequest)
		if err != nil {
			log.Errorf("Failed to convert tool call request to component tool call request: %v", err)
			return
		}
		if componentRequest == nil {
			err = &nmcp.RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown tool: %s", compositeRequest.Name),
			}
		}

		b, err := json.Marshal(componentRequest)
		if err != nil {
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to marshal request for server %s: %v", client.mcpServer.Name, err),
			}
			return
		}

		msg.Params = b
		var componentResult nmcp.CallToolResult
		if err = client.Session.Exchange(ctx, methodToolsCall, &msg, &componentResult); err != nil {
			log.Errorf("Failed to send %s message to server %s: %v", msg.Method, client.mcpID, err)
			return
		}

		if err = msg.Reply(ctx, componentResult); err != nil {
			log.Errorf("Failed to reply to composite server %s: %v", m.mcpID, err)
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to reply to composite server %s: %v", m.mcpID, err),
			}
		}

		result = componentResult
		return

	case methodNotificationsProgress, methodNotificationsRootsListChanged, methodNotificationsCancelled, methodLoggingSetLevel:
		// These methods don't require a result.
		result = nmcp.Notification{}
		for _, client := range clients {
			if err = client.Session.Exchange(ctx, msg.Method, &msg, &result); err != nil {
				log.Errorf("Failed to send %s message to server %s: %v", msg.Method, client.mcpID, err)
				return
			}
		}

		if err = msg.Reply(ctx, result); err != nil {
			log.Errorf("Failed to reply to composite server %s: %v", m.mcpID, err)
			err = &nmcp.RPCError{
				Code:    -32603,
				Message: fmt.Sprintf("failed to reply to composite server %s: %v", m.mcpID, err),
			}
		}
		return
	default:
		log.Errorf("Unknown method for server message: %s", msg.Method)
		err = &nmcp.RPCError{
			Code:    -32601,
			Message: "Method not allowed",
		}
	}

}

func normalizeName(name string) string {
	// Convert to lowercase
	key := strings.ToLower(name)
	// Convert underscores to hyphens first
	key = strings.ReplaceAll(key, "_", "-")
	// Replace any non [a-z0-9] rune with hyphen
	key = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, key)
	// Collapse multiple hyphens into one
	for strings.Contains(key, "--") {
		key = strings.ReplaceAll(key, "--", "-")
	}
	// Trim leading/trailing hyphens
	key = strings.Trim(key, "-")
	return key
}

func mergeInitializeResults(composite nmcp.InitializeResult, component nmcp.InitializeResult) nmcp.InitializeResult {
	// Merge Tools capability
	if compositeTools, componentTools := composite.Capabilities.Tools, component.Capabilities.Tools; componentTools != nil && (compositeTools == nil || !compositeTools.ListChanged) {
		composite.Capabilities.Tools = &nmcp.ToolsServerCapability{
			ListChanged: componentTools.ListChanged,
		}
	}

	// Merge Prompts capability
	if compositePrompts, componentPrompts := composite.Capabilities.Prompts, component.Capabilities.Prompts; componentPrompts != nil && (compositePrompts == nil || !compositePrompts.ListChanged) {
		composite.Capabilities.Prompts = &nmcp.PromptsServerCapability{
			ListChanged: componentPrompts.ListChanged,
		}
	}

	return composite
}

type compositeContext struct {
	componentServers []serverContext
	toolOverrides    map[string]otypes.ToolOverride
}

func newCompositeContext(config *otypes.CompositeRuntimeConfig, componentServers []serverContext) compositeContext {
	compositeContext := compositeContext{
		componentServers: componentServers,
		toolOverrides:    make(map[string]otypes.ToolOverride),
	}
	if config == nil {
		return compositeContext
	}

	for _, component := range config.ComponentServers {
		componentKey := normalizeName(component.Manifest.Name)
		for _, toolOverride := range component.ToolOverrides {
			// Map componentKey/toolKey -> ToolOverride
			toolKey := path.Join(componentKey, toolOverride.Name)
			compositeContext.toolOverrides[toolKey] = toolOverride

			var toolOverrideKey string
			if overrideName := toolOverride.OverrideName; overrideName != "" {
				toolOverrideKey = path.Join(componentKey, overrideName)
			} else {
				// If no override name, use the original tool name so that we can lookup tool parameters
				toolOverrideKey = path.Join(componentKey, toolOverride.Name)
			}
			compositeContext.toolOverrides[toolOverrideKey] = toolOverride
		}
	}

	return compositeContext
}

func (c *compositeContext) toCompositeTool(componentKey string, tool nmcp.Tool) (*nmcp.Tool, error) {
	var (
		toolKey      = path.Join(componentKey, tool.Name)
		override, ok = c.toolOverrides[toolKey]
	)
	if !ok {
		// No override found, return the original tool (with component key prefix)
		tool.Name = fmt.Sprintf("%s_%s", componentKey, tool.Name)
		return &tool, nil
	}
	if !override.Enabled {
		// Tool is disabled, return nil
		return nil, nil
	}

	if overrideName := override.OverrideName; overrideName != "" && tool.Name != overrideName {
		tool.Name = overrideName
	}
	tool.Name = fmt.Sprintf("%s_%s", componentKey, tool.Name)

	if overrideDescription := override.OverrideDescription; overrideDescription != "" {
		tool.Description = overrideDescription
	}

	return &tool, nil
}

func (c *compositeContext) toComponentCallToolRequest(componentKey string, request nmcp.CallToolRequest) (*nmcp.CallToolRequest, error) {
	// Remove the component key prefix from the tool name
	request.Name = strings.TrimPrefix(request.Name, fmt.Sprintf("%s_", componentKey))

	var (
		toolKey      = path.Join(componentKey, request.Name)
		override, ok = c.toolOverrides[toolKey]
	)
	if !ok {
		// No override found
		return &request, nil
	}

	if !override.Enabled {
		// Tool is disabled, return nil
		return nil, nil
	}

	if request.Name != override.Name {
		request.Name = override.Name
	}

	return &request, nil
}

func (c *compositeContext) toCompositePrompt(componentKey string, prompt nmcp.Prompt) nmcp.Prompt {
	// Prefix the prompt name with the component key
	// This lets us lookup the target component server for prompt get requests
	prompt.Name = fmt.Sprintf("%s_%s", componentKey, prompt.Name)
	return prompt
}

func (c *compositeContext) toComponentGetPromptRequest(componentKey string, request nmcp.GetPromptRequest) nmcp.GetPromptRequest {
	// Remove the component key prefix from the prompt name
	request.Name = strings.TrimPrefix(request.Name, fmt.Sprintf("%s_", componentKey))
	return request
}
