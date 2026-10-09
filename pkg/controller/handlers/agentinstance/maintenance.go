package agentinstance

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/substrate"
	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// RunMaintenance runs on the elected controller replica. Database environments
// do not emit Kubernetes watches, so poll alongside the orchestrator refresh.
func (h *Handler) RunMaintenance(ctx context.Context, storage kclient.Client) {
	timer := time.NewTicker(15 * time.Second)
	defer timer.Stop()
	for ctx.Err() == nil {
		iteration, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := h.maintain(iteration, storage)
		cancel()
		if err != nil {
			slog.ErrorContext(ctx, "Claude environment maintenance failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func (h *Handler) maintain(ctx context.Context, storage kclient.Client) error {
	environments, err := h.Gateway.ClaudeEnvironments(ctx)
	if err != nil {
		return err
	}

	var instances v1.AgentInstanceList
	if err := storage.List(ctx, &instances); err != nil {
		return err
	}

	inUse := map[string]bool{}
	for _, instance := range instances.Items {
		inUse[instance.Spec.ClaudeEnvironment] = true
	}

	// Discover remote resources without blocking admission or credential refresh.
	if err := h.Substrate.ReconcileWorkerPools(ctx, environments, inUse, func(ctx context.Context, id string, desired *types.ClaudeEnvironment, apply func(context.Context) error) error {
		return h.maintenanceMutation(ctx, func(ctx context.Context) error {
			current, err := h.Gateway.ClaudeEnvironment(ctx, id)
			if desired != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				if err != nil {
					return err
				}
				if !reflect.DeepEqual(current.Runtime, desired.Runtime) {
					return nil
				}
			} else {
				if err == nil {
					return nil
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				var live v1.AgentInstanceList
				if err := storage.List(ctx, &live); err != nil {
					return err
				}
				for _, instance := range live.Items {
					if instance.Spec.ClaudeEnvironment == id {
						return nil
					}
				}
			}

			return apply(ctx)
		})
	}); err != nil {
		return err
	}

	return h.Substrate.CollectOrphans(ctx, instances.Items, func(ctx context.Context, ref *pb.ObjectRef) error {
		return h.maintenanceMutation(ctx, func(ctx context.Context) error {
			var live v1.AgentInstanceList
			if err := storage.List(ctx, &live); err != nil {
				return err
			}
			for i := range live.Items {
				instance := &live.Items[i]
				current := substrate.Ref(instance)
				if current.Atespace == ref.Atespace && (current.Name == ref.Name || instance.Spec.ClaudePrevious == ref.Name) {
					return nil
				}
			}

			return h.Substrate.DeleteExecution(ctx, ref)
		})
	})
}

// A failed control-plane mutation must release the admission lock promptly.
func (h *Handler) maintenanceMutation(ctx context.Context, apply func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return h.Gateway.WithClaudeProvisionLock(ctx, func() error { return apply(ctx) })
}
