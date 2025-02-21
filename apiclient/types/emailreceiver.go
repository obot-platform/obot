package types

type EmailReceiver struct {
	Metadata
	EmailReceiverManifest
	AliasAssigned *bool  `json:"aliasAssigned,omitempty"`
	EmailAddress  string `json:"emailAddress,omitempty"`
}

type EmailReceiverManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Alias          string   `json:"alias,omitempty"`
	WorkflowName   string   `json:"workflowName"`
	AllowedSenders []string `json:"allowedSenders,omitempty"`
}

type EmailReceiverList List[EmailReceiver]
