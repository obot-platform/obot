// Package llmgatewayconfig reconciles resource-backed configuration on each gateway replica.
package llmgatewayconfig

import (
	"context"
	"time"

	"github.com/obot-platform/nah/pkg/router"
)

// Engine is the local gateway lifecycle needed by resource reconciliation.
// The application supplies the runtime adapter when it activates the core gateway.
type Engine interface {
	Ready() bool
	Sync(context.Context) (bool, error)
}

// Handler runs on every replica, independently of leader election. Application
// failures go back to the router for retry; the engine tracks its applied state.
type Handler struct {
	engine           Engine
	triggerModelSync func(context.Context) error
}

func New(engine Engine, triggerModelSync func(context.Context) error) *Handler {
	return &Handler{engine: engine, triggerModelSync: triggerModelSync}
}

func (h *Handler) Reconcile(req router.Request, resp router.Response) error {
	if !h.engine.Ready() {
		// Resource events can arrive before gateway startup finishes. Requeue them
		// so a startup-window edit cannot be lost while the engine is unavailable.
		resp.RetryAfter(time.Second)
		return nil
	}
	// Reload desired resources through Sync rather than applying a stale event payload.
	if _, err := h.engine.Sync(req.Ctx); err != nil {
		return err
	}
	// Application may have succeeded on a previous attempt whose discovery trigger
	// failed. Trigger discovery again even when Sync reports no runtime changes.
	return h.triggerModelSync(req.Ctx)
}
