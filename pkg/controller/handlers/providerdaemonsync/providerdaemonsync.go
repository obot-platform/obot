package providerdaemonsync

import (
	"sync"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type daemonStopper interface {
	StopAllAuthProviderDaemons()
	StopAllModelProviderDaemons()
}

// Handler owns replica-local state and is registered on a router without
// leader election, so each replica independently stops its cached daemons.
type Handler struct {
	mu                sync.Mutex
	lastDaemonRestart time.Time
	dispatcher        daemonStopper
}

func New(dispatcher daemonStopper) *Handler {
	return &Handler{
		lastDaemonRestart: time.Now(),
		dispatcher:        dispatcher,
	}
}

func (h *Handler) Reconcile(req router.Request, _ router.Response) error {
	daemonSync := req.Object.(*v1.ProviderDaemonSync)
	timestamp := daemonSync.Spec.Timestamp.Time
	if timestamp.IsZero() {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if !timestamp.After(h.lastDaemonRestart) {
		return nil
	}

	h.dispatcher.StopAllAuthProviderDaemons()
	h.dispatcher.StopAllModelProviderDaemons()
	h.lastDaemonRestart = timestamp
	return nil
}
