package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/acorn-io/baaah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*WebhookReference)(nil)
	_ fields.Fields         = (*WebhookReference)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WebhookReference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookReferenceSpec `json:"spec,omitempty"`
	Status ReferenceStatus      `json:"status,omitempty"`
}

func (in *WebhookReference) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *WebhookReference) Get(field string) string {
	if in != nil {
		switch field {
		case "spec.webhookNamespace":
			return in.Spec.WebhookNamespace
		case "spec.webhookName":
			return in.Spec.WebhookName
		}
	}
	return ""
}

func (*WebhookReference) FieldNames() []string {
	return []string{"spec.webhookNamespace", "spec.webhookName"}
}

func (*WebhookReference) NamespaceScoped() bool {
	return false
}

func (in *WebhookReference) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type WebhookReferenceSpec struct {
	WebhookNamespace string `json:"webhookNamespace,omitempty"`
	WebhookName      string `json:"webhookName,omitempty"`
	Custom           bool   `json:"custom,omitempty"`
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
