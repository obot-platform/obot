package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*ImagePullSecret)(nil)

type ImagePullSecretType string

const (
	ImagePullSecretTypeBasic ImagePullSecretType = "basic"
	ImagePullSecretTypeECR   ImagePullSecretType = "ecr"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ImagePullSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImagePullSecretSpec   `json:"spec,omitempty"`
	Status ImagePullSecretStatus `json:"status,omitempty"`
}

func (in *ImagePullSecret) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ImagePullSecret) Get(field string) (value string) {
	switch field {
	case "spec.secretName":
		return in.Spec.SecretName
	}
	return ""
}

func (in *ImagePullSecret) FieldNames() []string {
	return []string{"spec.secretName"}
}

type ImagePullSecretSpec struct {
	Enabled     bool                      `json:"enabled,omitempty"`
	Type        ImagePullSecretType       `json:"type,omitempty"`
	DisplayName string                    `json:"displayName,omitempty"`
	SecretName  string                    `json:"secretName,omitempty"`
	Basic       *BasicImagePullSecretSpec `json:"basic,omitempty"`
	ECR         *ECRImagePullSecretSpec   `json:"ecr,omitempty"`
}

type BasicImagePullSecretSpec struct {
	Server   string `json:"server,omitempty"`
	Username string `json:"username,omitempty"`
}

type ECRImagePullSecretSpec struct {
	RoleARN         string `json:"roleARN,omitempty"`
	Region          string `json:"region,omitempty"`
	IssuerURL       string `json:"issuerURL,omitempty"`
	Audience        string `json:"audience,omitempty"`
	RefreshSchedule string `json:"refreshSchedule,omitempty"`
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
