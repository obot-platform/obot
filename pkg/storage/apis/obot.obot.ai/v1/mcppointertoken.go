package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

var _ DeleteRefs = (*MCPPointerToken)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPPointerToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MCPPointerTokenSpec   `json:"spec,omitempty"`
	Status            MCPPointerTokenStatus `json:"status,omitempty"`
}

func (in *MCPPointerToken) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(MCPServerConfig), Name: in.Spec.Resource},
	}
}

type MCPPointerTokenSpec struct {
	Resource string `json:"resources,omitempty"`
}

type MCPPointerTokenStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPPointerTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPPointerToken `json:"items"`
}
