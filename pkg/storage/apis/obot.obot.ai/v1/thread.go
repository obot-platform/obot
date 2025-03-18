package v1

import (
	"slices"
	"strconv"

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

func (in *Thread) IsProjectBased() bool {
	return in.Spec.Project || in.Spec.ParentThreadName != ""
}

func (in *Thread) IsUserThread() bool {
	return in.IsProjectBased() && !in.Spec.Project && in.Spec.ParentThreadName != ""
}

func (in *Thread) IsProjectThread() bool {
	return in.Spec.Project
}

func (in *Thread) IsEditor() bool {
	return in.Spec.Project && in.Spec.ParentThreadName == ""
}

func (in *Thread) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *Thread) Get(field string) string {
	if in != nil {
		switch field {
		case "spec.agentName":
			return in.Spec.AgentName
		case "spec.userUID":
			return in.Spec.UserID
		case "spec.project":
			return strconv.FormatBool(in.Spec.Project)
		case "spec.parentThreadName":
			return in.Spec.ParentThreadName
		}
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
	// ParentThreadName The scope of this thread will inherit the scope of the parent thread. The parent should always be a project thread.
	ParentThreadName string `json:"parentThreadName,omitempty"`
	// SourceThreadName is the thread that this thread was copied from
	SourceThreadName string `json:"sourceThreadName,omitempty"`
	// AgentName is the associated agent for this thread.
	AgentName string `json:"agentName,omitempty"`
	// WorkspaceName is the workspace that will be used by this thread and a new workspace will not be created
	WorkspaceName string `json:"workspaceName,omitempty"`
	// UserID is the user that created this thread (notice the json field name is userUID, we should probably rename that too at some point)
	UserID string `json:"userUID,omitempty"`
	// SystemTask means that this thread was created for non-user purpose for backend operations
	SystemTask bool `json:"systemTask,omitempty"`
	// Abort means that this thread should be aborted immediately
	Abort bool `json:"abort,omitempty"`
	// This thread is a project thread which essentially used as a scope and not really used as a thread to chat with
	Project bool `json:"project,omitempty"`
	// Env is the environment variable keys that expected to be set in the credential that matches the thread.Name
	Env []types.EnvVar `json:"env,omitempty"`
	// Ephemeral means that this thread is used once and then can be deleted after an interval
	Ephemeral bool `json:"ephemeral,omitempty"`
	// SystemTools are tools that are set on this thread but not visible to the user
	SystemTools []string `json:"systemTools,omitempty"`

	// Owners

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
	// OAuthAppLoginName is the oauth app login owner of the thread
	OAuthAppLoginName string `json:"oAuthAppLoginName,omitempty"`
}

func (in *Thread) DeleteRefs() []Ref {
	refs := []Ref{
		{ObjType: &Agent{}, Name: in.Spec.AgentName},
		{ObjType: &WorkflowExecution{}, Name: in.Spec.WorkflowExecutionName},
		{ObjType: &Workflow{}, Name: in.Spec.WorkflowName},
		{ObjType: &CronJob{}, Name: in.Spec.CronJobName},
		{ObjType: &Webhook{}, Name: in.Spec.WebhookName},
		{ObjType: &EmailReceiver{}, Name: in.Spec.EmailReceiverName},
		{ObjType: &KnowledgeSource{}, Name: in.Spec.KnowledgeSourceName},
		{ObjType: &KnowledgeSet{}, Name: in.Spec.KnowledgeSetName},
		{ObjType: &Workspace{}, Name: in.Spec.WorkspaceName},
		{ObjType: &Workspace{}, Name: in.Status.WorkspaceName},
		{ObjType: &OAuthAppLogin{}, Name: in.Spec.OAuthAppLoginName},
		{ObjType: &Thread{}, Name: in.Spec.ParentThreadName},
	}
	return refs
}

type ThreadStatus struct {
	LastRunName            string              `json:"lastRunName,omitempty"`
	CurrentRunName         string              `json:"currentRunName,omitempty"`
	LastRunState           RunStateState       `json:"lastRunState,omitempty"`
	WorkflowState          types.WorkflowState `json:"workflowState,omitempty"`
	WorkspaceID            string              `json:"workspaceID,omitempty"`
	WorkspaceName          string              `json:"workspaceName,omitempty"`
	KnowledgeSetNames      []string            `json:"knowledgeSetNames,omitempty"`
	SharedKnowledgeSetName string              `json:"sharedKnowledgeSetName,omitempty"`
	// SharedWorkspaceName is used primarily to store the database content and is scoped to the project and shared across threads
	SharedWorkspaceName string `json:"sharedWorkspaceName,omitempty"`
	CopiedTasks         bool   `json:"copiedTasks,omitempty"`
	Created             bool   `json:"created,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Thread `json:"items"`
}
