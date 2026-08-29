package providerdaemonsync

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type daemonStopper interface {
	StopAuthProvider(namespace, authProviderName string)
	StopModelProvider(namespace, modelProviderName string)
}

// Handler owns replica-local state and is registered on a router without
// leader election, so each replica independently stops its cached daemons.
type Handler struct {
	mu            sync.Mutex
	lastRevisions map[string]int64
	dispatcher    daemonStopper
}

func New(dispatcher daemonStopper) *Handler {
	return &Handler{
		lastRevisions: make(map[string]int64),
		dispatcher:    dispatcher,
	}
}

func (h *Handler) Reconcile(req router.Request, _ router.Response) error {
	daemonSync := req.Object.(*v1.ProviderDaemonSync)

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lastRevisions == nil {
		h.lastRevisions = make(map[string]int64)
	}

	for key, revision := range daemonSync.Spec.Revisions {
		observationKey := string(daemonSync.UID) + "/" + key
		if revision.Revision <= h.lastRevisions[observationKey] {
			continue
		}

		switch revision.ProviderType {
		case v1.ProviderTypeAuth:
			slog.Info("Stopping auth provider daemon after synchronized configuration change",
				"authProvider", revision.ProviderName,
				"namespace", revision.ProviderNamespace,
				"revision", revision.Revision,
			)
			h.dispatcher.StopAuthProvider(revision.ProviderNamespace, revision.ProviderName)
		case v1.ProviderTypeModel:
			slog.Info("Stopping model provider daemon after synchronized configuration change",
				"modelProvider", revision.ProviderName,
				"namespace", revision.ProviderNamespace,
				"revision", revision.Revision,
			)
			h.dispatcher.StopModelProvider(revision.ProviderNamespace, revision.ProviderName)
		default:
			return fmt.Errorf("provider daemon revision %q has invalid provider type %q", key, revision.ProviderType)
		}
		h.lastRevisions[observationKey] = revision.Revision
	}
	return nil
}
