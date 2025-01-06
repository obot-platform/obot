package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeSummary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KnowledgeSummarySpec   `json:"spec,omitempty"`
	Status KnowledgeSummaryStatus `json:"status,omitempty"`
}

type KnowledgeSummarySpec struct {
	ThreadName  string `json:"threadName,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`
	Summary     []byte `json:"summary,omitempty"`
}

func (in *KnowledgeSummary) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Name},
	}
}

type KnowledgeSummaryStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeSummaryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []KnowledgeSummary `json:"items"`
}
