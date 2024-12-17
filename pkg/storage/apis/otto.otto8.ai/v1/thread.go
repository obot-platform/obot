package v1

import (
	gptscriptclient "github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Thread struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreadSpec   `json:"spec,omitempty"`
	Status ThreadStatus `json:"status,omitempty"`
}

func (in *Thread) Has(field string) (exists bool) {
	switch field {
	case "spec.userUID":
		return true
	}
	return false
}

func (in *Thread) Get(field string) (value string) {
	switch field {
	case "spec.userUID":
		return in.Spec.UserUID
	}
	return ""
}

func (in *Thread) FieldNames() []string {
	return []string{"spec.userUID"}
}

func (in *Thread) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"CurrentRun", "Status.CurrentRunName"},
		{"LastRun", "Status.LastRunName"},
		{"LastRunState", "Status.LastRunState"},
		{"WorkflowState", "Status.WorkflowState"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type ThreadSpec struct {
	Manifest              types.ThreadManifest `json:"manifest,omitempty"`
	ParentThreadName      string               `json:"parentThreadName,omitempty"`
	AgentName             string               `json:"agentName,omitempty"`
	AgentAlias            string               `json:"agentAlias,omitempty"`
	WorkflowName          string               `json:"workflowName,omitempty"`
	WorkflowExecutionName string               `json:"workflowExecutionName,omitempty"`
	KnowledgeSourceName   string               `json:"remoteKnowledgeSourceName,omitempty"`
	KnowledgeSetName      string               `json:"knowledgeSetName,omitempty"`
	WebhookName           string               `json:"webhookName,omitempty"`
	EmailReceiverName     string               `json:"emailReceiverName,omitempty"`
	CronJobName           string               `json:"cronJobName,omitempty"`
	WorkspaceName         string               `json:"workspaceName,omitempty"`
	FromWorkspaceNames    []string             `json:"fromWorkspaceNames,omitempty"`
	OAuthAppLoginName     string               `json:"oAuthAppLoginName,omitempty"`
	UserUID               string               `json:"userUID,omitempty"`
	TextEmbeddingModel    string               `json:"textEmbeddingModel,omitempty"`
	SystemTask            bool                 `json:"systemTask,omitempty"`
	Abort                 bool                 `json:"abort,omitempty"`
	Env                   []string             `json:"env,omitempty"`
}

func (in *Thread) DeleteRefs() []Ref {
	refs := []Ref{
		{ObjType: &WorkflowExecution{}, Name: in.Spec.WorkflowExecutionName},
		{ObjType: &Workflow{}, Name: in.Spec.WorkflowName},
		{ObjType: &CronJob{}, Name: in.Spec.CronJobName},
		{ObjType: &Webhook{}, Name: in.Spec.WebhookName},
		{ObjType: &EmailReceiver{}, Name: in.Spec.EmailReceiverName},
		{ObjType: &Thread{}, Name: in.Status.PreviousThreadName},
		{ObjType: &KnowledgeSource{}, Name: in.Spec.KnowledgeSourceName},
		{ObjType: &KnowledgeSet{}, Name: in.Spec.KnowledgeSetName},
		{ObjType: &Workspace{}, Name: in.Spec.WorkspaceName},
		{ObjType: &OAuthAppLogin{}, Name: in.Spec.OAuthAppLoginName},
	}
	for _, name := range in.Spec.FromWorkspaceNames {
		refs = append(refs, Ref{ObjType: &Workspace{}, Name: name})
	}
	return refs
}

type ThreadStatus struct {
	LastRunName        string                   `json:"lastRunName,omitempty"`
	CurrentRunName     string                   `json:"currentRunName,omitempty"`
	LastRunState       gptscriptclient.RunState `json:"lastRunState,omitempty"`
	WorkflowState      types.WorkflowState      `json:"workflowState,omitempty"`
	WorkspaceID        string                   `json:"workspaceID,omitempty"`
	PreviousThreadName string                   `json:"previousThreadName,omitempty"`
	KnowledgeSetNames  []string                 `json:"knowledgeSetNames,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Thread `json:"items"`
}
