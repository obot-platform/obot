package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*AuthProvider)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AuthProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthProviderSpec   `json:"spec,omitempty"`
	Status AuthProviderStatus `json:"status,omitempty"`
}

func (in *AuthProvider) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *AuthProvider) Get(field string) (value string) {
	switch field {
	case "status.configured":
		return strconv.FormatBool(in.Status.Configured)
	}
	return ""
}

func (in *AuthProvider) FieldNames() []string {
	return []string{"status.configured"}
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
