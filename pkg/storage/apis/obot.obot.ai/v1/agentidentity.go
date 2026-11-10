package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AgentIdentityChange reports that an agent identity was disabled or enabled, so that controllers re-evaluate what
// depends on its access. Its name comes from the gateway lifecycle event it delivers. The agent may have changed
// again since, so handlers must read its current status.
type AgentIdentityChange struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   AgentIdentityChangeSpec `json:"spec"`
	Status EmptyStatus             `json:"status"`
}

type AgentIdentityChangeSpec struct {
	AgentID uint `json:"agentID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentIdentityChangeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []AgentIdentityChange `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AgentIdentityDelete makes the controller remove what a deleted agent identity owns. Its name comes from the gateway
// lifecycle event it delivers.
type AgentIdentityDelete struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   AgentIdentityDeleteSpec `json:"spec"`
	Status EmptyStatus             `json:"status"`
}

type AgentIdentityDeleteSpec struct {
	AgentID uint `json:"agentID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentIdentityDeleteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []AgentIdentityDelete `json:"items"`
}
