package client

import (
	"context"
	"log/slog"
	"time"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// usageRecountRequestTimeout bounds a recount request, which doesn't stop when the deleting request's context
	// does.
	usageRecountRequestTimeout = 10 * time.Second
)

// requestUsageRecount has the leader count usage against the Subscription's limits again, after a delete lowered the
// user or device count. Once usage fits, the over-limit clock, and any restriction it led to, clear right away
// instead of at the next scheduled sync. Callers request it once the delete commits, so the count includes the delete.
//
// The request is an empty patch of the Subscription: storage records it as a change, which runs the Subscription's
// handlers, and billing isn't asked again unless a sync is due. It does nothing in an environment that isn't billed,
// which has no Subscription. A failure is only logged: the delete has happened, and the next scheduled sync counts it.
func (c *Client) requestUsageRecount(ctx context.Context) {
	// The delete has committed, so make the request even if the caller has gone away.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), usageRecountRequestTimeout)
	defer cancel()
	subscription := &v1.Subscription{
		Namespace: system.DefaultNamespace,
		Name:      system.SubscriptionName,
	}
	if err := c.storageClient.Patch(ctx, subscription, kclient.RawPatch(ktypes.MergePatchType, []byte("{}"))); kclient.IgnoreNotFound(err) != nil {
		slog.Warn("Failed to request a usage recount", "error", err)
	}
}
