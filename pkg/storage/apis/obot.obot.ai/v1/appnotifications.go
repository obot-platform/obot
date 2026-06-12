package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AppNotifications struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppNotificationsSpec   `json:"spec,omitempty"`
	Status AppNotificationsStatus `json:"status,omitempty"`
}

type AppNotificationsSpec struct {
	Banner types.BannerNotification `json:"banner,omitempty"`
}

type AppNotificationsStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AppNotificationsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AppNotifications `json:"items"`
}
