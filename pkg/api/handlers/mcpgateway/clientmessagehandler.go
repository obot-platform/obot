package mcpgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func (m *messageHandler) clientMessageHandlerAsClientOption(tokenStore nmcp.TokenStorage, session *nmcp.Session) nmcp.ClientOption {
	return nmcp.ClientOption{
		TokenStorage: tokenStore,
		OnMessage: (&clientMessageHandler{
			session:         session,
			pendingRequests: m.handler.pendingRequests,
			messageHandler:  m,
		}).onMessage,
	}
}

type clientMessageHandler struct {
	session         *nmcp.Session
	pendingRequests *nmcp.PendingRequests
	messageHandler  *messageHandler
}

func (c *clientMessageHandler) onMessage(ctx context.Context, msg nmcp.Message) error {
	// Capture audit log information
	startTime := time.Now()
	auditLog := &gatewaytypes.MCPAuditLog{
		UserID:               c.messageHandler.userID,
		MCPID:                c.messageHandler.mcpID,
		MCPServerDisplayName: c.messageHandler.mcpServer.Spec.Manifest.Name,
		ClientInfo:           gatewaytypes.ClientInfo(msg.Session.InitializeRequest.ClientInfo),
		CreatedAt:            startTime,
		CallType:             msg.Method,
	}
	auditLog.RequestID, _ = msg.ID.(string)

	// Capture request body if available
	if msg.Params != nil {
		if requestBody, err := json.Marshal(msg.Params); err == nil {
			auditLog.RequestBody = requestBody
		}
	}

	var (
		err    error
		result json.RawMessage
	)

	defer func() {
		// Complete audit log
		auditLog.ProcessingTimeMs = time.Since(startTime).Milliseconds()

		if err != nil {
			auditLog.Error = err.Error()
			auditLog.ResponseStatus = http.StatusInternalServerError
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
			c.messageHandler.insertAuditLog(auditLog)
		} else {
			c.messageHandler.updateAuditLog(auditLog)
		}
	}()

	ch := c.pendingRequests.WaitFor(msg.ID)
	defer c.pendingRequests.Done(msg.ID)

	if err = c.session.Send(ctx, msg); err != nil {
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
