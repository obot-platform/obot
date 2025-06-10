package v1

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Catalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CatalogSpec   `json:"spec,omitempty"`
	Status CatalogStatus `json:"status,omitempty"`
}

type CatalogSpec struct {
	URL string `json:"url,omitempty"`
}

type CatalogStatus struct {
	LastSyncTime metav1.Time `json:"lastSyncTime,omitempty"`
}

func (in *Catalog) NamespaceScoped() bool {
	return false
}

func (in *Catalog) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"URL", "Spec.URL"},
		{"Last Synced", "{{ago .Status.LastSynced}}"},
	}
}

func (in *Catalog) Get(field string) string {
	switch field {
	case "spec.url":
		return in.Spec.URL
	}
	return ""
}

func (in *Catalog) FieldNames() []string {
	return []string{"spec.url"}
}

func (in *Catalog) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Catalog `json:"items"`
}
