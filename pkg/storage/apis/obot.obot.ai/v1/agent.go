package v1

import (
	"context"
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ fields.Fields = (*Agent)(nil)
	_ Aliasable     = (*Agent)(nil)
	_ Generationed  = (*Agent)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

func (a *Agent) IsAssigned() bool {
	return a.Status.AliasAssigned
}

func (a *Agent) GetAliasName() string {
	return a.Spec.Manifest.Alias
}

func (a *Agent) SetAssigned(assigned bool) {
	a.Status.AliasAssigned = assigned
}

func (a *Agent) GetObservedGeneration() int64 {
	return a.Status.ObservedGeneration
}

func (a *Agent) SetObservedGeneration(gen int64) {
	a.Status.ObservedGeneration = gen
}

func (a *Agent) GetTools() []string {
	return slices.Concat(a.Spec.Manifest.Tools, a.Spec.Manifest.DefaultThreadTools, a.Spec.Manifest.AvailableThreadTools)
}

func (a *Agent) GetToolInfos() map[string]types.ToolInfo {
	return a.Status.ToolInfo
}

func (a *Agent) SetToolInfos(toolInfos map[string]types.ToolInfo) {
	a.Status.ToolInfo = toolInfos
}

func (a *Agent) Has(field string) bool {
	return a.Get(field) != ""
}

func (a *Agent) Get(field string) string {
	if a != nil {
		switch field {
		case "spec.manifest.model":
			return a.Spec.Manifest.Model
		}
	}

	return ""
}

func (a *Agent) FieldNames() []string {
	return []string{"spec.manifest.model"}
}

type AgentSpec struct {
	Manifest                     types.AgentManifest `json:"manifest,omitempty"`
	SystemTools                  []string            `json:"systemTools,omitempty"`
	ContextInput                 string              `json:"contextInput,omitempty"`
	InputFilters                 []string            `json:"inputFilters,omitempty"`
	CredentialContextID          string              `json:"credentialContextID,omitempty"`
	AdditionalCredentialContexts []string            `json:"additionalCredentialContexts,omitempty"`
	Env                          []string            `json:"env,omitempty"`
}

type AgentStatus struct {
	KnowledgeSetNames  []string                                 `json:"knowledgeSetNames,omitempty"`
	WorkspaceName      string                                   `json:"workspaceName,omitempty"`
	AliasAssigned      bool                                     `json:"aliasAssigned,omitempty"`
	AuthStatus         map[string]types.OAuthAppLoginAuthStatus `json:"authStatus,omitempty"`
	ToolInfo           map[string]types.ToolInfo                `json:"toolInfo,omitempty"`
	ObservedGeneration int64                                    `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Agent `json:"items"`
}

func CredentialTools(ctx context.Context, c kclient.Client, namespace string, toolReferenceName string) ([]string, error) {
	var toolReference ToolReference
	err := c.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: toolReferenceName}, &toolReference)
	if err != nil || toolReference.Status.Tool == nil {
		return nil, err
	}

	return toolReference.Status.Tool.Credentials, nil
}
