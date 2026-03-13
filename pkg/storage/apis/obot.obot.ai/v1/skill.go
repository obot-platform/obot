package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*Skill)(nil)
	_ DeleteRefs    = (*Skill)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Skill struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkillSpec   `json:"spec,omitempty"`
	Status SkillStatus `json:"status,omitempty"`
}

func (in *Skill) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *Skill) Get(field string) (value string) {
	switch field {
	case "spec.repoID":
		return in.Spec.RepoID
	case "spec.relativePath":
		return in.Spec.RelativePath
	}

	return ""
}

func (in *Skill) FieldNames() []string {
	return []string{"spec.repoID", "spec.relativePath"}
}

func (in *Skill) DeleteRefs() []Ref {
	return []Ref{{
		ObjType: &SkillRepository{},
		Name:    in.Spec.RepoID,
	}}
}

func (in *Skill) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.DisplayName"},
		{"Repository", "Spec.RepoID"},
		{"Path", "Spec.RelativePath"},
		{"Valid", "Status.Valid"},
		{"Last Indexed", "{{ago .Status.LastIndexedAt}}"},
	}
}

type SkillSpec struct {
	types.SkillManifest `json:",inline"`

	RepoID       string `json:"repoID,omitempty"`
	RepoURL      string `json:"repoURL,omitempty"`
	RepoRef      string `json:"repoRef,omitempty"`
	CommitSHA    string `json:"commitSHA,omitempty"`
	RelativePath string `json:"relativePath,omitempty"`
	InstallHash  string `json:"installHash,omitempty"`
}

type SkillStatus struct {
	Valid           bool        `json:"valid,omitempty"`
	ValidationError string      `json:"validationError,omitempty"`
	LastIndexedAt   metav1.Time `json:"lastIndexedAt,omitzero"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SkillList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Skill `json:"items"`
}
