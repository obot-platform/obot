package license

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// restrictionTTL bounds how stale a restriction decision can be. Restricted
	// mode is evaluated on every sign-in and on every gated request, so the
	// counts behind it are cached; the installation leaves restricted mode within
	// this interval of fitting again.
	restrictionTTL = 15 * time.Second
)

// ResourceCounter reports what an installation is currently using.
type ResourceCounter interface {
	UserCount(context.Context) (int64, error)
	HostedMCPServerCount(context.Context) (int64, error)
}

// Restriction describes whether an installation exceeds what it is entitled to,
// and why.
type Restriction struct {
	Restricted bool
	Violations []Violation
}

// Restrictor compares an installation's usage against its entitlements. It holds
// no state of its own beyond a short-lived cache, so an installation leaves
// restricted mode as soon as it fits again, whether usage came down or more was
// bought. Nothing is ever deleted to make it fit.
type Restrictor struct {
	enabled bool
	limits  LimitProvider
	counts  ResourceCounter

	lock     sync.Mutex
	cached   Restriction
	cachedAt time.Time
	now      func() time.Time
}

// NewRestrictor creates a restrictor. It is inert unless enforcement is enabled.
func NewRestrictor(enabled bool, limits LimitProvider, counts ResourceCounter) *Restrictor {
	return &Restrictor{
		enabled: enabled,
		limits:  limits,
		counts:  counts,
		now:     time.Now,
	}
}

// Enabled reports whether resource limits are enforced against existing usage.
func (r *Restrictor) Enabled() bool {
	return r != nil && r.enabled
}

// Restricted reports whether the installation is over any of its limits.
func (r *Restrictor) Restricted(ctx context.Context) (bool, error) {
	restriction, err := r.Restriction(ctx)
	if err != nil {
		return false, err
	}
	return restriction.Restricted, nil
}

// Restriction reports whether the installation is over any of its limits, and
// which limits those are.
func (r *Restrictor) Restriction(ctx context.Context) (Restriction, error) {
	if !r.Enabled() {
		return Restriction{}, nil
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.cachedAt.IsZero() && r.now().Sub(r.cachedAt) < restrictionTTL {
		return r.cached, nil
	}

	restriction, err := r.evaluate(ctx)
	if err != nil {
		return Restriction{}, err
	}

	r.cached = restriction
	r.cachedAt = r.now()
	return restriction, nil
}

func (r *Restrictor) evaluate(ctx context.Context) (Restriction, error) {
	var restriction Restriction

	userLimit, err := r.limits.UserLimit(ctx)
	if err != nil {
		return Restriction{}, fmt.Errorf("failed to check user limit: %w", err)
	}
	if !userLimit.Unlimited && userLimit.Maximum > 0 {
		userCount, err := r.counts.UserCount(ctx)
		if err != nil {
			return Restriction{}, fmt.Errorf("failed to check user count: %w", err)
		}
		if userCount > userLimit.Maximum {
			restriction.Violations = append(restriction.Violations, Violation{
				Type:    "userLimit",
				Message: fmt.Sprintf("user count (%d) exceeds maximum limit (%d)", userCount, userLimit.Maximum),
			})
		}
	}

	hostedMCPServerLimit, err := r.limits.HostedMCPServerLimit(ctx)
	if err != nil {
		return Restriction{}, fmt.Errorf("failed to check hosted MCP server limit: %w", err)
	}
	if hostedMCPServerLimit.Enforced() {
		hostedMCPServerCount, err := r.counts.HostedMCPServerCount(ctx)
		if err != nil {
			return Restriction{}, fmt.Errorf("failed to check hosted MCP server count: %w", err)
		}
		if hostedMCPServerCount > hostedMCPServerLimit.Maximum {
			restriction.Violations = append(restriction.Violations, Violation{
				Type:    "hostedMCPServerLimit",
				Message: fmt.Sprintf("hosted MCP server count (%d) exceeds maximum limit (%d)", hostedMCPServerCount, hostedMCPServerLimit.Maximum),
			})
		}
	}

	restriction.Restricted = len(restriction.Violations) > 0
	return restriction, nil
}
