package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*AgentSource)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSourceSpec   `json:"spec,omitempty"`
	Status AgentSourceStatus `json:"status,omitempty"`
}

func (in *AgentSource) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *AgentSource) Get(field string) (value string) {
	switch field {
	case "spec.repoURL":
		return in.Spec.RepoURL
	case "spec.ref":
		return in.Spec.Ref
	}

	return ""
}

func (in *AgentSource) FieldNames() []string {
	return []string{"spec.repoURL", "spec.ref"}
}

func (in *AgentSource) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.DisplayName"},
		{"Repo URL", "Spec.RepoURL"},
		{"Ref", "Spec.Ref"},
		{"Discovered Agents", "Status.DiscoveredAgentCount"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
	}
}

type AgentSourceSpec struct {
	types.AgentSourceManifest `json:",inline"`
}

type AgentSourceStatus struct {
	LastSyncTime         metav1.Time `json:"lastSyncTime,omitzero"`
	IsSyncing            bool        `json:"isSyncing,omitempty"`
	SyncError            string      `json:"syncError,omitempty"`
	ResolvedCommitSHA    string      `json:"resolvedCommitSHA,omitempty"`
	DiscoveredAgentCount int         `json:"discoveredAgentCount"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AgentSource `json:"items"`
}
