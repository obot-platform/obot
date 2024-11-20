package v1

import (
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ Aliasable = (*EmailReceiver)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EmailReceiver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EmailReceiverSpec   `json:"spec,omitempty"`
	Status EmailReceiverStatus `json:"status,omitempty"`
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

func (*EmailReceiver) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"User", "Spec.User"},
		{"Workflow", "Spec.Workflow"},
		{"Created", "{{ago .CreationTimestamp}}"},
		{"Description", "Spec.Description"},
	}
}

func (w *EmailReceiver) DeleteRefs() []Ref {
	if system.IsWorkflowID(w.Spec.Workflow) {
		return []Ref{
			{ObjType: new(Workflow), Name: w.Spec.Workflow},
		}
	}
	return nil
}

type EmailReceiverSpec struct {
	types.EmailReceiverManifest `json:",inline"`
}

type EmailReceiverStatus struct {
	AliasAssigned bool `json:"aliasAssigned,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EmailReceiverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EmailReceiver `json:"items"`
}
