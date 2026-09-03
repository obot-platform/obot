package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/producttelemetry"
)

type ProductTelemetryConsentHandler struct {
	consent *producttelemetry.Consent
}

func NewProductTelemetryConsentHandler(consent *producttelemetry.Consent) *ProductTelemetryConsentHandler {
	return &ProductTelemetryConsentHandler{consent: consent}
}

func (h *ProductTelemetryConsentHandler) Get(req api.Context) error {
	// When consent is force-enabled (Obot Cloud), we respond 404 to signal that
	// this feature is not available
	if h.consent.ForceEnabled() {
		return types.NewErrNotFound("")
	}

	consent, err := h.consent.Get(req.Context())
	if err != nil {
		return err
	}
	return req.Write(types.ProductTelemetryConsent{Consent: consent})
}

func (h *ProductTelemetryConsentHandler) Update(req api.Context) error {
	// When consent is force-enabled (Obot Cloud), we respond 404 to signal that
	// this feature is not available
	if h.consent.ForceEnabled() {
		return types.NewErrNotFound("")
	}

	var input types.ProductTelemetryConsentUpdate
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}
	if input.Consent == nil {
		return types.NewErrBadRequest("consent is required")
	}

	if err := h.consent.Set(req.Context(), *input.Consent); err != nil {
		return err
	}
	return req.Write(types.ProductTelemetryConsent(input))
}
