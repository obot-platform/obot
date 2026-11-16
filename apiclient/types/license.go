package types

// Entitlements is what the environment may do: its features and limits. A nil limit is unlimited, and a limit of 0
// allows none.
type Entitlements struct {
	Community                bool   `json:"community"`
	Enterprise               bool   `json:"enterprise"`
	Cloud                    bool   `json:"cloud"`
	EnterpriseAuthProviders  bool   `json:"enterpriseAuthProviders"`
	EnterpriseModelProviders bool   `json:"enterpriseModelProviders"`
	Users                    *int64 `json:"users"`
	Devices                  *int64 `json:"devices"`
	MemoryMiB                *int64 `json:"memoryMiB"`
	CPUMillicores            *int64 `json:"cpuMillicores"`
	AuditLogRetentionDays    *int64 `json:"auditLogRetentionDays"` // a setting; nil or 0 keeps Obot's configuration
	// AuditLogStorageMiB and NetworkEgressMiB, each billing period, aren't shown or enforced yet.
	AuditLogStorageMiB *int64 `json:"auditLogStorageMiB"`
	NetworkEgressMiB   *int64 `json:"networkEgressMiB"`
}

// EntitlementsUsage is what the environment uses against its limits, in the same units.
type EntitlementsUsage struct {
	Users         int64 `json:"users"`
	Devices       int64 `json:"devices"`
	MemoryMiB     int64 `json:"memoryMiB"`
	CPUMillicores int64 `json:"cpuMillicores"`
}

// Subscription is the environment's subscription as billing answers it.
type Subscription struct {
	Plan   string `json:"plan"`   // shown, never branched on
	Status string `json:"status"` // trialing, active, pastDue or ended
	// PeriodEnd is when the paid period ends, or the trial while trialing. An enterprise subscription has none.
	PeriodEnd         *Time        `json:"periodEnd,omitempty"`
	CancelAtPeriodEnd bool         `json:"cancelAtPeriodEnd"`
	SelfService       bool         `json:"selfService"`
	Entitlements      Entitlements `json:"entitlements,omitzero"`
}

// BillingSession is a one-time link to the billing pages.
type BillingSession struct {
	URL string `json:"url"`
}
