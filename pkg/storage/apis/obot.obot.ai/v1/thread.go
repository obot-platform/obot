package v1

import (
	"slices"

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
	return slices.Contains(in.FieldNames(), field)
}

func (in *Thread) Get(field string) (value string) {
	switch field {
	case "spec.agentName":
		return in.Spec.AgentName
	case "spec.userUID":
		return in.Spec.UserUID
	case "spec.project":
		if in.Spec.Project {
			return "true"
		}
		return "false"
	case "spec.parentThreadName":
		return in.Spec.ParentThreadName
	}
	return ""
}

func (in *Thread) FieldNames() []string {
	return []string{"spec.userUID", "spec.project", "spec.agentName", "spec.parentThreadName"}
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
	Manifest types.ThreadManifest `json:"manifest,omitempty"`
	// ThreadTemplateName is the thread template that will be used to create this thread (for the knowledge and file workspaces)
	ThreadTemplateName string `json:"threadTemplateName,omitempty"`
	// ParentThreadName The scope of this thread will inherit the scope of the parent thread
	ParentThreadName string `json:"parentThreadName,omitempty"`
	// AgentName is the associated agent for this thread. This value could change between multiple runs
	AgentName string `json:"agentName,omitempty"`
	// WorkflowName is the workflow owner of the thread
	WorkflowName string `json:"workflowName,omitempty"`
	// WorkflowExecutionName is the workflow execution owner of the thread
	WorkflowExecutionName string `json:"workflowExecutionName,omitempty"`
	// KnowledgeSourceName is the knowledge source owner of the thread
	KnowledgeSourceName string `json:"remoteKnowledgeSourceName,omitempty"`
	// KnowledgeSetName is the knowledge set owner of the thread
	KnowledgeSetName string `json:"knowledgeSetName,omitempty"`
	// WebhookName is the webhook owner of the thread
	WebhookName string `json:"webhookName,omitempty"`
	// EmailReceiverName is the email receiver owner of the thread
	EmailReceiverName string `json:"emailReceiverName,omitempty"`
	// CronJobName is the cron job owner of the thread
	CronJobName string `json:"cronJobName,omitempty"`
	// WorkspaceName is the workspace that will be used by this thread and a new workspace will not be created
	WorkspaceName      string   `json:"workspaceName,omitempty"`
	FromWorkspaceNames []string `json:"fromWorkspaceNames,omitempty"`
	OAuthAppLoginName  string   `json:"oAuthAppLoginName,omitempty"`
	// UserUID is the user that created this thread
	UserUID            string `json:"userUID,omitempty"`
	TextEmbeddingModel string `json:"textEmbeddingModel,omitempty"`
	SystemTask         bool   `json:"systemTask,omitempty"`
	Abort              bool   `json:"abort,omitempty"`
	// This thread is a project thread which essentially used as a scope and not really used as a thread to chat with
	Project bool `json:"project,omitempty"`
	// Env is the environment variable keys that expected to be set in the credential that matches the thread.Name
	Env []string `json:"env,omitempty"`
	// Ephemeral means that this thread is used once and then can be deleted
	Ephemeral bool `json:"ephemeral,omitempty"`
}

func (in *Thread) DeleteRefs() []Ref {
	refs := []Ref{
		{ObjType: &Agent{}, Name: in.Spec.AgentName},
		{ObjType: &WorkflowExecution{}, Name: in.Spec.WorkflowExecutionName},
		{ObjType: &Workflow{}, Name: in.Spec.WorkflowName},
		{ObjType: &CronJob{}, Name: in.Spec.CronJobName},
		{ObjType: &Webhook{}, Name: in.Spec.WebhookName},
		{ObjType: &EmailReceiver{}, Name: in.Spec.EmailReceiverName},
		{ObjType: &Thread{}, Name: in.Status.PreviousThreadName},
		{ObjType: &KnowledgeSource{}, Name: in.Spec.KnowledgeSourceName},
		{ObjType: &KnowledgeSet{}, Name: in.Spec.KnowledgeSetName},
		{ObjType: &Workspace{}, Name: in.Spec.WorkspaceName},
		{ObjType: &Workspace{}, Name: in.Status.WorkspaceName},
		{ObjType: &OAuthAppLogin{}, Name: in.Spec.OAuthAppLoginName},
	}
	return refs
}

type ThreadStatus struct {
	LastRunName        string                   `json:"lastRunName,omitempty"`
	CurrentRunName     string                   `json:"currentRunName,omitempty"`
	LastRunState       gptscriptclient.RunState `json:"lastRunState,omitempty"`
	WorkflowState      types.WorkflowState      `json:"workflowState,omitempty"`
	WorkspaceID        string                   `json:"workspaceID,omitempty"`
	WorkspaceName      string                   `json:"workspaceName,omitempty"`
	PreviousThreadName string                   `json:"previousThreadName,omitempty"`
	KnowledgeSetNames  []string                 `json:"knowledgeSetNames,omitempty"`
	TemplateLoaded     bool                     `json:"templateLoaded,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Thread `json:"items"`
}
