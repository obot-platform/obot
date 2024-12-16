package v1

import (
	"github.com/acorn-io/acorn/apiclient/types"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DefaultModelAlias struct {
	v1.TypeMeta   `json:",inline"`
	v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DefaultModelAliasSpec   `json:"spec"`
	Status DefaultModelAliasStatus `json:"status"`
}

func (a *DefaultModelAlias) IsAssigned() bool {
	return true
}

func (a *DefaultModelAlias) GetAliasName() string {
	return a.Spec.Manifest.Alias
}

func (a *DefaultModelAlias) SetAssigned(bool) {}

func (a *DefaultModelAlias) GetAliasScope() string {
	return "Model"
}

func (a *DefaultModelAlias) GetAliasObservedGeneration() int64 {
	return a.Generation
}

func (a *DefaultModelAlias) SetAliasObservedGeneration(int64) {}

type DefaultModelAliasSpec struct {
	Manifest types.DefaultModelAliasManifest `json:"manifest"`
}

type DefaultModelAliasStatus struct {
	SetAliasName string `json:"setAliasName"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DefaultModelAliasList struct {
	v1.TypeMeta `json:",inline"`
	v1.ListMeta `json:"metadata,omitempty"`
	Items       []DefaultModelAlias `json:"items"`
}
