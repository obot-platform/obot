package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/acorn-io/baaah/pkg/fields"
	"github.com/gptscript-ai/otto/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ conditions.Conditions = (*Webhook)(nil)
	_ fields.Fields         = (*Webhook)(nil)
	_ conditions.Conditions = (*WebhookExecution)(nil)
	_ fields.Fields         = (*WebhookExecution)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Webhook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookSpec   `json:"spec,omitempty"`
	Status WebhookStatus `json:"status,omitempty"`
}

func (w *Webhook) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(Workflow), Name: w.Spec.WorkflowName},
	}
}

func (w *Webhook) GetConditions() *[]metav1.Condition {
	return &w.Status.Conditions
}

func (w *Webhook) Has(field string) bool {
	return w.Get(field) != ""
}

func (w *Webhook) Get(field string) string {
	if w != nil {
		switch field {
		case "spec.workflowName":
			return w.Spec.WorkflowName
		}
	}

	return ""
}

func (*Webhook) FieldNames() []string {
	return []string{"spec.workflowName"}
}

type WebhookSpec struct {
	types.WebhookManifest `json:",inline"`
}

type WebhookStatus struct {
	Conditions []metav1.Condition          `json:"conditions,omitempty"`
	External   types.WebhookExternalStatus `json:"external,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WebhookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Webhook `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WebhookExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookExecutionSpec   `json:"spec,omitempty"`
	Status WebhookExecutionStatus `json:"status,omitempty"`
}

func (w *WebhookExecution) DeleteRefs() []Ref {
	return []Ref{}
}

func (w *WebhookExecution) GetConditions() *[]metav1.Condition {
	return &w.Status.Conditions
}

func (w *WebhookExecution) Has(field string) bool {
	return w.Get(field) != ""
}

func (w *WebhookExecution) Get(field string) string {
	if w != nil {
		switch field {
		case "spec.webhookName":
			return w.Spec.WebhookName
		}
	}

	return ""
}

func (*WebhookExecution) FieldNames() []string {
	return []string{"spec.webhookName"}
}

type WebhookExecutionSpec struct {
	WebhookName string            `json:"webhookName,omitempty"`
	Payload     string            `json:"payload,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type WebhookState string

const (
	WebhookStatePending  WebhookState = "Pending"
	WebhookStateRunning  WebhookState = "Running"
	WebhookStateError    WebhookState = "Error"
	WebhookStateComplete WebhookState = "Complete"
)

type WebhookExecutionStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	Output     string             `json:"output,omitempty"`
	State      WebhookState       `json:"state,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WebhookExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebhookExecution `json:"items"`
}
