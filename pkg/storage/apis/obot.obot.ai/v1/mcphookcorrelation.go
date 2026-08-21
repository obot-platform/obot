package v1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MCPHookCorrelationTTL = 24 * time.Hour
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MCPHookCorrelation stores the protocol context needed to process a JSON-RPC
// response that may arrive at a different Obot replica from its request.
type MCPHookCorrelation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              MCPHookCorrelationSpec `json:"spec"`
}

type MCPHookCorrelationSpec struct {
	Method          string           `json:"method"`
	Name            string           `json:"name,omitempty"`
	RequestMutation *MCPHookMutation `json:"requestMutation,omitempty"`
	ExpiresAt       metav1.Time      `json:"expiresAt"`
}

type MCPHookMutation struct {
	Mutated bool     `json:"mutated"`
	Reasons []string `json:"reasons,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPHookCorrelationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []MCPHookCorrelation `json:"items"`
}
