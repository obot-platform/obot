package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPWebhookValidation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MCPWebhookValidationSpec `json:"spec,omitempty"`
}

type MCPWebhookValidationSpec struct {
	Manifest types.MCPWebhookValidationManifest `json:"manifest"`
}

func (in *MCPWebhookValidation) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.Name"},
		{"Resources ", "{{len .Spec.Manifest.Resources}}"},
		{"Webhooks", "{{len .Spec.Manifest.Webhooks}}"},
		{"Disabled", "{{.Spec.Manifest.Disabled}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPWebhookValidationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPWebhookValidation `json:"items"`
}
