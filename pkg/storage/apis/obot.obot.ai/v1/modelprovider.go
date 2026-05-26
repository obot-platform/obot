package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelProviderSpec   `json:"spec,omitempty"`
	Status ModelProviderStatus `json:"status,omitempty"`
}

func (in *ModelProvider) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Image", "Spec.Image"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type ModelProviderSpec struct {
	types.ModelProviderManifest `json:",inline"`
}

type ModelProviderStatus struct {
	Configured                     bool     `json:"configured"`
	MissingConfigurationParameters []string `json:"missingConfigurationParameters,omitempty"`
	ModelsBackPopulated            *bool    `json:"modelsBackPopulated,omitempty"`
	Error                          string   `json:"error,omitempty"`
	ObservedGeneration             int64    `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelProvider `json:"items"`
}
