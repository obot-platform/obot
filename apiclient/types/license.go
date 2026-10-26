package types

// Entitlements is what the environment may do: its features and limits. A nil limit is unlimited, and a limit of 0
// allows none.
type Entitlements struct {
	Community                bool `json:"community"`
	Enterprise               bool `json:"enterprise"`
	Cloud                    bool `json:"cloud"`
	EnterpriseAuthProviders  bool `json:"enterpriseAuthProviders"`
	EnterpriseModelProviders bool `json:"enterpriseModelProviders"`
	// Restricted is set while the environment is past its plan. Only admins and owners can use it then.
	Restricted            bool   `json:"restricted,omitempty"`
	Users                 *int64 `json:"users"`
	Devices               *int64 `json:"devices"`
	MemoryMiB             *int64 `json:"memoryMiB"`
	CPUMillicores         *int64 `json:"cpuMillicores"`
	AuditLogRetentionDays *int64 `json:"auditLogRetentionDays"` // a setting; nil or 0 keeps Obot's configuration
}

// EntitlementsUsage is what the environment uses against its limits, in the same units.
type EntitlementsUsage struct {
	Users         int64 `json:"users"`
	Devices       int64 `json:"devices"`
	MemoryMiB     int64 `json:"memoryMiB"`
	CPUMillicores int64 `json:"cpuMillicores"`
}

// SubscriptionStatus is trialing, active, pastDue or ended.
type SubscriptionStatus string

const (
	SubscriptionStatusTrialing SubscriptionStatus = "trialing"
	SubscriptionStatusActive   SubscriptionStatus = "active"
	SubscriptionStatusPastDue  SubscriptionStatus = "pastDue"
	SubscriptionStatusEnded    SubscriptionStatus = "ended"
)

// Subscription is the environment's subscription as billing answers it, apart from its entitlements.
type Subscription struct {
	Plan              string             `json:"plan"` // shown, never branched on
	Status            SubscriptionStatus `json:"status"`
	PeriodEnd         *Time              `json:"periodEnd,omitempty"`
	CancelAtPeriodEnd bool               `json:"cancelAtPeriodEnd"`
	SelfService       bool               `json:"selfService"`
}

// BillingSession is a one-time link to the billing pages that works until ExpiresAt.
type BillingSession struct {
	URL       string `json:"url"`
	ExpiresAt Time   `json:"expiresAt"`
}
