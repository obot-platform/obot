package types

type Project struct {
	Metadata
	ProjectManifest
	AssistantID                 string              `json:"assistantID,omitempty"`
	Editor                      bool                `json:"editor"`
	ParentID                    string              `json:"parentID,omitempty"`
	SourceProjectID             string              `json:"sourceProjectID,omitempty"`
	UserID                      string              `json:"userID,omitempty"`
	Capabilities                ProjectCapabilities `json:"capabilities,omitzero"`
	WorkflowNameFromIntegration string              `json:"workflowNameFromIntegration,omitempty"`
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
	DefaultModelProvider string              `json:"defaultModelProvider,omitempty"`
	DefaultModel         string              `json:"defaultModel,omitempty"`
	Models               map[string][]string `json:"models,omitempty"`
}

type ProjectList List[Project]
