package types

type Webhook struct {
	Metadata
	WebhookManifest
	WebhookExternalStatus
}

type WebhookManifest struct {
	Description           string   `json:"description,omitempty"`
	RefName               string   `json:"refName,omitempty"`
	WorkflowName          string   `json:"workflowName,omitempty"`
	AfterWorkflowStepName string   `json:"afterWorkflowStepName,omitempty"`
	Headers               []string `json:"headers,omitempty"`
	Secret                string   `json:"secret,omitempty"`
	ValidationHeader      string   `json:"validationHeader,omitempty"`
}

type WebhookExternalStatus struct {
	RefNameAssigned bool `json:"refNameAssigned,omitempty"`
}

type WebhookList List[Webhook]
