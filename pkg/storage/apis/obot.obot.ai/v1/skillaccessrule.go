package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SkillAccessRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkillAccessRuleSpec `json:"spec,omitempty"`
	Status EmptyStatus         `json:"status,omitempty"`
}

func (in *SkillAccessRule) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.DisplayName"},
		{"Subjects", "{{len .Spec.Manifest.Subjects}}"},
		{"Resources", "{{len .Spec.Manifest.Resources}}"},
	}
}

type SkillAccessRuleSpec struct {
	Manifest types.SkillAccessRuleManifest `json:"manifest"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SkillAccessRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SkillAccessRule `json:"items"`
}
