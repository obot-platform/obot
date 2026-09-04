package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*VMCP)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VMCP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   VMCPSpec   `json:"spec"`
	Status VMCPStatus `json:"status"`
}

type VMCPSpec struct {
	Manifest types.VMCPManifest `json:"manifest"`
	// UserID is set for a personal VMCP and empty for an administrator-created shared VMCP.
	UserID                  string `json:"userID,omitempty"`
	StaticConfigurationHash string `json:"staticConfigurationHash,omitempty"`
}

type VMCPStatus struct {
	Ready      bool                  `json:"ready,omitempty"`
	Components []VMCPComponentStatus `json:"components,omitempty"`
}

type VMCPComponentStatus struct {
	Name          string `json:"name"`
	Ready         bool   `json:"ready,omitempty"`
	Error         string `json:"error,omitempty"`
	SourceMissing bool   `json:"sourceMissing,omitempty"`
	NeedsUpdate   bool   `json:"needsUpdate,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VMCPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VMCP `json:"items"`
}

func (in *VMCP) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *VMCP) Get(field string) string {
	if field == "spec.userID" {
		return in.Spec.UserID
	}
	return ""
}

func (*VMCP) FieldNames() []string {
	return []string{"spec.userID"}
}

func (in *VMCP) IsPersonal() bool {
	return in.Spec.UserID != ""
}
