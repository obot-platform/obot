package v1

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type UserCatalogAuthorization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec UserCatalogAuthorizationSpec `json:"spec,omitempty"`
}

type UserCatalogAuthorizationSpec struct {
	UserID         string `json:"userID,omitempty"`
	MCPCatalogName string `json:"mcpCatalogName,omitempty"`
}

func (in *UserCatalogAuthorization) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &MCPCatalog{}, Name: in.Spec.MCPCatalogName},
	}
}

func (in *UserCatalogAuthorization) GetColumns() [][]string {
	return [][]string{
		{"User ID", "Spec.UserID"},
		{"MCPCatalog Name", "Spec.MCPCatalogName"},
	}
}

func (in *UserCatalogAuthorization) Get(field string) string {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.mcpCatalogName":
		return in.Spec.MCPCatalogName
	}
	return ""
}

func (in *UserCatalogAuthorization) FieldNames() []string {
	return []string{"spec.userID", "spec.mcpCatalogName"}
}

func (in *UserCatalogAuthorization) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type UserCatalogAuthorizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []UserCatalogAuthorization `json:"items"`
}
