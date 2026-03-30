package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MessagePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MessagePolicySpec `json:"spec,omitempty"`
	Status EmptyStatus       `json:"status,omitempty"`
}

type MessagePolicySpec struct {
	Manifest types.MessagePolicyManifest `json:"manifest"`
}

func (in *MessagePolicy) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.DisplayName"},
		{"Direction", "Spec.Manifest.Direction"},
		{"Subjects", "{{len .Spec.Manifest.Subjects}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MessagePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MessagePolicy `json:"items"`
}
