package types

type Webhook struct {
	Metadata
	WebhookManifest
	AliasAssigned              bool  `json:"aliasAssigned,omitempty"`
	LastSuccessfulRunCompleted *Time `json:"lastSuccessfulRunCompleted,omitempty"`
	HasToken                   bool  `json:"hasToken,omitempty"`
}

type WebhookManifest struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Alias            string   `json:"alias"`
	Workflow         string   `json:"workflow"`
	Headers          []string `json:"headers"`
	Secret           string   `json:"secret"`
	ValidationHeader string   `json:"validationHeader"`
}

type WebhookList List[Webhook]
