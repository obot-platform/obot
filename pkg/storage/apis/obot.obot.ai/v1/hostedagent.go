package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*HostedAgent)(nil)
	_ DeleteRefs    = (*HostedAgent)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostedAgentSpec   `json:"spec,omitempty"`
	Status HostedAgentStatus `json:"status,omitempty"`
}

func (in *HostedAgent) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *HostedAgent) Get(field string) string {
	switch field {
	case "spec.perUser":
		return strconv.FormatBool(in.Spec.Manifest.PerUser)
	case "spec.sourceID":
		return in.Spec.SourceID
	}
	return ""
}

func (in *HostedAgent) FieldNames() []string {
	return []string{"spec.perUser", "spec.sourceID"}
}

// DeleteRefs makes agents discovered from a source disappear with it. Agents
// registered by hand have no SourceID, and an empty ref is skipped by
// cleanup.Cleanup, so they are unaffected.
func (in *HostedAgent) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &AgentSource{}, Name: in.Spec.SourceID},
	}
}

func (in *HostedAgent) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.Name"},
		{"Image", "Spec.Manifest.Image"},
		{"Per User", "Spec.Manifest.PerUser"},
		{"State", "Status.State"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type HostedAgentSpec struct {
	// Manifest holds the agent definition. Values for env entries marked
	// sensitive are blanked here and kept in the credential store instead.
	Manifest types.HostedAgentManifest `json:"manifest,omitempty"`

	// SourceID names the AgentSource this agent was discovered from. Empty for
	// agents an admin registered by hand, which the sync never touches.
	SourceID string `json:"sourceID,omitempty"`
	// RelativePath is where the agent was found within the source repository.
	RelativePath string `json:"relativePath,omitempty"`
	// CommitSHA is the source commit this agent was built from.
	CommitSHA string `json:"commitSHA,omitempty"`
}

type HostedAgentStatus struct {
	types.HostedAgentStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []HostedAgent `json:"items"`
}
