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
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// GracePeriod is how long the environment can be over its plan's limits before it's restricted.
	GracePeriod = 3 * 24 * time.Hour
	// maxAuditLogRetentionDays caps the plan's audit log retention, so the cleanup cutoff stays within the dates the
	// database can store.
	maxAuditLogRetentionDays = 100 * 365
)

var (
	// Key is the environment's single BillingSubscription.
	Key = kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.BillingSubscriptionName}
)

// Subscription reads the environment's BillingSubscription, which only the leader writes. It reads straight from
// storage, so every replica sees the same plan.
type Subscription struct {
	storage kclient.Client
	client  *Client
}

func NewSubscription(storage kclient.Client, client *Client) *Subscription {
	return &Subscription{
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
func (s *Subscription) Read(ctx context.Context) (v1.BillingSubscription, error) {
	return read(ctx, s.storage)
}

// read reads the BillingSubscription through c, or returns an empty one if it doesn't exist yet.
func read(ctx context.Context, c kclient.Client) (v1.BillingSubscription, error) {
	var subscription v1.BillingSubscription
	if err := c.Get(ctx, Key, &subscription); err != nil && !apierrors.IsNotFound(err) {
		return v1.BillingSubscription{}, fmt.Errorf("failed to read the billing subscription: %w", err)
	}
	return subscription, nil
}

// Entitlements returns the plan's entitlements.
func (s *Subscription) Entitlements(ctx context.Context) (types.Entitlements, error) {
	subscription, err := s.Read(ctx)
	if err != nil {
		return types.Entitlements{}, err
	}
	return EntitlementsOf(subscription.Status), nil
}

// RequestSync asks the leader to sync with billing now. The leader removes the sync annotation when it's done.
func (s *Subscription) RequestSync(ctx context.Context) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var subscription v1.BillingSubscription
		if err := s.storage.Get(ctx, Key, &subscription); err != nil {
			return fmt.Errorf("failed to read the billing subscription: %w", err)
		}
		if subscription.Annotations == nil {
			subscription.Annotations = map[string]string{}
		}
		subscription.Annotations[v1.BillingSyncAnnotation] = "true"
		return s.storage.Update(ctx, &subscription)
	})
}

// CreateSession opens the billing pages for an owner.
func (s *Subscription) CreateSession(ctx context.Context, returnURL string) (types.BillingSession, error) {
	return s.client.CreateSession(ctx, returnURL)
}

// AuditLogRetention returns the plan's audit log retention, or mcpDays and llmDays if the plan doesn't set one.
func (s *Subscription) AuditLogRetention(mcpDays, llmDays int) gatewayclient.AuditLogRetention {
	return func(ctx context.Context) (int, int, error) {
		entitlements, err := s.Entitlements(ctx)
		if err != nil {
			return 0, 0, err
		}
		if days := entitlements.AuditLogRetentionDays; days != nil && *days > 0 {
			days := int(min(*days, maxAuditLogRetentionDays))
			return days, days, nil
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

// EntitlementsOf returns billing's last entitlements answer and whether the environment is restricted. Billing is
// always Obot Cloud. Before billing first answers, no plan applies, so every feature is on and nothing is limited.
func EntitlementsOf(status v1.BillingSubscriptionStatus) types.Entitlements {
	entitlements := types.Entitlements{
		EnterpriseAuthProviders:  true,
		EnterpriseModelProviders: true,
	}
	if status.Entitlements != nil {
		entitlements = *status.Entitlements.DeepCopy()
	}
	entitlements.Cloud = true
	entitlements.Restricted = status.Restricted
	return entitlements
}
