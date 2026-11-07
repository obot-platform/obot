// Package llmgatewayconfig reconciles resource-backed configuration on each gateway replica.
package llmgatewayconfig

import (
	"context"
	"sync"
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
	mu      sync.Mutex
	running bool
	pending bool

	engine Engine
}

func New(engine Engine) *Handler {
	return &Handler{engine: engine}
}

func (h *Handler) Reconcile(req router.Request, resp router.Response) error {
	if !h.engine.Ready() {
		// Resource events can arrive before gateway startup finishes. Requeue them
		// so a startup-window edit cannot be lost while the engine is unavailable.
		resp.RetryAfter(time.Second)
		return nil
	}
	// The active event owns retries; overlapping events request another pass.
	h.mu.Lock()
	if h.running {
		h.pending = true
		h.mu.Unlock()
		return nil
	}
	h.running = true
	h.mu.Unlock()

	for {
		// Load the latest store state, including changes made during the previous pass.
		_, err := h.engine.Sync(req.Ctx)

		h.mu.Lock()
		if err != nil || !h.pending {
			// Completion and admission share the lock so a late event cannot be lost.
			h.running, h.pending = false, false
			h.mu.Unlock()
			return err
		}
		h.pending = false
		h.mu.Unlock()
	}
}
