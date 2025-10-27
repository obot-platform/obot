package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
)

func (h *Handler) asClientOption(session *nmcp.Session, userID, mcpID, mcpServerNamespace, mcpServerName, serverDisplayName, serverCatalogEntryName, serverCatalogName, powerUserWorkspaceID string) nmcp.ClientOption {
	ch := &clientMessageHandler{
		webhookHelper:   h.webhookHelper,
		gptClient:       h.gptClient,
		gatewayClient:   h.gatewayClient,
		pendingRequests: h.pendingRequestsForSession(session.ID()),
		session: &gatewaySession{
			session:                session,
			userID:                 userID,
			mcpID:                  mcpID,
			powerUserWorkspaceID:   powerUserWorkspaceID,
			serverNamespace:        mcpServerNamespace,
			serverName:             mcpServerName,
			serverDisplayName:      serverDisplayName,
			serverCatalogEntryName: serverCatalogEntryName,
			serverCatalogName:      serverCatalogName,
		},
	}

	opts := nmcp.ClientOption{
		ClientName:   "Obot MCP Gateway",
		TokenStorage: h.tokenStore.ForUserAndMCP(userID, mcpID),
		OnMessage:    ch.onMessage,
	}

	if session.InitializeRequest.Capabilities.Elicitation != nil {
		opts.OnElicit = func(ctx context.Context, msg nmcp.Message, _ nmcp.ElicitRequest) (nmcp.ElicitResult, error) {
			return nmcp.ElicitResult{
				// Returning action "handled" here tells nanobot that we are sending the response.
				Action: "handled",
			}, ch.onMessage(ctx, msg)
		}
	}

	if session.InitializeRequest.Capabilities.Sampling != nil {
		opts.OnSampling = ch.onSampling
	}

	if session.InitializeRequest.Capabilities.Roots != nil {
		opts.OnRoots = ch.onMessage
	}

	return opts
}

type clientMessageHandler struct {
	webhookHelper   *mcp.WebhookHelper
	gptClient       *gptscript.GPTScript
	gatewayClient   *gateway.Client
	pendingRequests *nmcp.PendingRequests
	session         *gatewaySession
}

func (c *clientMessageHandler) onMessage(ctx context.Context, msg nmcp.Message) error {
	if msg.Method == "" {
		// This is supposed to be a response to a request, but requester canceled the request.
		// Return an error indicating that the request was canceled.
		return fmt.Errorf("method is empty for message, the requester likely canceled and is no longer waiting for a response")
	}

	resp, err := c.handleMessage(ctx, msg)
	if err != nil {
		if msg.ID != nil {
			msg.SendError(ctx, err)
		}
		return err
	}

	if msg.ID != nil {
		if err = msg.Reply(ctx, resp); err != nil {
			err = fmt.Errorf("failed to reply to message: %w", err)
			msg.SendError(ctx, err)
			return err
		}
	}

	return nil
}

func (c *clientMessageHandler) onSampling(ctx context.Context, req nmcp.CreateMessageRequest) (nmcp.CreateMessageResult, error) {
	var result nmcp.CreateMessageResult

	msg, err := nmcp.NewMessage(methodSampling, req)
	if err != nil {
		return result, fmt.Errorf("failed to create message for request: %w", err)
	}

	// This message ID is only seen by us and the client. The MCP server has its own ID, we just don't see it here.
	// Calling this out because we've seen some issues with IDs as strings, but hopefully the clients that people use
	// handle this correctly.
	msg.ID = uuid.NewString()

	resp, err := c.handleMessage(ctx, *msg)
	if err != nil {
		return result, err
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil
}

func (c *clientMessageHandler) handleMessage(ctx context.Context, msg nmcp.Message) (json.RawMessage, error) {
	auditLog := gatewaytypes.MCPAuditLog{
		UserID:                    c.session.userID,
		MCPID:                     c.session.mcpID,
		PowerUserWorkspaceID:      c.session.powerUserWorkspaceID,
		MCPServerDisplayName:      c.session.serverDisplayName,
		MCPServerCatalogEntryName: c.session.serverCatalogEntryName,
		ClientName:                c.session.session.InitializeRequest.ClientInfo.Name,
		ClientVersion:             c.session.session.InitializeRequest.ClientInfo.Version,
		CreatedAt:                 time.Now(),
		CallType:                  msg.Method,
		CallIdentifier:            extractCallIdentifier(msg),
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

	var (
		err          error
		result       json.RawMessage
		dropAuditLog bool
	)
	defer func() {
		if dropAuditLog {
			return
		}

		// Complete audit log
		auditLog.ProcessingTimeMs = time.Since(auditLog.CreatedAt).Milliseconds()

		if err != nil {
			auditLog.Error = err.Error()
			if auditLog.ResponseStatus == 0 {
				auditLog.ResponseStatus = http.StatusInternalServerError
			}
		} else {
			auditLog.ResponseStatus = http.StatusOK
			auditLog.ResponseBody = result
		}

		c.gatewayClient.LogMCPAuditEntry(auditLog)
	}()

	var webhooks []mcp.Webhook
	webhooks, err = c.webhookHelper.GetWebhooksForMCPServer(ctx, c.gptClient, c.session.serverNamespace, c.session.serverName, c.session.serverCatalogEntryName, c.session.serverCatalogName, auditLog.CallType, auditLog.CallIdentifier)
	if err != nil {
		auditLog.ResponseStatus = http.StatusInternalServerError
		return nil, fmt.Errorf("failed to get webhooks: %w", err)
	}

	if err = fireWebhooks(ctx, webhooks, msg, &auditLog, "request", c.session.userID, c.session.mcpID); err != nil {
		auditLog.ResponseStatus = http.StatusFailedDependency
		return nil, fmt.Errorf("failed to fire webhooks for request: %w", err)
	}

	var ch <-chan nmcp.Message
	if msg.ID != nil {
		msg.ID = uuid.NewString()
		ch = c.pendingRequests.WaitFor(msg.ID)
		defer c.pendingRequests.Done(msg.ID)
	}

	if err = c.session.session.Send(ctx, msg); err != nil {
		if errors.Is(err, nmcp.ErrNoReader) {
			// No clients are reading these messages. Return and drop the audit log.
			dropAuditLog = true

			return nil, nil
		}
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	if msg.ID == nil || strings.HasPrefix(msg.Method, "notifications/") {
		// Notifications only go from server to client. We don't expect a response.
		return nil, nil
	}

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case m, ok := <-ch:
		if !ok {
			err = nmcp.ErrNoResponse
		}

		if m.Error != nil {
			err = m.Error
			return nil, fmt.Errorf("message returned with error: %w", err)
		}

		if err = fireWebhooks(ctx, webhooks, m, &auditLog, "response", c.session.userID, c.session.mcpID); err != nil {
			auditLog.ResponseStatus = http.StatusFailedDependency
			return nil, fmt.Errorf("failed to fire webhooks for response: %w", err)
		}

		result = m.Result
	}

	return result, nil
}

type gatewaySession struct {
	session                *nmcp.Session
	userID, mcpID          string
	powerUserWorkspaceID   string
	serverNamespace        string
	serverName             string
	serverDisplayName      string
	serverCatalogEntryName string
	serverCatalogName      string
}
