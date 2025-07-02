package v1

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPSession struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`
	Spec          MCPSessionSpec   `json:"spec"`
	Status        MCPSessionStatus `json:"status"`
}

type MCPSessionSpec struct {
	State []byte `json:"state"`
}

type MCPSessionStatus struct {
	LastUsedTime v1.Time `json:"lastUsedTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPSessionList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []MCPSession `json:"items"`
}
