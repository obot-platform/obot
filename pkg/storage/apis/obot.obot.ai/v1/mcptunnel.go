package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const MCPTunnelCredentialIDField = "spec.credentialID"

var _ fields.Fields = (*MCPTunnel)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPTunnel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MCPTunnelSpec `json:"spec,omitempty"`
}

func (in *MCPTunnel) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *MCPTunnel) Get(field string) string {
	if field == MCPTunnelCredentialIDField {
		return in.Spec.CredentialID
	}
	return ""
}

func (*MCPTunnel) FieldNames() []string {
	return []string{MCPTunnelCredentialIDField}
}

type MCPTunnelSpec struct {
	Manifest     types.MCPTunnelManifest `json:"manifest"`
	Credential   string                  `json:"credential"`
	CredentialID string                  `json:"credentialID"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPTunnelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPTunnel `json:"items"`
}
