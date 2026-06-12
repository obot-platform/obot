package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AuthProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthProviderSpec   `json:"spec,omitempty"`
	Status AuthProviderStatus `json:"status,omitempty"`
}

func (in *AuthProvider) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Command", "Spec.Command"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type AuthProviderSpec struct {
	types.AuthProviderManifest `json:",inline"`
}

type AuthProviderStatus struct {
	Configured                     bool     `json:"configured"`
	MissingConfigurationParameters []string `json:"missingConfigurationParameters,omitempty"`
	ObservedGeneration             int64    `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AuthProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AuthProvider `json:"items"`
}
