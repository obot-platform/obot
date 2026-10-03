package setup

import (
	"context"
	"errors"
	"log/slog"
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

	var errs []error
	for _, cleanup := range cleanups {
		if _, err := groupref.RemoveGroupSubjects(ctx, storage, cleanup.Namespace, func(groupID string) bool {
			return groupID == cleanup.GroupID
		}); err != nil {
			slog.Warn("Failed to remove the subjects of a group deleted through SCIM", "groupID", cleanup.GroupID, "error", err)
			errs = append(errs, err, gateway.FailSCIMGroupSubjectCleanup(ctx, cleanup.GroupID, err))
			continue
		}
		if err := gateway.CompleteSCIMGroupSubjectCleanup(ctx, cleanup.GroupID); err != nil {
			errs = append(errs, err)
			continue
		}
		slog.Info("Removed the subjects of a group deleted through SCIM", "groupID", cleanup.GroupID)
	}
	return errors.Join(errs...)
}
