package types

type WorkflowExecution struct {
	Metadata
	Workflow  WorkflowManifest `json:"workflow,omitempty"`
	StartTime Time             `json:"startTime"`
	EndTime   *Time            `json:"endTime"`
	Input     string           `json:"input"`
	Error     string           `json:"error,omitempty"`
	Warning   string           `json:"warning,omitempty"`
}

type WorkflowExecutionList List[WorkflowExecution]
