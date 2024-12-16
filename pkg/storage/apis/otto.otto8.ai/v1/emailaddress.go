package v1

import (
	"slices"

	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/acorn-io/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ Aliasable     = (*EmailReceiver)(nil)
	_ fields.Fields = (*EmailReceiver)(nil)
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
	}
	return ""
}

func (in *EmailReceiver) FieldNames() []string {
	return []string{"spec.threadName"}
}

func (in *EmailReceiver) GetAliasName() string {
	return in.Spec.EmailReceiverManifest.User
}

func (in *EmailReceiver) SetAssigned(assigned bool) {
	in.Status.AliasAssigned = assigned
}

func (in *EmailReceiver) IsAssigned() bool {
	return in.Status.AliasAssigned
}

func (in *EmailReceiver) GetAliasObservedGeneration() int64 {
	return in.Status.AliasObservedGeneration
}

func (in *EmailReceiver) SetAliasObservedGeneration(gen int64) {
	in.Status.AliasObservedGeneration = gen
}

func (*EmailReceiver) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"User", "Spec.User"},
		{"Workflow", "Spec.Workflow"},
		{"Created", "{{ago .CreationTimestamp}}"},
		{"Description", "Spec.Description"},
	}
}

func (in *EmailReceiver) DeleteRefs() []Ref {
	if system.IsWorkflowID(in.Spec.Workflow) {
		return []Ref{
			{ObjType: new(Workflow), Name: in.Spec.Workflow},
		}
	}
	return nil
}

type EmailReceiverSpec struct {
	types.EmailReceiverManifest `json:",inline"`
	ThreadName                  string `json:"threadName,omitempty"`
}

type EmailReceiverStatus struct {
	AliasAssigned           bool  `json:"aliasAssigned,omitempty"`
	AliasObservedGeneration int64 `json:"aliasProcessed,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EmailReceiverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EmailReceiver `json:"items"`
}
