package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MemorySet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemorySetSpec   `json:"spec,omitempty"`
	Status MemorySetStatus `json:"status,omitempty"`
}

type MemorySetSpec struct {
	ThreadName string                  `json:"threadName,omitempty"`
	Manifest   types.MemorySetManifest `json:"manifest,omitempty"`
}

func (in *MemorySet) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
	}
}

type MemorySetStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MemorySetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MemorySet `json:"items"`
}
