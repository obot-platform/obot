package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ToolReference)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ToolReference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ToolReferenceSpec   `json:"spec,omitempty"`
	Status ToolReferenceStatus `json:"status,omitempty"`
}

func (in *ToolReference) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *ToolReference) Get(field string) string {
	if in != nil {
		switch field {
		case "spec.type":
			return string(in.Spec.Type)
		}
	}

	return ""
}

func (in *ToolReference) FieldNames() []string {
	return []string{"spec.type"}
}

func (in *ToolReference) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Reference", "Spec.Reference"},
		{"Error", "Status.Error"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type ToolReferenceSpec struct {
	Type           types.ToolReferenceType `json:"type,omitempty"`
	Builtin        bool                    `json:"builtin,omitempty"`
	Reference      string                  `json:"reference,omitempty"`
	Active         *bool                   `json:"active,omitempty"`
	Bundle         bool                    `json:"bundle,omitempty"`
	BundleToolName string                  `json:"bundleToolName,omitempty"`
	ForceRefresh   metav1.Time             `json:"forceRefresh,omitempty"`
}

type ToolShortDescription struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	// Credentials are all the credentials for this tool, including for tools exported by this tool.
	Credentials []string `json:"credentials,omitempty"`
	// CredentialNames are the names of the credentials for each tool. This is different from the Credentials field
	// because these names could be aliases and identifies which tools have the same credential.
	CredentialNames []string `json:"credentialNames,omitempty"`
}

type ToolReferenceStatus struct {
	Reference          string                `json:"reference,omitempty"`
	Commit             string                `json:"commit,omitempty"`
	ObservedGeneration int64                 `json:"observedGeneration,omitempty"`
	LastReferenceCheck metav1.Time           `json:"lastReferenceCheck,omitempty"`
	Tool               *ToolShortDescription `json:"tool,omitempty"`
	Error              string                `json:"error,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ToolReferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ToolReference `json:"items"`
}
