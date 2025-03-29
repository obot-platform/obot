package types

type SlackTrigger struct {
	Metadata
	SlackTriggerManifest
}

// SlackTriggerManifest defines the configuration for a Slack trigger
type SlackTriggerManifest struct {
	// WorkflowName is the name of the workflow to trigger
	WorkflowName string `json:"workflowName"`

	// ThreadName is the name of the project thread where the trigger will be created
	ThreadName string `json:"threadName"`
}

type SlackTriggerList List[SlackTrigger]
