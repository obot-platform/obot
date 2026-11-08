package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MCPServerRequest is a user's request to add an MCP server to the catalog, which an
// administrator approves into a vMCP or denies.
type MCPServerRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   MCPServerRequestSpec   `json:"spec"`
	Status MCPServerRequestStatus `json:"status"`
}

type MCPServerRequestSpec struct {
	Manifest       types.MCPServerRequestManifest `json:"manifest"`
	RequesterID    string                         `json:"requesterID"`
	RequesterName  string                         `json:"requesterName,omitempty"`
	RequesterEmail string                         `json:"requesterEmail,omitempty"`
}

type MCPServerRequestStatus struct {
	State          types.MCPServerRequestState `json:"state,omitempty"`
	DecidedBy      string                      `json:"decidedBy,omitempty"`
	DecidedAt      *metav1.Time                `json:"decidedAt,omitempty"`
	DecisionReason string                      `json:"decisionReason,omitempty"`
	CatalogEntryID string                      `json:"catalogEntryID,omitempty"`
	VMCPID         string                      `json:"vmcpID,omitempty"`
	// Notified records that administrators were alerted about the request.
	Notified bool `json:"notified,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []MCPServerRequest `json:"items"`
}

func (in *MCPServerRequest) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Server", "Spec.Manifest.Name"},
		{"Requester", "Spec.RequesterID"},
		{"State", "Status.State"},
	}
}
