package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*HostedAgentPoolAssignment)(nil)
	_ DeleteRefs    = (*HostedAgentPoolAssignment)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   HostedAgentPoolSpec   `json:"spec"`
	Status HostedAgentPoolStatus `json:"status"`
}

type HostedAgentPoolSpec struct {
	Manifest types.HostedAgentPoolManifest `json:"manifest"`
}

type HostedAgentPoolStatus struct {
	types.HostedAgentPoolStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []HostedAgentPool `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentPoolDefaults struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec HostedAgentPoolDefaultsSpec `json:"spec"`
}

type HostedAgentPoolDefaultsSpec struct {
	Manifest types.HostedAgentPoolDefaultsManifest `json:"manifest"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentPoolDefaultsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []HostedAgentPoolDefaults `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentPoolAssignment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec HostedAgentPoolAssignmentSpec `json:"spec"`
}

func (in *HostedAgentPoolAssignment) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *HostedAgentPoolAssignment) Get(field string) string {
	switch field {
	case "spec.userID":
		return in.Spec.Manifest.UserID
	case "spec.poolID":
		return in.Spec.Manifest.PoolID
	case "spec.default":
		return strconv.FormatBool(in.Spec.Manifest.Default)
	}
	return ""
}

func (in *HostedAgentPoolAssignment) FieldNames() []string {
	return []string{"spec.userID", "spec.poolID", "spec.default"}
}

func (in *HostedAgentPoolAssignment) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &HostedAgentPool{}, Name: in.Spec.Manifest.PoolID},
	}
}

type HostedAgentPoolAssignmentSpec struct {
	Manifest types.HostedAgentPoolAssignmentManifest `json:"manifest"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentPoolAssignmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []HostedAgentPoolAssignment `json:"items"`
}
