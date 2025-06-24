package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AccessControlRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AccessControlRuleSpec `json:"spec,omitempty"`
}

type AccessControlRuleSpec struct {
	DisplayName                string   `json:"displayName,omitempty"`
	UserIDs                    []string `json:"userIDs,omitempty"`
	MCPServerCatalogEntryNames []string `json:"mcpServerCatalogEntryNames,omitempty"`
	MCPServerNames             []string `json:"mcpServerNames,omitempty"`
}

func (in *AccessControlRule) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.DisplayName"},
		{"User Count", "{{len .Spec.UserIDs}}"},
		{"Catalog Entries", "{{len .Spec.MCPServerCatalogEntryNames}}"},
		{"Servers", "{{len .Spec.MCPServerNames}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AccessControlRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AccessControlRule `json:"items"`
}
