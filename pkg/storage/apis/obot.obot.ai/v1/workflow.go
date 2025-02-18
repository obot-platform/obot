package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ Aliasable     = (*Workflow)(nil)
	_ Generationed  = (*Workflow)(nil)
	_ AliasScoped   = (*Workflow)(nil)
	_ fields.Fields = (*Workflow)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowSpec   `json:"spec,omitempty"`
	Status WorkflowStatus `json:"status,omitempty"`
}

func (in *Workflow) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *Workflow) Get(field string) (value string) {
	switch field {
	case "spec.threadName":
		return in.Spec.ThreadName
	}
	return ""
}

func (in *Workflow) FieldNames() []string {
	return []string{
		"spec.threadName",
	}
}

func (in *Workflow) GetAliasName() string {
	return in.Spec.Manifest.Alias
}

func (in *Workflow) SetAssigned(assigned bool) {
	in.Status.AliasAssigned = assigned
}

func (in *Workflow) IsAssigned() bool {
	return in.Status.AliasAssigned
}

func (in *Workflow) GetAliasScope() string {
	return "Agent"
}

func (in *Workflow) GetObservedGeneration() int64 {
	return in.Status.ObservedGeneration
}

func (in *Workflow) SetObservedGeneration(gen int64) {
	in.Status.ObservedGeneration = gen
}

func (in *Workflow) GetTools() []string {
	return slices.Concat(in.Spec.Manifest.Tools, in.Spec.Manifest.DefaultThreadTools, in.Spec.Manifest.AvailableThreadTools)
}

func (in *Workflow) GetToolInfos() map[string]types.ToolInfo {
	return in.Status.ToolInfo
}

func (in *Workflow) SetToolInfos(toolInfos map[string]types.ToolInfo) {
	in.Status.ToolInfo = toolInfos
}

type WorkflowSpec struct {
	ThreadName                   string                 `json:"threadName,omitempty"`
	Manifest                     types.WorkflowManifest `json:"manifest,omitempty"`
	CredentialContextID          string                 `json:"credentialContextID,omitempty"`
	AdditionalCredentialContexts []string               `json:"additionalCredentialContexts,omitempty"`
	KnowledgeSetNames            []string               `json:"knowledgeSetNames,omitempty"`
	WorkspaceName                string                 `json:"workspaceName,omitempty"`
}

func (in *Workflow) DeleteRefs() []Ref {
	refs := []Ref{
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
		{ObjType: &Workspace{}, Name: in.Spec.WorkspaceName},
	}
	for _, name := range in.Spec.KnowledgeSetNames {
		refs = append(refs, Ref{ObjType: &KnowledgeSet{}, Name: name})
	}
	return refs
}

type WorkflowStatus struct {
	WorkspaceName      string                                   `json:"workspaceName,omitempty"`
	KnowledgeSetNames  []string                                 `json:"knowledgeSetNames,omitempty"`
	AliasAssigned      bool                                     `json:"aliasAssigned,omitempty"`
	AuthStatus         map[string]types.OAuthAppLoginAuthStatus `json:"authStatus,omitempty"`
	ToolInfo           map[string]types.ToolInfo                `json:"toolInfo,omitempty"`
	ObservedGeneration int64                                    `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Workflow `json:"items"`
}
