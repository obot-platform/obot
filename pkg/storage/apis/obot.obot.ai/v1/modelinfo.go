package v1

import (
	"crypto/sha256"
	"fmt"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ DeleteRefs = (*ModelInfo)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ModelInfo stores synced cost for one provider/model pair.
type ModelInfo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelInfoSpec `json:"spec,omitempty"`
	Status EmptyStatus   `json:"status,omitempty"`
}

type ModelInfoSpec struct {
	// ModelInfoSourceName is the source that produced this ModelInfo.
	ModelInfoSourceName string          `json:"modelInfoSourceName,omitempty"`
	Provider            string          `json:"provider,omitempty"`
	Model               string          `json:"model,omitempty"`
	Cost                types.ModelCost `json:"cost,omitzero"`
}

func (in *ModelInfo) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &ModelInfoSource{}, Name: in.Spec.ModelInfoSourceName},
	}
}

func (in *ModelInfo) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Provider", "Spec.Provider"},
		{"Model", "Spec.Model"},
		{"Source", "Spec.ModelInfoSourceName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

// ModelInfoName returns the deterministic name for a provider/model pair.
func ModelInfoName(provider, model string) string {
	return name.SafeConcatName(provider, fmt.Sprintf("%x", sha256.Sum256([]byte(model))))
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ModelInfoList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelInfo `json:"items"`
}
