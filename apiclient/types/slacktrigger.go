package types

type SlackTrigger struct {
	Metadata
	SlackTriggerManifest
}

// SlackTriggerManifest defines the configuration for a Slack trigger
type SlackTriggerManifest struct {
	// WorkflowName is the name of the workflow to trigger
	WorkflowName string `json:"workflowName"`

	// TeamID is the Slack team/workspace ID
	TeamID string `json:"teamID"`
}

type SlackTriggerList List[SlackTrigger]
