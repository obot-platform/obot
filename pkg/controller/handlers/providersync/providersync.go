package providersync

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type daemonStopper interface {
	StopAuthProvider(namespace, authProviderName string)
}

type licenseRefresher interface {
	Validate(context.Context) error
}

// GatewayReloader reloads this replica's LLM gateway when its configuration revision
// advances. It reports whether it reloaded.
type GatewayReloader func(ctx context.Context, revision int64) (bool, error)

// Handler owns replica-local state and is registered on a router without
// leader election, so each replica independently refreshes its cached provider state.
type Handler struct {
	mu              sync.RWMutex
	lastRevisions   map[string]int64
	dispatcher      daemonStopper
	licenseProvider licenseRefresher
	reloadGateway   GatewayReloader
}

// New creates the handler.
func New(dispatcher daemonStopper, licenseProvider licenseRefresher, reloadGateway GatewayReloader) *Handler {
	return &Handler{
		lastRevisions:   make(map[string]int64),
		dispatcher:      dispatcher,
		licenseProvider: licenseProvider,
		reloadGateway:   reloadGateway,
	}
}

func (h *Handler) Reconcile(req router.Request, _ router.Response) error {
	daemonSync := req.Object.(*v1.ProviderSync)

	h.mu.RLock()
	if h.lastRevisions == nil {
		h.lastRevisions = make(map[string]int64)
	}
	lastRevisions := maps.Clone(h.lastRevisions)
	h.mu.RUnlock()

	for key, revision := range daemonSync.Spec.Revisions {
		observationKey := string(daemonSync.UID) + "/" + key
		if revision.Revision <= lastRevisions[observationKey] {
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
			// Model providers were replaced by the LLM gateway; they have no daemon.
		case v1.ProviderTypeLicense:
			slog.Info("Refreshing license after synchronized license check",
				"revision", revision.Revision,
			)
			if err := h.licenseProvider.Validate(req.Ctx); err != nil {
				return fmt.Errorf("refresh license at revision %d: %w", revision.Revision, err)
			}
		case v1.ProviderTypeLLMGateway:
			if h.reloadGateway != nil {
				reloaded, err := h.reloadGateway(req.Ctx, revision.Revision)
				if err != nil {
					return fmt.Errorf("reload LLM gateway at revision %d: %w", revision.Revision, err)
				}
				if reloaded {
					slog.Info("Reloaded LLM gateway after synchronized configuration change", "revision", revision.Revision)
				}
			}
		default:
			return fmt.Errorf("provider daemon revision %q has invalid provider type %q", key, revision.ProviderType)
		}
		h.mu.Lock()
		if h.lastRevisions[observationKey] < revision.Revision {
			h.lastRevisions[observationKey] = revision.Revision
		}
		h.mu.Unlock()
	}

	return nil
}
