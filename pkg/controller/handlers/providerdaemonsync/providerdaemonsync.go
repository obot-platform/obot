package providerdaemonsync

import (
	"sync"

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
	mu             sync.Mutex
	lastGeneration int64
	dispatcher     daemonStopper
}

func New(dispatcher daemonStopper) *Handler {
	return &Handler{
		dispatcher: dispatcher,
	}
}

func (h *Handler) Reconcile(req router.Request, _ router.Response) error {
	generation := req.Object.(*v1.ProviderDaemonSync).Spec.Generation

	h.mu.Lock()
	defer h.mu.Unlock()
	if generation <= h.lastGeneration {
		return nil
	}

	h.dispatcher.StopAllAuthProviderDaemons()
	h.dispatcher.StopAllModelProviderDaemons()
	h.lastGeneration = generation
	return nil
}
