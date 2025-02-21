package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ Aliasable     = (*Webhook)(nil)
	_ fields.Fields = (*Webhook)(nil)
	_ Generationed  = (*Webhook)(nil)
	_ DeleteRefs    = (*Webhook)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Webhook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookSpec   `json:"spec,omitempty"`
	Status WebhookStatus `json:"status,omitempty"`
}

func (w *Webhook) FieldNames() []string {
	return []string{"spec.threadName", "spec.workflowName"}
}

func (w *Webhook) Has(field string) (exists bool) {
	return slices.Contains(w.FieldNames(), field)
}

func (w *Webhook) Get(field string) (value string) {
	switch field {
	case "spec.threadName":
		return w.Spec.ThreadName
	case "spec.workflowName":
		return w.Spec.WorkflowName
	}
	return ""
}

func (w *Webhook) GetAliasName() string {
	return w.Spec.WebhookManifest.Alias
}

func (w *Webhook) SetAssigned(assigned bool) {
	w.Status.AliasAssigned = assigned
}

func (w *Webhook) IsAssigned() bool {
	return w.Status.AliasAssigned
}

func (w *Webhook) GetObservedGeneration() int64 {
	return w.Status.ObservedGeneration
}

func (w *Webhook) SetObservedGeneration(gen int64) {
	w.Status.ObservedGeneration = gen
}

func (*Webhook) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Alias", "Spec.Alias"},
		{"Workflow", "Spec.Workflow"},
		{"Created", "{{ago .CreationTimestamp}}"},
		{"Last Success", "{{ago .Status.LastSuccessfulRunCompleted}}"},
		{"Description", "Spec.Description"},
	}
}

func (w *Webhook) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Workflow{}, Name: w.Spec.WorkflowName},
	}
}

type WebhookSpec struct {
	types.WebhookManifest `json:",inline"`
	TokenHash             []byte `json:"tokenHash,omitempty"`
	ThreadName            string
}

type WebhookStatus struct {
	AliasAssigned              bool         `json:"aliasAssigned,omitempty"`
	LastSuccessfulRunCompleted *metav1.Time `json:"lastSuccessfulRunCompleted,omitempty"`
	ObservedGeneration         int64        `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WebhookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Webhook `json:"items"`
}
