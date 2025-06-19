package mcpgateway

import (
	"context"
	"fmt"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
)

func clientMessageHandlerAsClientOption(session *nmcp.Session, pendingRequests *nmcp.PendingRequests) nmcp.ClientOption {
	c := &clientMessageHandler{
		session:         session,
		pendingRequests: pendingRequests,
	}
	return nmcp.ClientOption{
		OnMessage: c.onMessage,
	}
}

type clientMessageHandler struct {
	session         *nmcp.Session
	pendingRequests *nmcp.PendingRequests
}

func (c *clientMessageHandler) onMessage(ctx context.Context, msg nmcp.Message) error {
	ch := c.pendingRequests.WaitFor(msg.ID)
	defer c.pendingRequests.Done(msg.ID)

	if err := c.session.Send(ctx, msg); err != nil {
		msg.SendError(ctx, err)
		return fmt.Errorf("failed to send message: %w", err)
	}

	select {
	case <-ctx.Done():
		msg.SendError(ctx, ctx.Err())
	case m, ok := <-ch:
		if !ok {
			msg.SendError(ctx, nmcp.ErrNoResponse)
		}

		if m.Error != nil {
			msg.SendError(ctx, m.Error)
			return fmt.Errorf("message returned with error: %w", m.Error)
		}

		if err := msg.Reply(ctx, m.Result); err != nil {
			msg.SendError(ctx, err)
			return fmt.Errorf("failed to reply to message: %w", err)
		}
	}

	return nil
}
