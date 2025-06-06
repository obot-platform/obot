package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthClientSpec   `json:"spec,omitempty"`
	Status            OAuthClientStatus `json:"status,omitempty"`
}

type OAuthClientSpec struct {
	Manifest                   types.OAuthClientManifest `json:"manifest"`
	ClientSecretHash           []byte                    `json:"clientSecretHash"`
	ClientSecretIssuedAt       metav1.Time               `json:"client_secret_issued_at"`
	ClientSecretExpiresAt      metav1.Time               `json:"client_secret_expires_at"`
	RegistrationTokenHash      []byte                    `json:"registrationTokenHash"`
	RegistrationTokenIssuedAt  metav1.Time               `json:"registration_token_issued_at"`
	RegistrationTokenExpiresAt metav1.Time               `json:"registration_token_expires_at"`
}

type OAuthClientStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OAuthClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthClient `json:"items"`
}
