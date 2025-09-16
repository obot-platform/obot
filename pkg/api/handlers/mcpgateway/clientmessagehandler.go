package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
)

func (h *Handler) asClientOption(session *nmcp.Session, userID, mcpID, mcpServerNamespace, mcpServerName, serverDisplayName, serverCatalogEntryName string) nmcp.ClientOption {
	return nmcp.ClientOption{
		ClientName:   "Obot MCP Gateway",
		TokenStorage: h.tokenStore.ForUserAndMCP(userID, mcpID),
		OnMessage: (&clientMessageHandler{
			webhookHelper:   h.webhookHelper,
			gptClient:       h.gptClient,
			gatewayClient:   h.gatewayClient,
			pendingRequests: h.pendingRequestsForSession(session.ID()),
			session: &gatewaySession{
				session:                session,
				userID:                 userID,
				mcpID:                  mcpID,
				serverNamespace:        mcpServerNamespace,
				serverName:             mcpServerName,
				serverDisplayName:      serverDisplayName,
				serverCatalogEntryName: serverCatalogEntryName,
			},
		}).onMessage,
	}
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

	auditLog := gatewaytypes.MCPAuditLog{
		UserID:                    c.session.userID,
		MCPID:                     c.session.mcpID,
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
			// Capture response body if available
			if result != nil {
				if responseBody, err := json.Marshal(result); err == nil {
					auditLog.ResponseBody = responseBody
				}
			}
		}

		c.gatewayClient.LogMCPAuditEntry(auditLog)
	}()

	var webhooks []mcp.Webhook
	webhooks, err = c.webhookHelper.GetWebhooksForMCPServer(ctx, c.gptClient, c.session.serverNamespace, c.session.serverName, c.session.serverCatalogEntryName, auditLog.CallType, auditLog.CallIdentifier)
	if err != nil {
		msg.SendError(ctx, err)
		auditLog.ResponseStatus = http.StatusInternalServerError
		return fmt.Errorf("failed to get webhooks: %w", err)
	}

	if err = fireWebhooks(ctx, webhooks, msg, &auditLog, "request", c.session.userID, c.session.mcpID); err != nil {
		msg.SendError(ctx, err)
		auditLog.ResponseStatus = http.StatusFailedDependency
		return fmt.Errorf("failed to fire webhooks: %w", err)
	}

	var ch <-chan nmcp.Message
	if msg.ID != nil {
		ch = c.pendingRequests.WaitFor(msg.ID)
		defer c.pendingRequests.Done(msg.ID)
	}

	if err = c.session.session.Send(ctx, msg); err != nil {
		if errors.Is(err, nmcp.ErrNoReader) {
			// No clients are reading these messages. Return and drop the audit log.
			dropAuditLog = true

			return nil
		}
		msg.SendError(ctx, err)
		return fmt.Errorf("failed to send message: %w", err)
	}

	if msg.ID == nil || strings.HasPrefix(msg.Method, "notifications/") {
		// Notifications only go from server to client. We don't expect a response.
		return nil
	}

	select {
	case <-ctx.Done():
		err = ctx.Err()
		msg.SendError(ctx, err)
	case m, ok := <-ch:
		if !ok {
			err = nmcp.ErrNoResponse
			msg.SendError(ctx, err)
		}

		if m.Error != nil {
			err = m.Error
			msg.SendError(ctx, err)
			return fmt.Errorf("message returned with error: %w", err)
		}

		if err = fireWebhooks(ctx, webhooks, m, &auditLog, "response", c.session.userID, c.session.mcpID); err != nil {
			msg.SendError(ctx, err)
			auditLog.ResponseStatus = http.StatusFailedDependency
			return fmt.Errorf("failed to fire webhooks: %w", err)
		}

		result = m.Result
		if err = msg.Reply(ctx, result); err != nil {
			msg.SendError(ctx, err)
			return fmt.Errorf("failed to reply to message: %w", err)
		}
	}

	return err
}

type gatewaySession struct {
	session                *nmcp.Session
	userID, mcpID          string
	serverNamespace        string
	serverName             string
	serverDisplayName      string
	serverCatalogEntryName string
}
