package types

// NanobotAgent represents a nanobot workflow in the API
type NanobotAgent struct {
	Metadata
	NanobotAgentManifest
	UserID      string `json:"userID,omitempty"`
	ProjectV2ID string `json:"projectV2ID,omitempty"`
	ConnectURL  string `json:"connectURL,omitempty"`
}

// NanobotAgentManifest contains the user-editable fields for a nanobot workflow
type NanobotAgentManifest struct {
	DisplayName  string `json:"displayName,omitempty"`
	Description  string `json:"description,omitempty"`
	DefaultAgent string `json:"defaultAgent,omitempty"`
}

// NanobotAgentList is a list of nanobot workflows
type NanobotAgentList List[NanobotAgent]
