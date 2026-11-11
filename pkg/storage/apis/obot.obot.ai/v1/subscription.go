package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Subscription is the environment's obot-billing subscription. There's one, default/subscription, and only while
// billing is on. The leader writes its status, and syncs it right away when SubscriptionSyncAnnotation is set.
type Subscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   SubscriptionSpec   `json:"spec"`
	Status SubscriptionStatus `json:"status"`
}

type SubscriptionSpec struct{}

type SubscriptionStatus struct {
	// Plan through Entitlements are billing's last answer, as a types.Subscription has them. They're empty until
	// billing first answers, and a failed sync keeps them.
	Plan              string             `json:"plan,omitempty"`
	Status            string             `json:"status,omitempty"`
	PeriodEnd         *metav1.Time       `json:"periodEnd,omitempty"`
	CancelAtPeriodEnd bool               `json:"cancelAtPeriodEnd,omitempty"`
	SelfService       bool               `json:"selfService,omitempty"`
	Entitlements      types.Entitlements `json:"entitlements,omitzero"`
	// LastSyncTime is when the leader last tried to sync, and SyncError is why that try failed.
	LastSyncTime metav1.Time `json:"lastSyncTime,omitzero"`
	SyncError    string      `json:"syncError,omitempty"`
	// OverLimitSince is when usage went over the subscription's limits, or nil while it's within them.
	OverLimitSince *metav1.Time `json:"overLimitSince,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Subscription `json:"items"`
}

// Entitlements are what the subscription grants: billing's last answer, on Obot Cloud. Before billing first answers,
// they're empty: no features and no limits, so the first owner can sign in.
func (in *Subscription) Entitlements() types.Entitlements {
	entitlements := in.Status.Entitlements
	entitlements.Cloud = true
	return entitlements
}
