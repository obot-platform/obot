package mcphookcorrelation

import (
	"time"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	now func() time.Time
}

func New() *Handler {
	return &Handler{now: time.Now}
}

func (h *Handler) Cleanup(req router.Request, resp router.Response) error {
	correlation := req.Object.(*v1.MCPHookCorrelation)
	remaining := correlation.Spec.ExpiresAt.Sub(h.now())
	if remaining <= 0 {
		return req.Delete(correlation)
	}
	resp.RetryAfter(remaining)
	return nil
}
