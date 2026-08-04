package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AppNotification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   AppNotificationSpec   `json:"spec"`
	Status AppNotificationStatus `json:"status"`
}

type AppNotificationSpec struct {
	Banner types.BannerNotification `json:"banner"`
	// Updated is set whenever the notification is updated after its initial creation.
	// When unset, the creation timestamp is used as the updated time.
	Updated metav1.Time `json:"updated"`
}

type AppNotificationStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AppNotificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []AppNotification `json:"items"`
}
