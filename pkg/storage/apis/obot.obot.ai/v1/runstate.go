package v1

import (
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/name"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunStateSpec `json:"spec,omitempty"`
	Status EmptyStatus  `json:"status,omitempty"`
}

type RunStateSpec struct {
	ThreadName string `json:"threadName,omitempty"`
	Program    []byte `json:"program,omitempty"`
	ChatState  []byte `json:"chatState,omitempty"`
	CallFrame  []byte `json:"callFrame,omitempty"`
	Output     []byte `json:"output,omitempty"`
	Done       bool   `json:"done,omitempty"`
	Error      string `json:"error,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []RunState `json:"items"`
}

func RunStateNameWithExternalID(runName, externalCallID string) string {
	return name.SafeHashConcatName(runName, hash.Digest(externalCallID)[:12])
}
