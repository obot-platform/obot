package v1

import (
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ DeleteRefs    = (*OAuthClient)(nil)
	_ fields.Fields = (*OAuthClient)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthClientSpec   `json:"spec,omitempty"`
	Status            OAuthClientStatus `json:"status,omitempty"`
}

func (o *OAuthClient) DeleteRefs() []Ref {
	if system.IsSystemMCPServerID(o.Spec.MCPServerName) {
		return []Ref{
			{
				ObjType: &SystemMCPServer{},
				Name:    o.Spec.MCPServerName,
			},
		}
	}
	return []Ref{
		{
			ObjType: &MCPServer{},
			Name:    o.Spec.MCPServerName,
		},
	}
}

func (o *OAuthClient) Has(field string) bool {
	return slices.Contains(o.FieldNames(), field)
}

func (o *OAuthClient) Get(field string) (value string) {
	switch field {
	case "spec.mcpServerName":
		return o.Spec.MCPServerName
	case "spec.static":
		return fmt.Sprintf("%v", o.Spec.Static)
	}
	return ""
}

func (*OAuthClient) FieldNames() []string {
	return []string{"spec.mcpServerName", "spec.static"}
}

type OAuthClientSpec struct {
	Manifest                   types.OAuthClientManifest `json:"manifest"`
	ClientSecretHash           []byte                    `json:"clientSecretHash"`
	ClientSecretIssuedAt       metav1.Time               `json:"client_secret_issued_at"`
	ClientSecretExpiresAt      metav1.Time               `json:"client_secret_expires_at"`
	RegistrationTokenHash      []byte                    `json:"registrationTokenHash"`
	RegistrationTokenIssuedAt  metav1.Time               `json:"registration_token_issued_at"`
	RegistrationTokenExpiresAt metav1.Time               `json:"registration_token_expires_at"`
	MCPServerName              string                    `json:"mcp_server_name"`
	// Ephemeral indicates that the OAuth client is temporary and will be deleted after a certain period of time.
	// This is used for generating tool previews for example.
	Ephemeral bool `json:"ephemeral"`

	// Static indicates that the OAuth client is not dynamically registered, but was created manually.
	Static bool `json:"static"`
}

type OAuthClientStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthClient `json:"items"`
}
