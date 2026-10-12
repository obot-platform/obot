package billing

import (
	"strconv"
	"time"

	"github.com/obot-platform/obot/pkg/license"
)

// Subscription statuses the billing service reports. Every other status, and any
// status this list does not cover, is treated as live.
const (
	StatusTrialing          = "trialing"
	StatusActive            = "active"
	StatusPastDue           = "past_due"
	StatusIncomplete        = "incomplete"
	StatusUnpaid            = "unpaid"
	StatusCanceled          = "canceled"
	StatusIncompleteExpired = "incomplete_expired"
)

// Entitlements is the billing service's answer to GET /v1/entitlements. The
// service reports plain numbers and knows nothing about how Obot expresses
// limits.
type Entitlements struct {
	Plan                  string     `json:"plan"`
	Status                string     `json:"status"`
	Seats                 int64      `json:"seats"`
	HostedMCPServers      int64      `json:"hostedMcpServers"`
	AuditLogRetentionDays int64      `json:"auditLogRetentionDays"`
	PeriodEnd             *time.Time `json:"periodEnd"`
	CancelAtPeriodEnd     bool       `json:"cancelAtPeriodEnd"`
	DeleteAfter           *time.Time `json:"deleteAfter"`
}

// Names converts the numbers the billing service reports into the entitlement
// names Obot's limit parsing understands, so a limit from a subscription and a
// limit from a license key are read by the same code. The billing service never
// sees these names.
func (e *Entitlements) Names() []string {
	if e == nil {
		return nil
	}

	var names []string
	for _, limit := range []struct {
		value  int64
		suffix string
	}{
		{
			value:  e.Seats,
			suffix: license.UsersEntitlementSuffix,
		},
		{
			value:  e.HostedMCPServers,
			suffix: license.HostedMCPServersEntitlementSuffix,
		},
		{
			value:  e.AuditLogRetentionDays,
			suffix: license.AuditLogDaysEntitlementSuffix,
		},
	} {
		if limit.value <= 0 {
			continue
		}
		names = append(names, license.LimitEntitlementPrefix+strconv.FormatInt(limit.value, 10)+limit.suffix)
	}

	return names
}

// Ended reports whether the subscription is over. Unrecognized statuses are live,
// so a status the billing service has not classified can never end an
// environment.
func (e *Entitlements) Ended() bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case StatusUnpaid, StatusCanceled, StatusIncompleteExpired:
		return true
	default:
		return false
	}
}

// PaymentRequired reports whether the owner has an outstanding payment to make.
func (e *Entitlements) PaymentRequired() bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case StatusPastDue, StatusIncomplete, StatusUnpaid:
		return true
	default:
		return false
	}
}
