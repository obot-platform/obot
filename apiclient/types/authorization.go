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
