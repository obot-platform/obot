package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/obot-platform/obot/pkg/billing"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// subscriptionSyncRequestTimeout bounds a sync request, which doesn't stop when the deleting request's context
	// does.
	subscriptionSyncRequestTimeout = 10 * time.Second
)

// requestSubscriptionSync asks the leader to sync the Subscription, as the license page does, after a delete lowered
// the user or device count. The sync counts usage again, so once usage fits, the over-limit clock, and any
// restriction it led to, clear right away instead of at the next scheduled sync. Callers request it once the delete
// commits, so the count includes the delete.
//
// It does nothing in an environment that isn't billed, which has no Subscription, or while a sync is already
// pending: the leader counts after it clears the request, so that count includes the delete too, and a bulk delete
// writes the Subscription once per sync. A failure is only logged: the delete has happened, and the next scheduled
// sync counts it.
func (c *Client) requestSubscriptionSync(ctx context.Context) {
	// The delete has committed, so make the request even if the caller has gone away.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), subscriptionSyncRequestTimeout)
	defer cancel()
	if err := billing.RequestSync(ctx, c.storageClient); kclient.IgnoreNotFound(err) != nil {
		slog.Warn("Failed to request a subscription sync", "error", err)
	}
}
