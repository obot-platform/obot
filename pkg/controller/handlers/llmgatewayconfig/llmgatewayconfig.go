// Package llmgatewayconfig reconciles resource-backed configuration on each gateway replica.
package llmgatewayconfig

import (
	"context"
	"errors"
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
	_, syncErr := h.engine.Sync(req.Ctx)
	// Sync can apply healthy providers while preserving a rule whose target is
	// missing. Discover those successes even while incomplete application retries.
	// Always retry discovery too: a previous trigger may have failed after Sync
	// finished, leaving no runtime change on the next attempt.
	return errors.Join(syncErr, h.triggerModelSync(req.Ctx))
}
