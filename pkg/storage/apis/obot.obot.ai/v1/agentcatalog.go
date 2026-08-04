package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*AgentCatalog)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   AgentCatalogSpec   `json:"spec"`
	Status AgentCatalogStatus `json:"status"`
}

func (in *AgentCatalog) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *AgentCatalog) Get(field string) (value string) {
	switch field {
	case "spec.repoURL":
		return in.Spec.RepoURL
	case "spec.ref":
		return in.Spec.Ref
	}

	return ""
}

func (in *AgentCatalog) FieldNames() []string {
	return []string{"spec.repoURL", "spec.ref"}
}

func (in *AgentCatalog) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.DisplayName"},
		{"Repo URL", "Spec.RepoURL"},
		{"Ref", "Spec.Ref"},
		{"Discovered Agents", "Status.DiscoveredAgentCount"},
		{"Discovered Harnesses", "Status.DiscoveredHarnessCount"},
		{"Last Synced", "{{ago .Status.LastSyncTime}}"},
	}
}

type AgentCatalogSpec struct {
	types.AgentCatalogManifest `json:",inline"`
}

type AgentCatalogStatus struct {
	LastSyncTime           metav1.Time `json:"lastSyncTime,omitzero"`
	IsSyncing              bool        `json:"isSyncing,omitempty"`
	SyncError              string      `json:"syncError,omitempty"`
	ResolvedCommitSHA      string      `json:"resolvedCommitSHA,omitempty"`
	DiscoveredAgentCount   int         `json:"discoveredAgentCount"`
	DiscoveredHarnessCount int         `json:"discoveredHarnessCount"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []AgentCatalog `json:"items"`
}
