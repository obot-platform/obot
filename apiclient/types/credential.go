package types

type Credential struct {
	ContextID string   `json:"contextID,omitempty"`
	Name      string   `json:"name,omitempty"`
	EnvVars   []string `json:"envVars,omitempty"`
	ExpiresAt *Time    `json:"expiresAt,omitempty"`
}

type CredentialList List[Credential]

type ProjectCredential struct {
	ToolID    string `json:"toolID,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	Icon      string `json:"icon,omitempty"`
	Exists    bool   `json:"exists"`
	BaseAgent bool   `json:"baseAgent"`
}

type ProjectCredentialList List[ProjectCredential]
