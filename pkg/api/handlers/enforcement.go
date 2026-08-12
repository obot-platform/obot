package handlers

import (
	"github.com/obot-platform/obot/pkg/api"
)

const enforcementCompatibilityReason = "legacy allowlist enforcement has been retired"

type enforcementCompatibilityResponse struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// AllowLegacyDecision handles POST /api/enforcement/decisions for older Obot
// Sentry clients. Authentication is still enforced by the router's authz
// middleware, but the body is deliberately neither parsed nor evaluated: every
// legacy hook must fail open while devices converge away from allowlist hooks.
// This compatibility path performs no database reads or writes.
func AllowLegacyDecision(req api.Context) error {
	return req.Write(enforcementCompatibilityResponse{
		Decision: "allow",
		Reason:   enforcementCompatibilityReason,
	})
}
