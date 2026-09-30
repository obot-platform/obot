package billing

import (
	"context"
	"slices"
	"strings"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
)

// EntitlementSource reports the entitlements a subscription currently grants.
type EntitlementSource interface {
	Entitlements() *Entitlements
}

// Limits resolves each resource limit from the subscription where the
// subscription defines one and from the license key everywhere else. The two
// sources are never added together.
type Limits struct {
	subscription EntitlementSource
	license      license.LimitProvider
}

// NewLimits creates a limit provider backed by a subscription and a license key.
func NewLimits(subscription EntitlementSource, licenseLimits license.LimitProvider) *Limits {
	return &Limits{
		subscription: subscription,
		license:      licenseLimits,
	}
}

func (l *Limits) UserLimit(ctx context.Context) (gatewayclient.UserLimit, error) {
	maximum, unlimited, ok := l.subscribed(license.UsersEntitlementSuffix, gatewayclient.DefaultUserLimit)
	if !ok {
		return l.license.UserLimit(ctx)
	}
	return gatewayclient.UserLimit{
		Maximum:   maximum,
		Unlimited: unlimited,
	}, nil
}

// DeviceLimit always comes from the license key. A subscription does not sell
// devices.
func (l *Limits) DeviceLimit(ctx context.Context) (gatewayclient.DeviceLimit, error) {
	return l.license.DeviceLimit(ctx)
}

func (l *Limits) HostedMCPServerLimit(ctx context.Context) (gatewayclient.HostedMCPServerLimit, error) {
	maximum, unlimited, ok := l.subscribed(license.HostedMCPServersEntitlementSuffix, gatewayclient.DefaultHostedMCPServerLimit)
	if !ok {
		return l.license.HostedMCPServerLimit(ctx)
	}
	return gatewayclient.HostedMCPServerLimit{
		Maximum:   maximum,
		Unlimited: unlimited,
	}, nil
}

func (l *Limits) AuditLogRetention(ctx context.Context) (gatewayclient.AuditLogRetention, error) {
	days, unlimited, ok := l.subscribed(license.AuditLogDaysEntitlementSuffix, gatewayclient.DefaultAuditLogRetentionDays)
	if !ok {
		return l.license.AuditLogRetention(ctx)
	}
	return gatewayclient.AuditLogRetention{
		Days:      days,
		Unlimited: unlimited,
	}, nil
}

// subscribed resolves a limit the subscription defines, reporting false when it
// defines none.
func (l *Limits) subscribed(entitlementSuffix string, defaultMaximum int64) (int64, bool, bool) {
	names := l.subscription.Entitlements().Names()
	if !slices.ContainsFunc(names, func(name string) bool {
		return strings.HasSuffix(name, entitlementSuffix)
	}) {
		return 0, false, false
	}

	maximum, unlimited := license.ResourceLimit(names, entitlementSuffix, defaultMaximum)
	return maximum, unlimited, true
}
