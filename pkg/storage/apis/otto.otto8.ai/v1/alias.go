package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ DeleteRefs = (*Alias)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Alias struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AliasSpec   `json:"spec,omitempty"`
	Status EmptyStatus `json:"status,omitempty"`
}

func (in *Alias) DeleteRefs() []Ref {
	return []Ref{
		{
			Kind:      in.Spec.TargetKind,
			Name:      in.Spec.TargetName,
			Namespace: in.Spec.TargetNamespace,
		},
	}
}

func (in *Alias) NamespaceScoped() bool {
	return false
}

type AliasSpec struct {
	Name            string `json:"name,omitempty"`
	TargetName      string `json:"targetName,omitempty"`
	TargetNamespace string `json:"targetNamespace,omitempty"`
	TargetKind      string `json:"targetKind,omitempty"`
}

// +k8s:deepcopy-gen=false

type Aliasable interface {
	kclient.Object
	GetAliasName() string
	SetAssigned(bool)
	IsAssigned() bool
	GetAliasObservedGeneration() int64
	SetAliasObservedGeneration(int64)
}

// +k8s:deepcopy-gen=false

type AliasScoped interface {
	// GetAliasScope returns the scope of the alias which defaults to the Kind name if this interface
	// is not implemented.
	GetAliasScope() string
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AliasList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Alias `json:"items"`
}
