package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*WebhookReference)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WebhookReference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookReferenceSpec `json:"spec,omitempty"`
	Status ReferenceStatus      `json:"status,omitempty"`
}

func (in *WebhookReference) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type WebhookReferenceSpec struct {
	WebhookName string `json:"webhookName,omitempty"`
}

func (in *WebhookReference) DeleteRefs() []Ref {
	return []Ref{}
}

type WebhookReferenceStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WebhookReferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []WebhookReference `json:"items"`
}
