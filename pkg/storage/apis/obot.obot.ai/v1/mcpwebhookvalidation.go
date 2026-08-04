package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPWebhookValidation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   MCPWebhookValidationSpec   `json:"spec"`
	Status MCPWebhookValidationStatus `json:"status"`
}

type MCPWebhookValidationSpec struct {
	Manifest types.MCPWebhookValidationManifest `json:"manifest"`
}

type MCPWebhookValidationStatus struct {
	Configured bool `json:"configured"`
}

func (in *MCPWebhookValidation) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.Name"},
		{"Resources ", "{{len .Spec.Manifest.Resources}}"},
		{"URL", "{{.Spec.Manifest.URL}}"},
		{"Disabled", "{{.Spec.Manifest.Disabled}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPWebhookValidationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []MCPWebhookValidation `json:"items"`
}
