package v1

import (
	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RunFinalizer                   = "obot.obot.ai/run"
	ThreadFinalizer                = "obot.obot.ai/thread"
	ToolReferenceFinalizer         = "obot.obot.ai/tool-reference"
	MCPServerFinalizer             = "obot.obot.ai/mcp-server"
	MCPServerCatalogEntryFinalizer = "obot.obot.ai/mcp-server-catalog-entry"
	MCPServerInstanceFinalizer     = "obot.obot.ai/mcp-server-instance"
	MCPSessionFinalizer            = "obot.obot.ai/mcp-session"
	OAuthClientFinalizer           = "obot.obot.ai/oauth-client"
	AccessControlRuleFinalizer     = "obot.obot.ai/access-control-rule"
	SystemMCPServerFinalizer       = "obot.obot.ai/system-mcp-server"
	NanobotAgentFinalizer          = "obot.obot.ai/nanobot-agent"
	ImagePullSecretFinalizer       = "obot.obot.ai/image-pull-secret"

	ModelProviderSyncAnnotation               = "obot.ai/model-provider-sync"
	AuthProviderSyncAnnotation                = "obot.ai/auth-provider-sync"
	MCPCatalogSyncAnnotation                  = "obot.ai/mcp-catalog-sync"
	SystemMCPCatalogSyncAnnotation            = "obot.ai/system-mcp-catalog-sync"
	SkillRepositorySyncAnnotation             = "obot.ai/skill-repository-sync"
	MCPServerCatalogEntrySyncAnnotation       = "obot.ai/mcp-server-catalog-entry-sync"
	SystemMCPServerCatalogEntrySyncAnnotation = "obot.ai/system-mcp-server-catalog-entry-sync"
)

var (
	_ fields.Fields = (*Run)(nil)
	_ DeleteRefs    = (*Run)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Run struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunSpec   `json:"spec,omitempty"`
	Status RunStatus `json:"status,omitempty"`
}

func (in *Run) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *Run) Get(field string) string {
	if in != nil {
		switch field {
		case "spec.threadName":
			return in.Spec.ThreadName
		case "spec.previousRunName":
			return in.Spec.PreviousRunName
		}
	}

	return ""
}

func (in *Run) FieldNames() []string {
	return []string{"spec.threadName", "spec.previousRunName"}
}

func (in *Run) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"PreviousRun", "Spec.PreviousRunName"},
		{"State", "Status.State"},
		{"Thread", "Spec.ThreadName"},
		{"Agent", "Spec.AgentName"},
		{"Workflow", "Spec.WorkflowName"},
		{"Step", "Spec.WorkflowStepName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type RunSpec struct {
	ThreadName           string            `json:"threadName,omitempty"`
	PreviousRunName      string            `json:"previousRunName,omitempty"`
	Input                string            `json:"input"`
	Env                  []string          `json:"env,omitempty"`
	Tool                 string            `json:"tool,omitempty"`
	ToolReferenceType    ToolReferenceType `json:"toolReferenceType,omitempty"`
	CredentialContextIDs []string          `json:"credentialContextIDs,omitempty"`
	Timeout              metav1.Duration   `json:"timeout,omitempty"`
	Username             string            `json:"username,omitempty"`
	CallDecisions        map[string]bool   `json:"callDecisions,omitempty"`
}

func (in *Run) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
	}
}

type RunStateState string

const (
	Creating RunStateState = "creating"
	Running  RunStateState = "running"
	Continue RunStateState = "continue"
	Finished RunStateState = "finished"
	Error    RunStateState = "error"
)

type RunStatus struct {
	Conditions             []metav1.Condition `json:"conditions,omitempty"`
	State                  RunStateState      `json:"state,omitempty"`
	Output                 string             `json:"output"`
	EndTime                metav1.Time        `json:"endTime,omitempty"`
	Error                  string             `json:"error,omitempty"`
	RequestedCallDecisions []string           `json:"requestedCallDecisions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Run `json:"items"`
}
