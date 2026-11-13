// Package billing is the obot-billing subscription of a billed Obot Cloud environment. Client asks obot-billing
// itself. Provider provides the entitlements the subscription grants, and the restriction for staying over them, from
// the Subscription the leader stores.
package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// gracePeriod is how long the environment can be over its limits, or past its period without an answer from
	// billing, before it's restricted.
	gracePeriod = 3 * 24 * time.Hour
)

var (
	subscriptionKey = kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.SubscriptionName,
	}
)

// Provider provides a billed environment's entitlements, in place of its license.
type Provider struct {
	// Client asks obot-billing itself.
	Client *Client

	cache, storage kclient.Reader
}

// NewProvider returns the provider for the environment that client's billing key belongs to. Requests read the
// Subscription all the time, so it's read from the cache, which follows the leader's writes within moments. Startup
// reads it before the cache starts, and those reads go to storage.
func NewProvider(client *Client, cache, storage kclient.Reader) *Provider {
	return &Provider{
		Client:  client,
		cache:   cache,
		storage: storage,
	}
}

// Entitlements are what the subscription grants.
func (p *Provider) Entitlements(ctx context.Context) (types.Entitlements, error) {
	subscription, err := p.subscription(ctx)
	if err != nil {
		return types.Entitlements{}, err
	}
	return subscription.Entitlements(), nil
}

// Restricted reports whether only admins and owners can use the environment. It's restricted once usage has been
// over the limits for the grace period, or once billing hasn't answered for the grace period past the end of the
// period it last answered with.
func (p *Provider) Restricted(ctx context.Context) (bool, error) {
	subscription, err := p.subscription(ctx)
	if err != nil {
		return false, err
	}

	status := subscription.Status
	if since := status.OverLimitSince; since != nil && time.Since(since.Time) >= gracePeriod {
		return true, nil
	}

	// A failed sync keeps the last answer, but only this far past the period it was for. An enterprise subscription
	// has no period to run out.
	lapsed := status.PeriodEnd != nil && time.Since(status.PeriodEnd.Time) >= gracePeriod
	return status.SyncError != "" && lapsed, nil
}

// subscription returns the environment's Subscription, or an empty one before the leader creates it.
func (p *Provider) subscription(ctx context.Context) (v1.Subscription, error) {
	var subscription v1.Subscription
	err := p.cache.Get(ctx, subscriptionKey, &subscription)
	if _, notStarted := errors.AsType[*cache.ErrCacheNotStarted](err); notStarted {
		// Still starting up, so read what's stored.
		err = p.storage.Get(ctx, subscriptionKey, &subscription)
	}
	if kclient.IgnoreNotFound(err) != nil {
		return v1.Subscription{}, fmt.Errorf("failed to read the subscription: %w", err)
	}
	return subscription, nil
}

// RequestSync asks the leader to sync the Subscription with billing now, by setting SubscriptionSyncAnnotation. A sync
// that's already pending covers the request, so it isn't written again. Without a Subscription, which only a billed
// environment has, it returns a not-found error.
func RequestSync(ctx context.Context, c kclient.Client) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var subscription v1.Subscription
		if err := c.Get(ctx, subscriptionKey, &subscription); err != nil {
			return err
		}
		if subscription.Annotations[v1.SubscriptionSyncAnnotation] == "true" {
			return nil
		}
		if subscription.Annotations == nil {
			subscription.Annotations = make(map[string]string, 1)
		}
		subscription.Annotations[v1.SubscriptionSyncAnnotation] = "true"
		return c.Update(ctx, &subscription)
	})
}
