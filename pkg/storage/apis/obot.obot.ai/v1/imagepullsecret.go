package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ImagePullSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImagePullSecretSpec   `json:"spec,omitempty"`
	Status ImagePullSecretStatus `json:"status,omitempty"`
}

type ImagePullSecretSpec struct {
	Enabled     bool                              `json:"enabled,omitempty"`
	Type        types.ImagePullSecretType         `json:"type,omitempty"`
	DisplayName string                            `json:"displayName,omitempty"`
	Basic       *types.BasicImagePullSecretConfig `json:"basic,omitempty"`
	ECR         *types.ECRImagePullSecretConfig   `json:"ecr,omitempty"`
}

type ImagePullSecretStatus struct {
	LastReconciledTime *metav1.Time `json:"lastReconciledTime,omitempty"`
	LastSuccessTime    *metav1.Time `json:"lastSuccessTime,omitempty"`
	LastError          string       `json:"lastError,omitempty"`

	IssuerURL         string       `json:"issuerURL,omitempty"`
	Subject           string       `json:"subject,omitempty"`
	Audience          string       `json:"audience,omitempty"`
	TokenExpiresAt    *metav1.Time `json:"tokenExpiresAt,omitempty"`
	RegistryEndpoints []string     `json:"registryEndpoints,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ImagePullSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ImagePullSecret `json:"items"`
}
