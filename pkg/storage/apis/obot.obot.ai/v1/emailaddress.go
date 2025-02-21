package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ Aliasable     = (*EmailReceiver)(nil)
	_ Generationed  = (*EmailReceiver)(nil)
	_ fields.Fields = (*EmailReceiver)(nil)
	_ DeleteRefs    = (*EmailReceiver)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EmailReceiver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EmailReceiverSpec   `json:"spec,omitempty"`
	Status EmailReceiverStatus `json:"status,omitempty"`
}

func (in *EmailReceiver) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *EmailReceiver) Get(field string) (value string) {
	switch field {
	case "spec.threadName":
		return in.Spec.ThreadName
	case "spec.workflowName":
		return in.Spec.WorkflowName
	}
	return ""
}

func (in *EmailReceiver) FieldNames() []string {
	return []string{"spec.threadName", "spec.workflowName"}
}

func (in *EmailReceiver) GetAliasName() string {
	return in.Spec.EmailReceiverManifest.Alias
}

func (in *EmailReceiver) SetAssigned(assigned bool) {
	in.Status.AliasAssigned = assigned
}

func (in *EmailReceiver) IsAssigned() bool {
	return in.Status.AliasAssigned
}

func (in *EmailReceiver) GetObservedGeneration() int64 {
	return in.Status.ObservedGeneration
}

func (in *EmailReceiver) SetObservedGeneration(gen int64) {
	in.Status.ObservedGeneration = gen
}

func (*EmailReceiver) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Alias", "Spec.Alias"},
		{"Workflow", "Spec.Workflow"},
		{"Created", "{{ago .CreationTimestamp}}"},
		{"Description", "Spec.Description"},
	}
}

func (in *EmailReceiver) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Workflow{}, Name: in.Spec.WorkflowName},
	}
}

type EmailReceiverSpec struct {
	types.EmailReceiverManifest `json:",inline"`
	ThreadName                  string `json:"threadName,omitempty"`
}

type EmailReceiverStatus struct {
	AliasAssigned      bool  `json:"aliasAssigned,omitempty"`
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EmailReceiverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EmailReceiver `json:"items"`
}
