package mcpsession

import (
	"errors"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const sessionCleanupDuration = 24 * time.Hour

type Handler struct {
	gptClient *gptscript.GPTScript
}

func New(gptClient *gptscript.GPTScript) *Handler {
	return &Handler{
		gptClient: gptClient,
	}
}

func (h *Handler) RemoveUnused(req router.Request, resp router.Response) error {
	mcpSession := req.Object.(*v1.MCPSession)

	lastUsed := mcpSession.Status.LastUsedTime.Time
	if lastUsed.IsZero() {
		lastUsed = mcpSession.CreationTimestamp.Time
	}

	if since := time.Since(lastUsed); since > sessionCleanupDuration {
		return req.Delete(mcpSession)
	} else if retryAfter := sessionCleanupDuration - since; retryAfter < 10*time.Hour {
		resp.RetryAfter(retryAfter)
	}

	return nil
}

func (h *Handler) CleanupCredentials(req router.Request, _ router.Response) error {
	if err := h.gptClient.DeleteCredential(req.Ctx, req.Object.GetName(), "mcp-oauth"); err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return nil
		}
		return err
	}
	return nil
}
