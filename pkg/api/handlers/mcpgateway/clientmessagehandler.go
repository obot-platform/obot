package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type clientMessageHandlerFactory struct {
	gatewayClient   *gateway.Client
	pendingRequests *nmcp.PendingRequests
}

func (c *clientMessageHandlerFactory) asClientOption(tokenStore nmcp.TokenStorage, session *nmcp.Session, userID, mcpID, serverDisplayName, serverCatalogEntryName string) nmcp.ClientOption {
	return nmcp.ClientOption{
		ClientName:   "Obot MCP Gateway",
		TokenStorage: tokenStore,
		OnMessage: (&clientMessageHandler{
			gatewayClient:   c.gatewayClient,
			pendingRequests: c.pendingRequests,
			session: &gatewaySession{
				session:                session,
				userID:                 userID,
				mcpID:                  mcpID,
				serverDisplayName:      serverDisplayName,
				serverCatalogEntryName: serverCatalogEntryName,
			},
		}).onMessage,
	}
}

type clientMessageHandler struct {
	gatewayClient   *gateway.Client
	pendingRequests *nmcp.PendingRequests
	session         *gatewaySession
}

func (c *clientMessageHandler) onMessage(ctx context.Context, msg nmcp.Message) error {
	startTime := time.Now()
	auditLog := &gatewaytypes.MCPAuditLog{
		UserID:                    c.session.userID,
		MCPID:                     c.session.mcpID,
		MCPServerDisplayName:      c.session.serverDisplayName,
		MCPServerCatalogEntryName: c.session.serverCatalogEntryName,
		ClientName:                c.session.session.InitializeRequest.ClientInfo.Name,
		ClientVersion:             c.session.session.InitializeRequest.ClientInfo.Version,
		CreatedAt:                 startTime,
		CallType:                  msg.Method,
	}
	auditLog.RequestID, _ = msg.ID.(string)

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
		auditLog.ProcessingTimeMs = time.Since(startTime).Milliseconds()

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

		if strings.HasPrefix(msg.Method, "notifications/") {
			insertAuditLog(c.gatewayClient, auditLog)
		} else {
			updateAuditLog(c.gatewayClient, auditLog)
		}
	}()

	ch := c.pendingRequests.WaitFor(msg.ID)
	defer c.pendingRequests.Done(msg.ID)

	if err = c.session.session.Send(ctx, msg); err != nil {
		if errors.Is(err, nmcp.ErrNoReader) {
			// No clients are reading these messages. Return and drop the audit log.
			dropAuditLog = true

			return nil
		}
		msg.SendError(ctx, err)
		return fmt.Errorf("failed to send message: %w", err)
	}

	if strings.HasPrefix(msg.Method, "notifications/") {
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

		result = m.Result
		if err = msg.Reply(ctx, m.Result); err != nil {
			msg.SendError(ctx, err)
			return fmt.Errorf("failed to reply to message: %w", err)
		}
	}

	return err
}

type gatewaySession struct {
	session                *nmcp.Session
	userID, mcpID          string
	serverDisplayName      string
	serverCatalogEntryName string
}
