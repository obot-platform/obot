package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/obot-platform/nah/pkg/backend"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// We need to reconcile the status fields on auth providers on a routine interval.
// This is because the API route that handles auth provider reconfiguration could
// successfully write the new credential to the database but then fail to add the sync
// annotation to the AuthProvider kinm object, leaving the Status fields stale.
// During the authentication process, the authenticator fetches the credential and
// the AuthProvider object and makes sure that everything lines up before allowing
// the user to authenticate. So it could be possible, due to an intermittent DB failure
// at the worst time, for the AuthProvider Status fields to be stale and for
// authentication to fail as a result.
//
// In the future, we could consider finding a way to wrap the reconciliation logic in a
// transaction. This is not trivial, since the credential is outside of kinm and the
// AuthProvider is inside.

const authProviderReconciliationInterval = 30 * time.Second

func (c *Controller) runAuthProviderReconciliation(ctx context.Context, client kclient.Client) {
	ticker := time.NewTicker(authProviderReconciliationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := reconcileAuthProviders(ctx, client, c.services.Router.Backend()); err != nil {
				slog.Error("Failed to reconcile auth providers", "error", err)
			}
		}
	}
}

func reconcileAuthProviders(ctx context.Context, client kclient.Client, trigger backend.Trigger) error {
	var authProviders v1.AuthProviderList
	if err := client.List(ctx, &authProviders); err != nil {
		return fmt.Errorf("failed to list auth providers: %w", err)
	}

	var errs []error
	for i := range authProviders.Items {
		authProvider := &authProviders.Items[i]
		key := kclient.ObjectKeyFromObject(authProvider).String()
		if err := trigger.Trigger(ctx, v1.SchemeGroupVersion.WithKind("AuthProvider"), key, 0); err != nil {
			errs = append(errs, fmt.Errorf("failed to trigger auth provider %q: %w", key, err))
		}
	}

	return errors.Join(errs...)
}
