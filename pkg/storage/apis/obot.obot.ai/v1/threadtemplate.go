package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*ThreadTemplate)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreadTemplateSpec   `json:"spec,omitempty"`
	Status ThreadTemplateStatus `json:"status,omitempty"`
}

func (in *ThreadTemplate) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ThreadTemplate) Get(field string) (value string) {
	switch field {
	case "spec.projectThreadName":
		return in.Spec.ProjectThreadName
	case "spec.userID":
		return in.Spec.UserID
	case "status.publicID":
		return in.Status.PublicID
	default:
		return ""
	}
}

func (in *ThreadTemplate) FieldNames() []string {
	return []string{"spec.projectThreadName", "spec.userID", "status.publicID"}
}

type ThreadTemplateSpec struct {
	ProjectThreadName string `json:"projectThreadName,omitempty"`
	UserID            string `json:"userID,omitempty"`
}

type ThreadTemplateStatus struct {
	Manifest               types.ThreadManifest `json:"manifest,omitempty"`
	Tasks                  []types.TaskManifest `json:"tasks,omitempty"`
	AgentName              string               `json:"agentName,omitempty"`
	PublicID               string               `json:"publicID,omitempty"`
	Ready                  bool                 `json:"ready,omitempty"`
	WorkspaceName          string               `json:"workspaceName,omitempty"`
	KnowledgeWorkspaceName string               `json:"knowledgeWorkspaceName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ThreadTemplate `json:"items"`
}
