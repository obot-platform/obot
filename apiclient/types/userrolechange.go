package types

type UserRoleChange struct {
	Metadata
	UserID  uint `json:"userID,omitempty"`
	OldRole Role `json:"oldRole,omitempty"`
	NewRole Role `json:"newRole,omitempty"`
}

type UserRoleChangeList List[UserRoleChange]