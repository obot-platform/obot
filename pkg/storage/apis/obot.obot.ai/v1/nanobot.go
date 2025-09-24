package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*NanobotConfig)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type NanobotConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              NanobotConfigSpec `json:"spec,omitzero"`
	Status            EmptyStatus       `json:"status,omitzero"`
}

func (in *NanobotConfig) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *NanobotConfig) Get(field string) (value string) {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	}
	return ""
}

func (in *NanobotConfig) FieldNames() []string {
	return []string{
		"spec.userID",
	}
}

type NanobotConfigSpec struct {
	Manifest types.NanobotConfigManifest `json:"manifest,omitzero"`
	UserID   string                      `json:"userID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type NanobotConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NanobotConfig `json:"items"`
}
