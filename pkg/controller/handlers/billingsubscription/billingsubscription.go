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
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// syncInterval matches how long billing takes to catch up with Stripe.
	syncInterval  = 15 * time.Minute
	retryInterval = 5 * time.Minute
	// checkInterval is how often usage is recounted.
	checkInterval = time.Minute
)

// Handler syncs the BillingSubscription on the leader.
type Handler struct {
	client         *billing.Client
	gatewayClient  *gatewayclient.Client
	sessionManager *mcp.SessionManager
}

func New(client *billing.Client, gatewayClient *gatewayclient.Client, sessionManager *mcp.SessionManager) *Handler {
	return &Handler{
		client:         client,
		gatewayClient:  gatewayClient,
		sessionManager: sessionManager,
	}
}

// Sync syncs with billing when it's due or requested, then counts usage and decides whether the environment is
// restricted. A failed sync keeps billing's last answer.
func (h *Handler) Sync(req router.Request, resp router.Response) error {
	if req.Object == nil {
		// Recreate the BillingSubscription if it was deleted.
		return billing.EnsureSubscription(req.Ctx, req.Client)
	}

	subscription := req.Object.(*v1.BillingSubscription)
	status := subscription.Status
	now := time.Now()

	forceSync := subscription.Annotations[v1.BillingSyncAnnotation] == "true"
	if forceSync || syncDue(status, now) {
		plan, granted, err := h.client.Subscription(req.Ctx)
		if err != nil {
			slog.Error("failed to sync the billing subscription", "error", err)
		}
		status = synced(status, plan, granted, err, now)
	}

	// If usage can't be counted, leave the over-limit clock alone.
	if usage, err := h.usage(req); err != nil {
		slog.Error("failed to count usage against the plan", "error", err)
	} else {
		status = tracked(status, usage, now)
	}
	resp.RetryAfter(checkInterval)

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

// usage counts running hosted MCP servers, users and devices.
func (h *Handler) usage(req router.Request) (types.EntitlementsUsage, error) {
	usage, err := h.sessionManager.RunningUsage(req.Ctx)
	if err != nil {
		return types.EntitlementsUsage{}, err
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

// syncDue returns whether it's time to sync: never synced, syncInterval since the last sync, or retryInterval after a
// failure.
func syncDue(status v1.BillingSubscriptionStatus, now time.Time) bool {
	interval := syncInterval
	if status.SyncError != "" {
		interval = retryInterval
	}
	return status.LastSyncTime.IsZero() || now.Sub(status.LastSyncTime.Time) >= interval
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

// tracked records usage and decides whether the environment is restricted. It's restricted when it's been over its
// plan's limits for GracePeriod, or when billing can't be reached after the subscription's period ended. It's never
// restricted before billing first answers.
func tracked(status v1.BillingSubscriptionStatus, usage types.EntitlementsUsage, now time.Time) v1.BillingSubscriptionStatus {
	status.Usage = usage

	over := status.Entitlements != nil && len(entitlements.LimitViolations(billing.EntitlementsOf(status), usage)) > 0
	// Start the clock when usage goes over, and clear it once usage is back under.
	switch {
	case !over:
		status.OverLimitSince = nil
	case status.OverLimitSince == nil:
		since := metav1.NewTime(now)
		status.OverLimitSince = &since
	}

	lapsed := status.SyncError != "" && status.Subscription != nil && status.Subscription.PeriodEnd != nil &&
		now.After(status.Subscription.PeriodEnd.Time)
	status.Restricted = lapsed || (status.OverLimitSince != nil && !now.Before(status.OverLimitSince.Add(billing.GracePeriod)))
	return status
}
