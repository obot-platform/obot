package handlers

import (
	"errors"
	"net/http"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/billing"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// BillingHandler serves the license routes instead of LicenseHandler while billing is on.
type BillingHandler struct {
	billing   *billing.Billing
	returnURL string
}

func NewBillingHandler(billing *billing.Billing, serverURL string) *BillingHandler {
	return &BillingHandler{
		billing:   billing,
		returnURL: serverURL + "/admin/platform?billing=returned",
	}
}

func (h *BillingHandler) Get(req api.Context) error {
	status, err := h.status(req)
	if err != nil {
		return err
	}
	return req.Write(status)
}

// Check asks the leader to sync with billing. It returns right away, with Syncing set until the sync is done.
func (h *BillingHandler) Check(req api.Context) error {
	if err := h.billing.RequestSync(req.Context()); err != nil {
		return err
	}
	return h.Get(req)
}

// Manage opens the billing pages for an owner. Billing sends them back to the platform page when they're done.
func (h *BillingHandler) Manage(req api.Context) error {
	session, err := h.billing.CreateSession(req.Context(), h.returnURL)
	if billingErr, ok := errors.AsType[*billing.Error](err); ok {
		// Billing returns a conflict for a closed environment or a plan without billing pages. Show its message as is.
		if billingErr.Status == http.StatusConflict {
			return apitypes.NewErrHTTP(http.StatusConflict, billingErr.Message)
		}
		return apitypes.NewErrHTTP(http.StatusBadGateway, "billing refused to open the billing pages: "+billingErr.Message)
	} else if err != nil {
		return apitypes.NewErrHTTP(http.StatusBadGateway, "couldn't reach billing: "+err.Error())
	}
	return req.WriteCreated(session)
}

func (h *BillingHandler) status(req api.Context) (LicenseStatus, error) {
	subscription, err := h.billing.Read(req.Context())
	if err != nil {
		return LicenseStatus{}, err
	}

	status := subscription.Status
	licenseStatus := LicenseStatus{
		Entitlements: billing.EntitlementsOf(status),
		Usage:        status.Usage,
		Locked:       true,
		Syncing:      subscription.Annotations[v1.BillingSyncAnnotation] == "true",
	}
	// Only admins and auditors see the subscription.
	if req.UserIsAdmin() || req.UserIsAuditor() {
		licenseStatus.Subscription = billing.SubscriptionOf(status)
	}
	return licenseStatus, nil
}
