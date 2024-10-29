package v1

import (
	"context"

	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KnowledgeSourceSpec   `json:"spec,omitempty"`
	Status KnowledgeSourceStatus `json:"status,omitempty"`
}

func (in *KnowledgeSource) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"KnowledgeSet", "Spec.KnowledgeSetName"},
		{"State", "{{.PublicState}}"},
		{"Type", "{{.Spec.Manifest.KnowledgeSourceInput.GetType}}"},
		{"Status", "Status.Status"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *KnowledgeSource) PublicState() types.KnowledgeSourceState {
	if in.Status.SyncState == "" {
		return types.KnowledgeSourceStatePending
	}
	return in.Status.SyncState
}

func (in *KnowledgeSource) CredentialTool(ctx context.Context, c kclient.Client) (string, bool, error) {
	var tool string
	switch in.Spec.Manifest.GetType() {
	case types.KnowledgeSourceTypeOneDrive:
		tool = "onedrive"
	case types.KnowledgeSourceTypeNotion:
		tool = "notion"
	}

	if tool == "" {
		return "", false, nil
	}

	var toolReference ToolReference
	err := c.Get(ctx, kclient.ObjectKey{Namespace: in.Namespace, Name: tool + "-data-source"}, &toolReference)
	if err != nil || toolReference.Status.Tool == nil {
		return "", true, err
	}

	return toolReference.Status.Tool.Credential, true, nil
}

func (in *KnowledgeSource) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *KnowledgeSource) Get(field string) string {
	if in == nil {
		return ""
	}

	switch field {
	case "spec.knowledgeSetName":
		return in.Spec.KnowledgeSetName
	}

	return ""
}

func (*KnowledgeSource) FieldNames() []string {
	return []string{"spec.knowledgeSetName"}
}

func (in *KnowledgeSource) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &KnowledgeSet{}, Name: in.Spec.KnowledgeSetName},
	}
}

type KnowledgeSourceSpec struct {
	Manifest         types.KnowledgeSourceManifest `json:"manifest,omitempty"`
	KnowledgeSetName string                        `json:"knowledgeSetName,omitempty"`
	SyncGeneration   int64                         `json:"syncGeneration,omitempty"`
}

type KnowledgeSourceStatus struct {
	WorkspaceName     string                        `json:"workspaceName,omitempty"`
	ThreadName        string                        `json:"threadName,omitempty"`
	RunName           string                        `json:"runName,omitempty"`
	SyncState         types.KnowledgeSourceState    `json:"syncState,omitempty"`
	Status            string                        `json:"status,omitempty"`
	SyncDetails       []byte                        `json:"syncDetails,omitempty"`
	Error             string                        `json:"error,omitempty"`
	SyncGeneration    int64                         `json:"syncGeneration,omitempty"`
	LastSyncStartTime metav1.Time                   `json:"lastSyncStartTime,omitempty"`
	LastSyncEndTime   metav1.Time                   `json:"lastSyncEndTime,omitempty"`
	NextSyncTime      metav1.Time                   `json:"nextSyncTime,omitempty"`
	Auth              types.OAuthAppLoginAuthStatus `json:"auth,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []KnowledgeSource `json:"items"`
}
