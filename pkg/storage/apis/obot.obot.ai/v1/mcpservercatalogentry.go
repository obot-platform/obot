package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ DeleteRefs = (*MCPServerCatalogEntry)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerCatalogEntry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerCatalogEntrySpec   `json:"spec,omitempty"`
	Status MCPServerCatalogEntryStatus `json:"status,omitempty"`
}

func (in *MCPServerCatalogEntry) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &ToolReference{}, Name: in.Spec.ToolReferenceName},
	}
}

type MCPServerCatalogEntrySpec struct {
	Manifest          types.MCPServerCatalogEntryManifest `json:"manifest,omitempty"`
	URL               string                              `json:"url,omitempty"`
	ToolReferenceName string                              `json:"toolReferenceName,omitempty"`
	GitHubStars       int                                 `json:"githubStars,omitempty"`
	Metadata          map[string]string                   `json:"metadata,omitempty"`
}

type MCPServerCatalogEntryStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerCatalogEntryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPServerCatalogEntry `json:"items"`
}
