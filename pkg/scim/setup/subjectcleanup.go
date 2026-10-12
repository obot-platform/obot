package setup

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"time"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/groupref"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	groupSubjectCleanupInterval  = 10 * time.Second
	groupSubjectCleanupBatchSize = 100
)

// RunGroupSubjectCleanups removes the subjects of groups that a SCIM DELETE deleted from access policies, until ctx
// is done. Every replica runs it, and claims keep replicas from usually running the same cleanup.
func RunGroupSubjectCleanups(ctx context.Context, gateway *gclient.Client, storage kclient.Client) {
	ticker := time.NewTicker(groupSubjectCleanupInterval)
	defer ticker.Stop()

	for {
		if err := CleanUpGroupSubjects(ctx, gateway, storage); err != nil && ctx.Err() == nil {
			slog.Error("Failed to remove the subjects of groups deleted through SCIM", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// CleanUpGroupSubjects runs the cleanups of group subjects that no replica is running. A cleanup that fails is
// retried once its claim expires.
func CleanUpGroupSubjects(ctx context.Context, gateway *gclient.Client, storage kclient.Client) error {
	cleanups, err := gateway.ClaimSCIMGroupSubjectCleanups(ctx, groupSubjectCleanupBatchSize)
	if err != nil || len(cleanups) == 0 {
		return err
	}

	// A write of group references that checked a group before the group's deletion committed may still be saving a
	// subject of it. Waiting for it means the policies read next include that subject.
	if err := gateway.WaitForSCIMReferenceWrites(ctx); err != nil {
		return err
	}

	// The policies of each namespace are read once for every cleanup claimed in it.
	groupIDs := make(map[string][]string)
	for _, cleanup := range cleanups {
		groupIDs[cleanup.Namespace] = append(groupIDs[cleanup.Namespace], cleanup.GroupID)
	}

	var errs []error
	for _, namespace := range slices.Sorted(maps.Keys(groupIDs)) {
		ids := groupIDs[namespace]
		deleted := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			deleted[id] = struct{}{}
		}
		if _, err := groupref.RemoveGroupSubjects(ctx, storage, namespace, func(groupID string) bool {
			_, ok := deleted[groupID]
			return ok
		}); err != nil {
			slog.Warn("Failed to remove the subjects of groups deleted through SCIM", "namespace", namespace, "groupIDs", ids, "error", err)
			errs = append(errs, err)
			for _, id := range ids {
				errs = append(errs, gateway.FailSCIMGroupSubjectCleanup(ctx, id, err))
			}
			continue
		}
		for _, id := range ids {
			if err := gateway.CompleteSCIMGroupSubjectCleanup(ctx, id); err != nil {
				errs = append(errs, err)
				continue
			}
			slog.Info("Removed the subjects of a group deleted through SCIM", "groupID", id)
		}
	}
	return errors.Join(errs...)
}
