package types

type WorkflowState string

const (
	WorkflowStatePending  WorkflowState = "Pending"
	WorkflowStateRunning  WorkflowState = "Running"
	WorkflowStateError    WorkflowState = "Error"
	WorkflowStateComplete WorkflowState = "Complete"
	WorkflowStateSubCall  WorkflowState = "SubCall"
	WorkflowStateBlocked  WorkflowState = "Blocked"
)

func (in WorkflowState) IsBlocked() bool {
	return in == WorkflowStateBlocked || in == WorkflowStateError
}

func (in WorkflowState) IsTerminal() bool {
	return in == WorkflowStateComplete || in == WorkflowStateError
}

type Thread struct {
	Metadata
	ThreadManifest
	AssistantID     string   `json:"assistantID,omitempty"`
	TaskID          string   `json:"taskID,omitempty"`
	TaskRunID       string   `json:"taskRunID,omitempty"`
	WebhookID       string   `json:"webhookID,omitempty"`
	EmailReceiverID string   `json:"emailReceiverID,omitempty"`
	State           string   `json:"state,omitempty"`
	LastRunID       string   `json:"lastRunID,omitempty"`
	CurrentRunID    string   `json:"currentRunID,omitempty"`
	ProjectID       string   `json:"projectID,omitempty"`
	UserID          string   `json:"userID,omitempty"`
	Abort           bool     `json:"abort,omitempty"`
	SystemTask      bool     `json:"systemTask,omitempty"`
	Ephemeral       bool     `json:"ephemeral,omitempty"`
	Project         bool     `json:"project,omitempty"`
	Env             []string `json:"env,omitempty"`
	Ready           bool     `json:"ready,omitempty"`
}

type ThreadList List[Thread]

type ThreadManifest struct {
	Name                 string            `json:"name"`
	Tools                []string          `json:"tools,omitempty"`
	Description          string            `json:"description,omitempty"`
	Icons                *AgentIcons       `json:"icons"`
	Prompt               string            `json:"prompt"`
	KnowledgeDescription string            `json:"knowledgeDescription"`
	IntroductionMessage  string            `json:"introductionMessage"`
	StarterMessages      []string          `json:"starterMessages"`
	SharedTasks          []string          `json:"sharedTasks,omitempty"`
	WebsiteKnowledge     *WebsiteKnowledge `json:"websiteKnowledge,omitempty"`
}
