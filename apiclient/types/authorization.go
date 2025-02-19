package types

type AgentAuthorization struct {
	AgentAuthorizationManifest
	User *User `json:"user,omitempty"`
}

type AgentAuthorizationManifest struct {
	UserID  string `json:"userID,omitempty"`
	AgentID string `json:"agentId,omitempty"`
}

type AuthorizationList List[AgentAuthorization]

type ThreadAuthorization struct {
	ThreadAuthorizationManifest
}

type ThreadAuthorizationManifest struct {
	UserID   string `json:"userID,omitempty"`
	ThreadID string `json:"threadID,omitempty"`
}

type ThreadAuthorizationList List[ThreadAuthorization]

type ProjectAuthorization struct {
	Project  *Project `json:"project,omitempty"`
	Target   string   `json:"target,omitempty"`
	Accepted bool     `json:"accepted,omitempty"`
}

type ProjectAuthorizationList List[ProjectAuthorization]

type TemplateAuthorization struct {
	TemplateAuthorizationManifest
}

type TemplateAuthorizationManifest struct {
	UserID     string `json:"userID,omitempty"`
	TemplateID string `json:"templateID,omitempty"`
}

type TemplateAuthorizationList List[TemplateAuthorization]
