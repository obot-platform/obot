package server

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/entitlements"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

var (
	// deviceRequests are what managed devices send. They're allowed while restricted, so devices keep enforcing policy.
	deviceRequests = []string{
		"POST /api/local-agent-audit-logs",
		"POST /api/enforcement/decisions",
		"POST /api/mdm/enroll",
	}
)

func gatedPaths() *http.ServeMux {
	mux := http.NewServeMux()
	for _, path := range entitlements.GatedPaths {
		mux.Handle(path, http.NotFoundHandler())
	}
	return mux
}

// entitlementGate refuses:
// - gated paths while a configured provider needs a missing entitlement;
// - gated paths for everyone while the environment is restricted;
// - the rest of the API for non-admins while the environment is restricted.
// Admins and owners keep the API so they can get usage back under the plan.
func (s *Server) entitlementGate(req *http.Request, user kuser.Info) error {
	if s.entitlements == nil {
		return nil
	}

	_, pattern := s.gatedPaths.Handler(req)
	gated := pattern != ""
	if !gated && !s.restrictable(req, user) {
		return nil
	}

	restricted, err := s.entitlements.Restricted(req.Context())
	if err != nil {
		return fmt.Errorf("failed to check whether the environment is restricted: %w", err)
	}
	if restricted {
		return types.NewErrHTTP(http.StatusPaymentRequired, entitlements.RestrictedMessage)
	}
	if !gated {
		return nil
	}

	granted, err := s.entitlements.Entitlements(req.Context())
	if err != nil {
		return fmt.Errorf("failed to check provider license entitlements: %w", err)
	}
	violations, err := entitlements.ProviderViolations(req.Context(), s.storageClient, granted)
	if err != nil {
		return fmt.Errorf("failed to check provider license entitlements: %w", err)
	}
	if len(violations) > 0 {
		return types.NewErrHTTP(http.StatusPaymentRequired, "configured provider is missing required license entitlements")
	}
	return nil
}

// restrictable returns whether restriction applies to the request. Pages, sign out, password changes and the reads
// that explain the restriction are always allowed.
func (s *Server) restrictable(req *http.Request, user kuser.Info) bool {
	groups := user.GetGroups()
	return slices.Contains(groups, types.GroupAuthenticated) &&
		!slices.Contains(groups, types.GroupAdmin) &&
		!slices.Contains(groups, types.GroupOwner) &&
		req.Pattern != "/" &&
		!passwordChangeRequestAllowed(req, s.devUI) &&
		!slices.Contains(deviceRequests, req.Method+" "+req.URL.Path)
}
