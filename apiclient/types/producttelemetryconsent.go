package types

// ProductTelemetryConsent is the installation-wide product telemetry consent state.
// A nil Consent means that no administrator has made a decision yet.
type ProductTelemetryConsent struct {
	Consent *bool `json:"consent,omitempty"`
}

// ProductTelemetryConsentUpdate is the request for changing product telemetry consent.
// Consent is a pointer so the API can reject a missing or null value.
type ProductTelemetryConsentUpdate struct {
	Consent *bool `json:"consent"`
}
