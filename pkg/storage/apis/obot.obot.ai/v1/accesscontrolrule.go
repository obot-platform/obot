package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*AccessControlRule)(nil)
	_ DeleteRefs    = (*AccessControlRule)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AccessControlRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AccessControlRuleSpec `json:"spec,omitempty"`
}

type AccessControlRuleSpec struct {
	MCPCatalogID           string                          `json:"mcpCatalogID,omitempty"`
	Manifest               types.AccessControlRuleManifest `json:"manifest"`
	PowerUserWorkspaceName string                          `json:"powerUserWorkspaceName,omitempty"`
}

func (in *AccessControlRule) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *AccessControlRule) Get(field string) (value string) {
	switch field {
	case "spec.mcpCatalogID":
		return in.Spec.MCPCatalogID
	case "spec.powerUserWorkspaceName":
		return in.Spec.PowerUserWorkspaceName
	}
	return ""
}

func (in *AccessControlRule) FieldNames() []string {
	return []string{
		"spec.mcpCatalogID",
		"spec.powerUserWorkspaceName",
	}
}

func (in *AccessControlRule) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &MCPCatalog{}, Name: in.Spec.MCPCatalogID},
		{ObjType: &PowerUserWorkspace{}, Name: in.Spec.PowerUserWorkspaceName},
	}
}

func (in *AccessControlRule) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.DisplayName"},
		{"Catalog", "Spec.MCPCatalogID"},
		{"Subjects", "{{len .Spec.Manifest.Subjects}}"},
		{"Resources", "{{len .Spec.Manifest.Resources}}"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AccessControlRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AccessControlRule `json:"items"`
}
