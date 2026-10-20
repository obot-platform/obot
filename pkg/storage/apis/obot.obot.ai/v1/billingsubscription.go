package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BillingSubscription holds billing's last answer and the usage and restriction the leader derives from it. There's
// one, default/billing, while billing is on. Only the leader writes its status.
type BillingSubscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Status BillingSubscriptionStatus `json:"status"`
}

type BillingSubscriptionStatus struct {
	// Subscription and Entitlements are billing's last answer, and are nil until it first answers.
	Subscription *BillingSubscriptionPlan `json:"subscription,omitempty"`
	Entitlements *types.Entitlements      `json:"entitlements,omitempty"`
	// Usage is the usage the leader last counted.
	Usage types.EntitlementsUsage `json:"usage"`
	// LastSyncTime is when the leader last tried to sync.
	LastSyncTime metav1.Time `json:"lastSyncTime,omitzero"`
	// SyncError is why the last sync failed.
	SyncError string `json:"syncError,omitempty"`
	// OverLimitSince is when usage went over the plan's limits, or nil while it's under them.
	OverLimitSince *metav1.Time `json:"overLimitSince,omitempty"`
	// Restricted is set by the leader after GracePeriod over the limits.
	Restricted bool `json:"restricted,omitempty"`
}

// BillingSubscriptionPlan is billing's subscription answer, without the entitlements.
type BillingSubscriptionPlan struct {
	Plan              string                   `json:"plan"`
	Status            types.SubscriptionStatus `json:"status"`
	PeriodEnd         *metav1.Time             `json:"periodEnd,omitempty"`
	CancelAtPeriodEnd bool                     `json:"cancelAtPeriodEnd"`
	SelfService       bool                     `json:"selfService"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BillingSubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []BillingSubscription `json:"items"`
}
