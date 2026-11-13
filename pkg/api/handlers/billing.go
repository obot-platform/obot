package handlers

import (
	"errors"
	"net/http"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/billing"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BillingStatus is what a billed environment's subscription grants, and for admins and auditors, the subscription
// itself and what the environment uses of it.
type BillingStatus struct {
	Entitlements apitypes.Entitlements `json:"entitlements"`
	// Subscription leaves out its entitlements, which are the ones above.
	Usage        *apitypes.EntitlementsUsage `json:"usage,omitempty"`
	Subscription *apitypes.Subscription      `json:"subscription,omitempty"`
	// Syncing is true while a requested sync with billing runs. SyncError is why the last sync failed.
	Syncing   bool   `json:"syncing,omitempty"`
	SyncError string `json:"syncError,omitempty"`
}

// BillingHandler serves the license routes instead of LicenseHandler when the environment is billed.
type BillingHandler struct {
	billingClient *billing.Client
	returnURL     string
}

func NewBillingHandler(billingClient *billing.Client, serverURL string) *BillingHandler {
	return &BillingHandler{
		billingClient: billingClient,
		returnURL:     serverURL + "/admin/platform?billing=returned",
	}
}

func (h *BillingHandler) Get(req api.Context) error {
	// Before the leader creates the Subscription, it's empty.
	var subscription v1.Subscription
	if err := req.Get(&subscription, system.SubscriptionName); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	status := BillingStatus{
		Entitlements: subscription.Entitlements(),
		Syncing:      subscription.Annotations[v1.SubscriptionSyncAnnotation] == "true",
	}
	// Only admins and auditors see the usage, the subscription and why a sync failed.
	if req.UserIsAdmin() || req.UserIsAuditor() {
		used, err := license.Usage(req.Context(), req.GatewayClient, req.Storage)
		if err != nil {
			return err
		}
		status.Usage = &used
		if stored := subscription.Status; stored.Status != "" {
			// Billing has answered. What it grants is in Entitlements, so it isn't shown a second time.
			status.Subscription = &apitypes.Subscription{
				Plan:              stored.Plan,
				Status:            stored.Status,
				PeriodEnd:         v1.NewTime(stored.PeriodEnd),
				CancelAtPeriodEnd: stored.CancelAtPeriodEnd,
				SelfService:       stored.SelfService,
			}
		}
		status.SyncError = subscription.Status.SyncError
	}
	return req.Write(status)
}

// Check asks the leader to sync with billing. It returns right away, with Syncing set until the sync is done.
func (h *BillingHandler) Check(req api.Context) error {
	if err := billing.RequestSync(req.Context(), req.Storage); err != nil {
		return err
	}
	return h.Get(req)
}

// Manage opens the billing pages for an owner. Billing sends them back to the platform page when they're done.
func (h *BillingHandler) Manage(req api.Context) error {
	session, err := h.billingClient.CreateSession(req.Context(), h.returnURL)
	if billingErr, ok := errors.AsType[*billing.Error](err); ok && billingErr.Status == http.StatusConflict {
		// Billing returns a conflict for a closed environment or a plan without billing pages. Show its message as is.
		return apitypes.NewErrHTTP(http.StatusConflict, billingErr.Message)
	} else if err != nil {
		return apitypes.NewErrHTTP(http.StatusBadGateway, err.Error())
	}
	return req.WriteCreated(session)
}
