package mcpclientsession

import (
	"time"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	maxIdleTime = 7 * 24 * time.Hour
)

type Handler struct {
	now func() time.Time
}

func New() *Handler {
	return &Handler{now: time.Now}
}

func (h *Handler) Cleanup(req router.Request, resp router.Response) error {
	session := req.Object.(*v1.MCPClientSession)
	lastUsed := session.Status.LastUsed
	if lastUsed.IsZero() {
		lastUsed = session.CreationTimestamp
	}
	if remaining := lastUsed.Add(maxIdleTime).Sub(h.now()); remaining <= 0 {
		return req.Delete(session)
	} else if remaining < 10*time.Hour {
		resp.RetryAfter(remaining)
	}
	return nil
}
