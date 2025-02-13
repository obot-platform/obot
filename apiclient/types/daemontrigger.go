package types

type DaemonTrigger struct {
	Metadata
	DaemonTriggerManifest
	WorkflowExecutions []WorkflowExecution `json:"workflowExecutions,omitempty"`
}

type DaemonTriggerManifest struct {
	Workflow    string `json:"workflow"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Provider    string `json:"provider"`
	Options     string `json:"options"`
}

type DaemonTriggerList List[DaemonTrigger]
