package v1

import (
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*ThreadShare)(nil)
var _ DeleteRefs = (*ThreadShare)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadShare struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreadShareSpec   `json:"spec,omitempty"`
	Status ThreadShareStatus `json:"status,omitempty"`
}

func (in *ThreadShare) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &Thread{},
			Name:    in.Spec.ProjectThreadName,
		},
	}
}

func (in *ThreadShare) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ThreadShare) Get(field string) (value string) {
	switch field {
	case "spec.publicID":
		return in.Spec.PublicID
	case "spec.public":
		return fmt.Sprintf("%t", in.Spec.Manifest.Public)
	case "spec.userID":
		return in.Spec.UserID
	case "spec.template":
		return fmt.Sprintf("%t", in.Spec.Template)
	case "spec.featured":
		return fmt.Sprintf("%t", in.Spec.Featured)
	case "spec.projectThreadName":
		return in.Spec.ProjectThreadName
	default:
		return ""
	}
}

func (in *ThreadShare) FieldNames() []string {
	return []string{"spec.publicID", "spec.public", "spec.userID", "spec.featured", "spec.projectThreadName", "spec.template"}
}

type ThreadShareSpec struct {
	Manifest          types.ProjectShareManifest `json:"manifest,omitempty"`
	PublicID          string                     `json:"publicID,omitempty"`
	UserID            string                     `json:"userID,omitempty"`
	ProjectThreadName string                     `json:"projectThreadName,omitempty"`
	Template          bool                       `json:"template,omitempty"`
	Featured          bool                       `json:"featured,omitempty"`
	Editor            bool                       `json:"editor,omitempty"`
}

type ThreadShareStatus struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Icons       *types.AgentIcons `json:"icons"`
	Tools       []string          `json:"tools,omitempty"`

	// MCPServers contains the MCP server catalog IDs of the MCP servers that have been added to the project thread being shared.
	MCPServers []string `json:"mcpServers,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadShareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ThreadShare `json:"items"`
}
