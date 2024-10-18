package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*Webhook)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Webhook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookSpec   `json:"spec,omitempty"`
	Status WebhookStatus `json:"status,omitempty"`
}

func (*Webhook) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"RefName", "Spec.RefName"},
		{"Workflow", "Spec.WorkflowID"},
		{"Created", "{{ago .CreationTimestamp}}"},
		{"Last Success", "{{agoptr .Status.LastSuccessfulRunCompleted}}"},
		{"Description", "Spec.Description"},
	}
}

func (w *Webhook) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(Workflow), Name: w.Spec.WorkflowID},
	}
}

func (w *Webhook) GetConditions() *[]metav1.Condition {
	return &w.Status.Conditions
}

type WebhookSpec struct {
	types.WebhookManifest `json:",inline"`
}

type WebhookStatus struct {
	Conditions                 []metav1.Condition          `json:"conditions,omitempty"`
	External                   types.WebhookExternalStatus `json:"external,omitempty"`
	LastSuccessfulRunCompleted *metav1.Time                `json:"lastSuccessfulRunCompleted,omitempty"`
	PasswordHash               []byte                      `json:"passwordHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WebhookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Webhook `json:"items"`
}
