// Package subscription keeps a billed environment's Subscription and EntitlementUsage current. It runs on the leader
// only.
package subscription

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/billing"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// syncInterval is how often billing is asked again. A sync requested when an owner comes back from the billing
	// pages covers the rest.
	syncInterval  = 15 * time.Minute
	retryInterval = 5 * time.Minute
)

type Handler struct {
	billingClient *billing.Client
	gatewayClient *gateway.Client
}

func New(billingClient *billing.Client, gatewayClient *gateway.Client) *Handler {
	return &Handler{
		billingClient: billingClient,
		gatewayClient: gatewayClient,
	}
}

// EnsureResources creates the Subscription and the EntitlementUsage unless they exist.
func EnsureResources(ctx context.Context, c kclient.Client) error {
	for _, obj := range []kclient.Object{
		&v1.Subscription{
			Namespace: system.DefaultNamespace,
			Name:      system.SubscriptionName,
		},
		&v1.EntitlementUsage{
			Namespace: system.DefaultNamespace,
			Name:      system.EntitlementUsageName,
		},
	} {
		if err := kclient.IgnoreAlreadyExists(c.Create(ctx, obj)); err != nil {
			return fmt.Errorf("failed to create %s: %w", obj.GetName(), err)
		}
	}
	return nil
}

// Sync records billing's answer when a sync is due, or requested with SubscriptionSyncAnnotation. A failed sync keeps
// the last answer.
func (h *Handler) Sync(req router.Request, resp router.Response) error {
	subscription := req.Object.(*v1.Subscription)
	status := &subscription.Status
	requested := subscription.Annotations[v1.SubscriptionSyncAnnotation] == "true"
	if wait := time.Until(nextSync(subscription)); wait > 0 && !requested {
		// Not due yet, come back when it is.
		resp.RetryAfter(wait)
		return nil
	}

	answer, err := h.billingClient.FetchSubscription(req.Ctx)
	status.LastSyncTime = metav1.Now()
	if err != nil {
		slog.Error("Failed to sync the subscription with billing", "error", err)
		status.SyncError = err.Error()
	} else {
		// Keep the answer. One without a period end clears the last one's.
		status.Plan, status.Status = answer.Plan, answer.Status
		status.PeriodEnd = nil
		if !answer.PeriodEnd.IsZero() {
			status.PeriodEnd = &metav1.Time{Time: answer.PeriodEnd.Time}
		}
		status.CancelAtPeriodEnd, status.SelfService = answer.CancelAtPeriodEnd, answer.SelfService
		status.Entitlements = answer.Entitlements
		status.SyncError = ""
	}

	if requested {
		// Store the answer before clearing the request, so whoever waits for the request to clear finds the answer.
		if err := req.Client.Status().Update(req.Ctx, subscription); err != nil {
			return fmt.Errorf("failed to update the subscription's status: %w", err)
		}
		delete(subscription.Annotations, v1.SubscriptionSyncAnnotation)
		if err := req.Client.Update(req.Ctx, subscription); err != nil {
			return fmt.Errorf("failed to clear the subscription's sync request: %w", err)
		}
	}
	resp.RetryAfter(time.Until(nextSync(subscription)))
	return nil
}

// nextSync returns when the subscription is due to sync: syncInterval after the last one, or retryInterval after a
// failure.
func nextSync(subscription *v1.Subscription) time.Time {
	status := subscription.Status
	if status.SyncError != "" {
		return status.LastSyncTime.Add(retryInterval)
	}
	return status.LastSyncTime.Add(syncInterval)
}

// EnsureOverLimitSince starts the over-limit clock when usage is over the subscription's limits, and clears it once
// usage is back within them. Reading the EntitlementUsage through the controller's client makes it run again when
// compute changes. Users and devices are counted at each sync.
func (h *Handler) EnsureOverLimitSince(req router.Request, _ router.Response) error {
	subscription := req.Object.(*v1.Subscription)
	status := &subscription.Status
	// Without an EntitlementUsage, compute counts as 0.
	used, err := license.Usage(req.Ctx, h.gatewayClient, req.Client)
	if err != nil {
		// Leave the clock as it is. An error here would keep the Subscription's status from being saved, and
		// with it what Sync just recorded.
		slog.Error("Failed to count usage against the subscription's limits", "error", err)
		return nil
	}

	// Before billing first answers, there are no limits to be over.
	switch over := len(license.LimitViolations(subscription.Entitlements(), used)) > 0; {
	case !over:
		// Back within the limits, clear the clock.
		status.OverLimitSince = nil
	case status.OverLimitSince == nil:
		// Just went over, start the clock. If it's already running, leave it.
		now := metav1.Now()
		status.OverLimitSince = &now
	}
	return nil
}

// CountUsage keeps the EntitlementUsage's status current. Listing the objects that use compute through the
// controller's client makes it run again when any of them changes.
func (h *Handler) CountUsage(req router.Request, _ router.Response) error {
	usage := req.Object.(*v1.EntitlementUsage)
	compute, err := mcp.ListCompute(req.Ctx, req.Client)
	if err != nil {
		return err
	}
	used, err := compute.Usage()
	if err != nil {
		return err
	}
	usage.Status.MemoryMiB, usage.Status.CPUMillicores = used.MemoryMiB, used.CPUMillicores
	return nil
}
