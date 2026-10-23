package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// GracePeriod is how long the environment can be over its plan's limits before it's restricted.
	GracePeriod = 24 * time.Hour
)

var (
	// Key is the environment's single BillingSubscription.
	Key = kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.BillingSubscriptionName}
)

// Billing is the entitlements source while billing is on. It reads the BillingSubscription, which only the leader
// writes, straight from storage, so every replica sees the same state.
type Billing struct {
	storage kclient.Client
	client  *Client
}

func New(storage kclient.Client, client *Client) *Billing {
	return &Billing{
		storage: storage,
		client:  client,
	}
}

// EnsureSubscription creates the BillingSubscription if it doesn't exist. It's safe to run on every replica.
func EnsureSubscription(ctx context.Context, c kclient.Client) error {
	if err := c.Create(ctx, &v1.BillingSubscription{
		Namespace: Key.Namespace,
		Name:      Key.Name,
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create the billing subscription: %w", err)
	}
	return nil
}

// Read returns the BillingSubscription, or an empty one if it doesn't exist yet.
func (b *Billing) Read(ctx context.Context) (v1.BillingSubscription, error) {
	var subscription v1.BillingSubscription
	if err := b.storage.Get(ctx, Key, &subscription); err != nil && !apierrors.IsNotFound(err) {
		return v1.BillingSubscription{}, fmt.Errorf("failed to read the billing subscription: %w", err)
	}
	return subscription, nil
}

func (b *Billing) status(ctx context.Context) (v1.BillingSubscriptionStatus, error) {
	subscription, err := b.Read(ctx)
	return subscription.Status, err
}

// Entitlements returns the plan's entitlements.
func (b *Billing) Entitlements(ctx context.Context) (types.Entitlements, error) {
	status, err := b.status(ctx)
	if err != nil {
		return types.Entitlements{}, err
	}
	return EntitlementsOf(status), nil
}

// Usage returns the usage the leader last counted.
func (b *Billing) Usage(ctx context.Context) (types.EntitlementsUsage, error) {
	status, err := b.status(ctx)
	return status.Usage, err
}

// Restricted returns whether the leader has restricted the environment.
func (b *Billing) Restricted(ctx context.Context) (bool, error) {
	status, err := b.status(ctx)
	return status.Restricted, err
}

// RequestSync asks the leader to sync with billing now. The leader removes the sync annotation when it's done.
func (b *Billing) RequestSync(ctx context.Context) error {
	var subscription v1.BillingSubscription
	if err := b.storage.Get(ctx, Key, &subscription); err != nil {
		return fmt.Errorf("failed to read the billing subscription: %w", err)
	}
	if subscription.Annotations == nil {
		subscription.Annotations = map[string]string{}
	}
	subscription.Annotations[v1.BillingSyncAnnotation] = "true"
	return b.storage.Update(ctx, &subscription)
}

// CreateSession opens the billing pages for an owner.
func (b *Billing) CreateSession(ctx context.Context, returnURL string) (types.BillingSession, error) {
	return b.client.CreateSession(ctx, returnURL)
}

// AuditLogRetention returns the plan's audit log retention, or mcpDays and llmDays if the plan doesn't set one.
func (b *Billing) AuditLogRetention(mcpDays, llmDays int) gatewayclient.AuditLogRetention {
	return func(ctx context.Context) (int, int, error) {
		status, err := b.status(ctx)
		if err != nil || status.Entitlements == nil {
			// Keep everything until billing first answers.
			return 0, 0, err
		}
		if days := status.Entitlements.AuditLogRetentionDays; days != nil {
			return int(*days), int(*days), nil
		}
		return mcpDays, llmDays, nil
	}
}

// SubscriptionOf returns billing's last subscription answer, or nil before it first answers.
func SubscriptionOf(status v1.BillingSubscriptionStatus) *types.Subscription {
	if status.Subscription == nil {
		return nil
	}
	return &types.Subscription{
		Plan:              status.Subscription.Plan,
		Status:            status.Subscription.Status,
		PeriodEnd:         v1.NewTime(status.Subscription.PeriodEnd),
		CancelAtPeriodEnd: status.Subscription.CancelAtPeriodEnd,
		SelfService:       status.Subscription.SelfService,
	}
}

// EntitlementsOf returns billing's last entitlements answer. Before billing first answers, no features are granted and
// every limit is 0.
func EntitlementsOf(status v1.BillingSubscriptionStatus) types.Entitlements {
	if status.Entitlements == nil {
		return types.Entitlements{
			Cloud:            true,
			Users:            new(int64),
			Devices:          new(int64),
			HostedMCPServers: new(int64),
			MemoryMiB:        new(int64),
			CPUMillicores:    new(int64),
		}
	}
	entitlements := *status.Entitlements.DeepCopy()
	entitlements.Cloud = true
	return entitlements
}
