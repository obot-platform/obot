package types

const (
	RoleUnknown Role = iota
	RoleAdmin

	// RoleBasic is the default role. Leaving a little space for future roles.
	RoleBasic Role = 10
)

type Role int

func (u Role) HasRole(role Role) bool {
	return u != RoleUnknown && role >= u
}

type User struct {
	Metadata
	Username      string `json:"username,omitempty"`
	Role          Role   `json:"role,omitempty"`
	ExplicitAdmin bool   `json:"explicitAdmin,omitempty"`
	Email         string `json:"email,omitempty"`
	IconURL       string `json:"iconURL,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	AuthProvider  string `json:"authProvider,omitempty"`
}

type UserList List[User]
