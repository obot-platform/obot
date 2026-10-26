// Package billingsubscription syncs the BillingSubscription with billing and decides when the environment is
// restricted. It runs on the leader only.
package billingsubscription

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/billing"
	"github.com/obot-platform/obot/pkg/entitlements"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// syncInterval matches how long billing takes to catch up with Stripe.
	syncInterval  = 15 * time.Minute
	retryInterval = 5 * time.Minute
)

// Handler syncs the BillingSubscription on the leader.
type Handler struct {
	client        *billing.Client
	gatewayClient *gatewayclient.Client
}

func New(client *billing.Client, gatewayClient *gatewayclient.Client) *Handler {
	return &Handler{
		client:        client,
		gatewayClient: gatewayClient,
	}
}

// Sync syncs with billing when it's due or requested, then counts usage and decides whether the environment is
// restricted. A failed sync keeps billing's last answer. It runs again when the next sync is due, and when the objects
// usage is counted from change, since counting them registers them as triggers. Users and devices are recounted at
// the next sync.
func (h *Handler) Sync(req router.Request, resp router.Response) error {
	if req.Object == nil {
		// Recreate the BillingSubscription if it was deleted.
		return billing.EnsureSubscription(req.Ctx, req.Client)
	}

	subscription := req.Object.(*v1.BillingSubscription)
	status := subscription.Status
	now := time.Now()

	forceSync := subscription.Annotations[v1.BillingSyncAnnotation] == "true"
	syncing := forceSync || syncDue(status, now)
	if syncing {
		plan, granted, err := h.client.Subscription(req.Ctx)
		if err != nil {
			slog.Error("failed to sync the billing subscription", "error", err)
		}
		status = synced(status, plan, granted, err, now)
	}

	// If usage can't be counted, leave the last count and the over-limit clock alone.
	if usage, err := h.usage(req, status.Usage, syncing); err != nil {
		slog.Error("failed to count usage against the plan", "error", err)
	} else {
		status = tracked(status, usage, now)
	}
	status.Restricted = restricted(status, now)
	resp.RetryAfter(nextSync(status).Sub(now))

	if changed, err := statusChanged(subscription.Status, status); err != nil {
		return err
	} else if changed {
		subscription.Status = status
		if err := req.Client.Status().Update(req.Ctx, subscription); err != nil {
			return err
		}
	}
	if forceSync {
		delete(subscription.Annotations, v1.BillingSyncAnnotation)
		return req.Client.Update(req.Ctx, subscription)
	}
	return nil
}

// usage counts what hosted MCP servers request. Users and devices are counted only when syncing, and otherwise kept
// from last, since their changes don't trigger the handler.
func (h *Handler) usage(req router.Request, last types.EntitlementsUsage, syncing bool) (types.EntitlementsUsage, error) {
	usage, err := billing.Usage(req.Ctx, req.Client)
	if err != nil {
		return types.EntitlementsUsage{}, err
	}
	if !syncing {
		usage.Users, usage.Devices = last.Users, last.Devices
		return usage, nil
	}
	if usage.Users, err = h.gatewayClient.UserCount(req.Ctx); err != nil {
		return types.EntitlementsUsage{}, err
	}
	if usage.Devices, err = h.gatewayClient.DeviceCount(req.Ctx); err != nil {
		return types.EntitlementsUsage{}, err
	}
	return usage, nil
}

// statusChanged compares the statuses as JSON, since equality.Semantic can't compare API times.
func statusChanged(before, after v1.BillingSubscriptionStatus) (bool, error) {
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return false, err
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return false, err
	}
	return !bytes.Equal(beforeJSON, afterJSON), nil
}

// syncDue returns whether it's time to sync.
func syncDue(status v1.BillingSubscriptionStatus, now time.Time) bool {
	return !now.Before(nextSync(status))
}

// nextSync returns when the next sync is due: syncInterval after the last sync, or retryInterval after a failure. An
// environment that never synced is long due.
func nextSync(status v1.BillingSubscriptionStatus) time.Time {
	interval := syncInterval
	if status.SyncError != "" {
		interval = retryInterval
	}
	return status.LastSyncTime.Add(interval)
}

// synced records a sync's result. A failed sync keeps billing's last answer.
func synced(status v1.BillingSubscriptionStatus, plan types.Subscription, granted types.Entitlements, err error, now time.Time) v1.BillingSubscriptionStatus {
	status.LastSyncTime = metav1.NewTime(now)
	if err != nil {
		status.SyncError = err.Error()
		return status
	}
	status.Subscription = &v1.BillingSubscriptionPlan{
		Plan:              plan.Plan,
		Status:            plan.Status,
		CancelAtPeriodEnd: plan.CancelAtPeriodEnd,
		SelfService:       plan.SelfService,
	}
	if plan.PeriodEnd != nil {
		status.Subscription.PeriodEnd = &metav1.Time{Time: plan.PeriodEnd.Time}
	}
	status.Entitlements = &granted
	status.SyncError = ""
	return status
}

// tracked records usage, and starts the over-limit clock when usage goes over the plan or clears it once usage is
// back under.
func tracked(status v1.BillingSubscriptionStatus, usage types.EntitlementsUsage, now time.Time) v1.BillingSubscriptionStatus {
	status.Usage = usage

	over := len(entitlements.LimitViolations(billing.EntitlementsOf(status), usage)) > 0
	switch {
	case !over:
		status.OverLimitSince = nil
	case status.OverLimitSince == nil:
		since := metav1.NewTime(now)
		status.OverLimitSince = &since
	}
	return status
}

// restricted returns whether the environment is restricted: it's been over its plan's limits for GracePeriod, or
// billing has been unreachable for GracePeriod past the subscription's last known period end. A renewal moves the
// period end only once billing answers, so the grace keeps an outage at renewal from restricting a paid environment.
// It's never restricted before billing first answers.
func restricted(status v1.BillingSubscriptionStatus, now time.Time) bool {
	lapsed := status.SyncError != "" && status.Subscription != nil && status.Subscription.PeriodEnd != nil &&
		!now.Before(status.Subscription.PeriodEnd.Add(billing.GracePeriod))
	return lapsed || (status.OverLimitSince != nil && !now.Before(status.OverLimitSince.Add(billing.GracePeriod)))
}
