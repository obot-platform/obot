package v1

import (
	"context"

	"github.com/otto8-ai/nah/pkg/fields"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ fields.Fields = (*Agent)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
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
	Manifest            types.AgentManifest `json:"manifest,omitempty"`
	InputFilters        []string            `json:"inputFilters,omitempty"`
	CredentialContextID string              `json:"credentialContextID,omitempty"`
}

type AgentStatus struct {
	KnowledgeSetNames []string                  `json:"knowledgeSetNames,omitempty"`
	WorkspaceName     string                    `json:"workspaceName,omitempty"`
	External          types.AgentExternalStatus `json:"external,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Agent `json:"items"`
}

func CredentialTool(ctx context.Context, c kclient.Client, namespace string, toolReferenceName string) (string, error) {
	var toolReference ToolReference
	err := c.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: toolReferenceName}, &toolReference)
	if err != nil || toolReference.Status.Tool == nil {
		return "", err
	}

	return toolReference.Status.Tool.Credential, nil
}
