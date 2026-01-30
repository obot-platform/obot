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
	return !in.Spec.Project && !in.Spec.Template && in.Spec.ParentThreadName != ""
}

func (in *Thread) IsSharedProject() bool {
	return in.Spec.Project && in.Spec.ParentThreadName != ""
}

func (in *Thread) IsProjectThread() bool {
	return in.Spec.Project
}

func (in *Thread) IsTemplate() bool {
	return in.Spec.Template
}

func (in *Thread) IsEditor() bool {
	return in.Spec.Project && in.Spec.ParentThreadName == ""
}

func (in *Thread) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

// SetLatestConfigRevision sets the latest config revision for the thread and returns true if the
// operation has changed the revision history.
func (in *Thread) SetLatestConfigRevision(revision string) bool {
	if len(in.Status.ConfigRevisions) == 0 {
		// No revision history, initialize the revision history
		in.Status.ConfigRevisions = []string{revision}
		return true
	}

	if in.Spec.Template {
		// Add the new revision to the end of the history (marking it as the latest revision)
		// Then deduplicate the history so that the latest revisions are kept and order is preserved
		// This corrects corrupted history and removes duplicate transient revisions (i.e. revisions that are generated during an upgrade, but aren't the target revision of the upgrade).
		deduped := append(slices.Clone(in.Status.ConfigRevisions), revision)
		deduped = dedupeRevisions(deduped)

		if !slices.Equal(in.Status.ConfigRevisions, deduped) {
			// The revision history has changed, set the field to the updated history
			in.Status.ConfigRevisions = deduped
			return true
		}

		// Revision history has stayed the same, no-op
		return false
	}

	// Non-template thread, if the revision differs from the latest revision or there's more than one
	// revision in the history, replace it with the latest revision.
	// For non-template threads, we only want to keep the latest revision in the history.
	var (
		revisionCount = len(in.Status.ConfigRevisions)
		latest        = in.Status.ConfigRevisions[revisionCount-1]
	)
	if latest != revision || revisionCount > 1 {
		in.Status.ConfigRevisions = []string{revision}
		return true
	}

	return false
}

// dedupeRevisions removes duplicate revisions from the given list of revisions.
// The revisions are deduplicated by keeping the latest revision and removing duplicates.
// The order of the revisions is preserved.
func dedupeRevisions(revisions []string) []string {
	// First pass: find the last index of each revision
	lastIndex := make(map[string]int, len(revisions))
	for i, revision := range revisions {
		lastIndex[revision] = i
	}

	// Second pass: build result keeping only items at their last index
	deduped := make([]string, 0, len(revisions))
	for i, revision := range revisions {
		if lastIndex[revision] == i {
			deduped = append(deduped, revision)
		}
	}

	return deduped
}

// GetLatestConfigRevision returns the latest config revision for the thread.
//
// This should always be the last element in the ConfigRevisions status field.
// If there are no revisions, an empty string is returned.
func (in *Thread) GetLatestConfigRevision() string {
	if len(in.Status.ConfigRevisions) == 0 {
		return ""
	}

	return in.Status.ConfigRevisions[len(in.Status.ConfigRevisions)-1]
}

// HasRevision determines if a given revision exists in the thread's ConfigRevisions status field.
//
// The first return argument will be set true IFF the revision exists in the thread's revision history.
// The second return argument will be set true IFF the revision both exists in the thread's revision history
// AND is the latest revision (i.e. the last element in the ConfigRevisions status field).
// (i.e. the last element) in the ConfigRevisions status field.
func (in *Thread) HasRevision(revision string) (found, latest bool) {
	revisions := in.Status.ConfigRevisions
	for i := len(revisions) - 1; i >= 0; i-- {
		if revisions[i] == revision {
			return true, i == len(revisions)-1
		}
	}

	return false, false
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
		case "spec.template":
			return strconv.FormatBool(in.Spec.Template)
		case "spec.parentThreadName":
			return in.Spec.ParentThreadName
		case "spec.sourceThreadName":
			return in.Spec.SourceThreadName
		}
	}
	return ""
}

func (in *Thread) FieldNames() []string {
	return []string{"spec.userUID", "spec.project", "spec.template", "spec.agentName", "spec.parentThreadName", "spec.sourceThreadName"}
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

type ThreadCapabilities struct {
	OnSlackMessage   bool             `json:"onSlackMessage"`
	OnDiscordMessage bool             `json:"onDiscordMessage"`
	OnEmail          *types.OnEmail   `json:"onEmail"`
	OnWebhook        *types.OnWebhook `json:"onWebhook"`
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
	// Project determines whether this thread is a project thread which essentially used as a scope and not really used as a thread to chat with
	Project bool `json:"project,omitempty"`
	// Env is the environment variable keys that expected to be set in the credential that matches the thread.Name
	Env []types.EnvVar `json:"env,omitempty"`
	// Ephemeral means that this thread is used once and then can be deleted after an interval
	Ephemeral bool `json:"ephemeral,omitempty"`
	// SystemTools are tools that are set on this thread but not visible to the user
	SystemTools []string `json:"systemTools,omitempty"`
	// Capabilities are the capabilities of this thread
	Capabilities ThreadCapabilities `json:"capabilities,omitempty"`

	// Project Model Settings

	// DefaultModelProvider is the provider for the default model for the project.
	DefaultModelProvider string `json:"defaultModelProvider,omitempty"`
	// DefaultModel is the default model for the project.
	DefaultModel string `json:"defaultModel,omitempty"`
	// Models is the list of models that users of the project may choose from.
	// It is a map of model provider to models.
	Models map[string][]string `json:"models,omitempty"`

	// Owners

	// WorkflowName is the workflow owner of the thread
	WorkflowName string `json:"workflowName,omitempty"`
	// WorkflowExecutionName is the workflow execution owner of the thread
	WorkflowExecutionName string `json:"workflowExecutionName,omitempty"`
	// KnowledgeSourceName is the knowledge source owner of the thread
	KnowledgeSourceName string `json:"remoteKnowledgeSourceName,omitempty"`
	// KnowledgeSetName is the knowledge set owner of the thread
	KnowledgeSetName string `json:"knowledgeSetName,omitempty"`
	// CronJobName is the cron job owner of the thread
	CronJobName string `json:"cronJobName,omitempty"`
	// OAuthAppLoginName is the oauth app login owner of the thread
	OAuthAppLoginName string `json:"oAuthAppLoginName,omitempty"`

	// Template determines whether this thread is a project template.
	//
	// When this field is true, the thread represents a snapshot of the source thread.
	Template bool `json:"template,omitempty"`

	// TargetConfigRevision is the target revision of the source thread to upgrade this thread to.
	//
	// This field tracks the latest revision of the source thread when UpgradeApproved is set.
	// It's compared with the last element in ConfigRevisions (i.e. the latest revision of this thread)
	// to determine when an upgrade has finished.
	// It is managed entirely by controllers and should never be directly exposed in user-facing APIs.
	TargetConfigRevision string `json:"targetConfigRevision,omitempty"`

	// UpgradeApproved indicates whether the user has approved an upgrade from the source thread.
	//
	// When this field is true, and an upgrade is available, the thread and its associated resources
	// will be updated to match the latest revision of the source thread.
	UpgradeApproved bool `json:"upgradeApproved,omitempty"`

	// ApprovedTools is a list of tool names that are auto-approved in this thread.
	// When a tool call matches a name in this list, it will be auto-approved.
	// Use "*" as a wildcard to auto-approve all tools.
	ApprovedTools []string `json:"approvedTools,omitempty"`
}

func (in *Thread) DeleteRefs() []Ref {
	refs := []Ref{
		{ObjType: &Agent{}, Name: in.Spec.AgentName},
		{ObjType: &WorkflowExecution{}, Name: in.Spec.WorkflowExecutionName},
		{ObjType: &Workflow{}, Name: in.Spec.WorkflowName},
		{ObjType: &CronJob{}, Name: in.Spec.CronJobName},
		{ObjType: &KnowledgeSource{}, Name: in.Spec.KnowledgeSourceName},
		{ObjType: &KnowledgeSet{}, Name: in.Spec.KnowledgeSetName},
		{ObjType: &Workspace{}, Name: in.Spec.WorkspaceName},
		{ObjType: &Workspace{}, Name: in.Status.WorkspaceName},
		{ObjType: &OAuthAppLogin{}, Name: in.Spec.OAuthAppLoginName},
		{ObjType: &Thread{}, Name: in.Spec.ParentThreadName},
	}

	if in.Spec.Template {
		refs = append(refs, Ref{
			ObjType: &Thread{},
			Name:    in.Spec.SourceThreadName,
		})
	}

	return refs
}

type ThreadStatus struct {
	LastRunName            string              `json:"lastRunName,omitempty"`
	CurrentRunName         string              `json:"currentRunName,omitempty"`
	LastRunState           RunStateState       `json:"lastRunState,omitempty"`
	LastUsedTime           metav1.Time         `json:"lastUsedTime,omitempty"`
	WorkflowState          types.WorkflowState `json:"workflowState,omitempty"`
	WorkspaceID            string              `json:"workspaceID,omitempty"`
	WorkspaceName          string              `json:"workspaceName,omitempty"`
	KnowledgeSetNames      []string            `json:"knowledgeSetNames,omitempty"`
	SharedKnowledgeSetName string              `json:"sharedKnowledgeSetName,omitempty"`
	// SharedWorkspaceName is used primarily to store the database content and is scoped to the project and shared across threads
	SharedWorkspaceName string `json:"sharedWorkspaceName,omitempty"`
	CopiedTasks         bool   `json:"copiedTasks,omitempty"`
	CopiedTools         bool   `json:"copiedTools,omitempty"`
	Created             bool   `json:"created,omitempty"`
	// WorkflowNamesFromIntegration is the workflow names created from external integration, like slack, discord..
	WorkflowNamesFromIntegration types.WorkflowNamesFromIntegration `json:"workflowNamesFromIntegration,omitempty"`

	// ConfigRevisions is a list of revisions of the thread's configuration.
	// Each revision is a digest of the thread's configuration at a given point in time and is used
	// to determine if the thread's configuration has diverged from the source thread's configuration.
	//
	// A given revision is computed by creating a stable hash of the thread's:
	// - introduction message
	// - starter messages
	// - tools
	// - tasks
	// - knowledge files
	// - model provider
	// - model
	// - prompt
	// - allowed MCP tools
	// - project MCP servers
	//
	// Revisions are sorted by creation time in ascending order, with the most recent revision at the end.
	//
	// For template threads (i.e. "project snapshots"), this ConfigRevisions will contain the sequence of revisions
	// taken after each successful update from the source thread.
	//
	// For project threads that own a template or are created from a template, ConfigRevisions
	// will contain a single element representing the latest digest of the thread's configuration.
	ConfigRevisions []string `json:"configRevisions,omitempty"`

	// UpgradeAvailable is a flag to indicate if an upgrade is available from the source thread.
	//
	// An upgrade is considered available if the source thread's configuration has changed since it was copied
	// into this thread AND the thread's configuration has not changed since it was copied.
	UpgradeAvailable bool `json:"upgradeAvailable,omitempty"`

	// UpgradeInProgress indicates if an upgrade from the source thread is in progress.
	UpgradeInProgress bool `json:"upgradeInProgress,omitempty"`

	// UpgradePublicID is the public ID of the template that this thread was copied from if any.
	UpgradePublicID string `json:"upgradePublicID,omitempty"`

	// LastUpgraded is a timestamp corresponding to the last time the thread was last successfully
	// upgraded from the source thread.
	LastUpgraded metav1.Time `json:"lastUpgraded,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Thread `json:"items"`
}
