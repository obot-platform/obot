package types

type Project struct {
	Metadata
	ProjectManifest
	AssistantID                  string                       `json:"assistantID,omitempty"`
	Editor                       bool                         `json:"editor"`
	ParentID                     string                       `json:"parentID,omitempty"`
	SourceProjectID              string                       `json:"sourceProjectID,omitempty"`
	UserID                       string                       `json:"userID,omitempty"`
	WorkflowNamesFromIntegration WorkflowNamesFromIntegration `json:"workflowNamesFromIntegration,omitempty"`
}

type WorkflowNamesFromIntegration struct {
	SlackWorkflowName   string `json:"slackWorkflowName,omitempty"`
	DiscordWorkflowName string `json:"discordWorkflowName,omitempty"`
	EmailWorkflowName   string `json:"emailWorkflowName,omitempty"`
	WebhookWorkflowName string `json:"webhookWorkflowName,omitempty"`
}

type ProjectCapabilities struct {
	OnSlackMessage   bool       `json:"onSlackMessage,omitempty"`
	OnDiscordMessage bool       `json:"onDiscordMessage,omitempty"`
	OnEmail          *OnEmail   `json:"onEmail,omitempty"`
	OnWebhook        *OnWebhook `json:"onWebhook,omitempty"`
}

type OnEmail struct {
	EmailReceiverManifest `json:",inline"`
}

type OnWebhook struct {
	WebhookManifest `json:",inline"`
}

type ProjectManifest struct {
	ThreadManifest
	Capabilities         *ProjectCapabilities `json:"capabilities,omitempty"`
	DefaultModelProvider string               `json:"defaultModelProvider,omitempty"`
	DefaultModel         string               `json:"defaultModel,omitempty"`
	Models               map[string][]string  `json:"models,omitempty"`
}

type ProjectList List[Project]
