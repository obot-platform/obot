package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ DeleteRefs = (*AccessControlRule)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AccessControlRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AccessControlRuleSpec `json:"spec,omitempty"`
}

type AccessControlRuleSpec struct {
	MCPCatalogID          string                          `json:"mcpCatalogID,omitempty"`
	PowerUserWorkspaceID  string                          `json:"powerUserWorkspaceID,omitempty"`
	Manifest              types.AccessControlRuleManifest `json:"manifest"`
}

func (in *AccessControlRule) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.DisplayName"},
		{"Catalog", "Spec.MCPCatalogID"},
		{"Workspace", "Spec.PowerUserWorkspaceID"},
		{"Subjects", "{{len .Spec.Manifest.Subjects}}"},
		{"Resources", "{{len .Spec.Manifest.Resources}}"},
	}
}

func (in *AccessControlRule) DeleteRefs() []Ref {
	refs := []Ref{
		{ObjType: &MCPCatalog{}, Name: in.Spec.MCPCatalogID},
	}
	if in.Spec.PowerUserWorkspaceID != "" {
		refs = append(refs, Ref{ObjType: &PowerUserWorkspace{}, Name: in.Spec.PowerUserWorkspaceID})
	}
	return refs
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AccessControlRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AccessControlRule `json:"items"`
}
