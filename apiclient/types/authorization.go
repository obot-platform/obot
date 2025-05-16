package types

type ThreadAuthorization struct {
	ThreadAuthorizationManifest
}

type ThreadAuthorizationManifest struct {
	UserID   string `json:"userID,omitempty"`
	ThreadID string `json:"threadID,omitempty"`
}

type ThreadAuthorizationList List[ThreadAuthorization]

type ProjectInvitationManifest struct {
	Code    string                  `json:"code,omitempty"`
	Project *Project                `json:"project,omitempty"`
	Status  ProjectInvitationStatus `json:"status,omitempty"`
	Created string                  `json:"created,omitempty"`
}

type ProjectInvitationStatus string

const (
	ProjectInvitationStatusPending  ProjectInvitationStatus = "pending"
	ProjectInvitationStatusAccepted ProjectInvitationStatus = "accepted"
	ProjectInvitationStatusRejected ProjectInvitationStatus = "rejected"
	ProjectInvitationStatusExpired  ProjectInvitationStatus = "expired"
)

type ProjectMember struct {
	UserID  string `json:"userID,omitempty"`
	IconURL string `json:"iconURL,omitempty"`
	Email   string `json:"email,omitempty"`
	IsOwner bool   `json:"isOwner,omitempty"`
}

type TemplateAuthorization struct {
	TemplateAuthorizationManifest
}

type TemplateAuthorizationManifest struct {
	UserID     string `json:"userID,omitempty"`
	TemplateID string `json:"templateID,omitempty"`
}

type TemplateAuthorizationList List[TemplateAuthorization]
