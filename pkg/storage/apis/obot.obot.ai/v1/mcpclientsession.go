package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MCPClientSession associates an MCP session with the client that initialized it.
type MCPClientSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              MCPClientSessionSpec   `json:"spec"`
	Status            MCPClientSessionStatus `json:"status"`
}

type MCPClientSessionSpec struct {
	MCPServerID   string `json:"mcpServerID"`
	UserID        string `json:"userID"`
	ClientName    string `json:"clientName"`
	ClientVersion string `json:"clientVersion"`
}

type MCPClientSessionStatus struct {
	LastUsed metav1.Time `json:"lastUsed"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPClientSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []MCPClientSession `json:"items"`
}
