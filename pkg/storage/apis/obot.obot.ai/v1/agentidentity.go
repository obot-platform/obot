package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*AgentIdentityChange)(nil)
	_ fields.Fields = (*AgentIdentityDelete)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AgentIdentityChange reports that an agent identity was disabled or enabled, so that controllers re-evaluate what
// depends on its access. Its name comes from the gateway lifecycle event it delivers. The agent may have changed
// again since, so handlers must read its current status.
type AgentIdentityChange struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   AgentIdentityChangeSpec `json:"spec"`
	Status EmptyStatus             `json:"status"`
}

type AgentIdentityChangeSpec struct {
	AgentID uint `json:"agentID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentIdentityChangeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []AgentIdentityChange `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AgentIdentityDelete makes the controller remove what a deleted agent identity owns. Its name comes from the gateway
// lifecycle event it delivers.
type AgentIdentityDelete struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   AgentIdentityDeleteSpec `json:"spec"`
	Status EmptyStatus             `json:"status"`
}

type AgentIdentityDeleteSpec struct {
	AgentID uint `json:"agentID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentIdentityDeleteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []AgentIdentityDelete `json:"items"`
}

func (in *AgentIdentityChange) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

// Get returns the agent's bare decimal ID for spec.agentID, so that a controller watching an agent's events selects
// them by the ID in the agent's principal (principal.Principal.ID).
func (in *AgentIdentityChange) Get(field string) string {
	switch field {
	case "spec.agentID":
		return strconv.FormatUint(uint64(in.Spec.AgentID), 10)
	}
	return ""
}

func (*AgentIdentityChange) FieldNames() []string {
	return []string{"spec.agentID"}
}

func (in *AgentIdentityDelete) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

// Get returns the agent's bare decimal ID for spec.agentID, as AgentIdentityChange.Get does.
func (in *AgentIdentityDelete) Get(field string) string {
	switch field {
	case "spec.agentID":
		return strconv.FormatUint(uint64(in.Spec.AgentID), 10)
	}
	return ""
}

func (*AgentIdentityDelete) FieldNames() []string {
	return []string{"spec.agentID"}
}
