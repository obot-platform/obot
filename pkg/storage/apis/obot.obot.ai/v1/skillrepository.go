package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*SkillRepository)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SkillRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkillRepositorySpec   `json:"spec,omitempty"`
	Status SkillRepositoryStatus `json:"status,omitempty"`
}

func (in *SkillRepository) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *SkillRepository) Get(field string) (value string) {
	switch field {
	case "spec.repoURL":
		return in.Spec.RepoURL
	case "spec.ref":
		return in.Spec.Ref
	}

	return ""
}

func (in *SkillRepository) FieldNames() []string {
	return []string{"spec.repoURL", "spec.ref"}
}

func (in *SkillRepository) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.DisplayName"},
		{"Repo URL", "Spec.RepoURL"},
		{"Ref", "Spec.Ref"},
		{"Discovered Skills", "Status.DiscoveredSkillCount"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
	}
}

type SkillRepositorySpec struct {
	types.SkillRepositoryManifest `json:",inline"`
}

type SkillRepositoryStatus struct {
	LastSyncTime         metav1.Time `json:"lastSyncTime,omitzero"`
	IsSyncing            bool        `json:"isSyncing,omitempty"`
	SyncError            string      `json:"syncError,omitempty"`
	ResolvedCommitSHA    string      `json:"resolvedCommitSHA,omitempty"`
	DiscoveredSkillCount int         `json:"discoveredSkillCount"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SkillRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SkillRepository `json:"items"`
}
