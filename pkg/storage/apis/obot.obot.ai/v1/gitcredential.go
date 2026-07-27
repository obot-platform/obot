package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type GitCredential struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitCredentialSpec   `json:"spec,omitempty"`
	Status GitCredentialStatus `json:"status,omitempty"`
}

type GitCredentialSpec struct {
	DisplayName string `json:"displayName,omitempty"`
	Host        string `json:"host,omitempty"`
}

type GitCredentialStatus struct {
	References GitCredentialReferences `json:"references,omitempty"`
}

type GitCredentialReferences struct {
	SkillRepositories []GitCredentialReference `json:"skillRepositories,omitempty"`
	MCPCatalogs       []GitCredentialReference `json:"mcpCatalogs,omitempty"`
	SystemMCPCatalogs []GitCredentialReference `json:"systemMcpCatalogs,omitempty"`
}

func (r GitCredentialReferences) Len() int {
	return len(r.SkillRepositories) + len(r.MCPCatalogs) + len(r.SystemMCPCatalogs)
}

type GitCredentialReference struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
}

func (in *GitCredential) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.DisplayName"},
		{"Host", "Spec.Host"},
	}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type GitCredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []GitCredential `json:"items"`
}
