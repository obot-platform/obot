package types

// Role represents user privilege levels as bit flags that can be combined.
//
// Base roles (mutually exclusive):
//   - RoleBasic (4): Basic authenticated user
//   - RoleOwner (8): Full system owner
//   - RoleAdmin (16): Administrative user
//   - RolePowerUserPlus (64): Enhanced power user with ACL and MCP server management
//   - RolePowerUser (128): Standard power user with workspace
//
// Orthogonal role:
//   - RoleAuditor (32): Can view audit logs and sensitive data (can be combined with any base role)
//
// Examples of combined roles:
//   - RoleAdmin | RoleAuditor (48): Admin with audit access
//   - RolePowerUser | RoleAuditor (160): Power user with audit access
//   - RoleOwner | RoleAuditor (40): Owner with audit access
const (
	// We're start with 4 here so that our old and new roles are mutually exclusive.
	// This makes migrations and detecting when migrations are needed easier.
	RoleBasic Role = 1 << (iota + 2)
	RoleOwner
	RoleAdmin
	RoleAuditor
	RolePowerUserPlus
	RolePowerUser

	RoleUnknown Role = 0

	GroupOwner         = "owner"
	GroupAdmin         = "admin"
	GroupAuditor       = "auditor"
	GroupPowerUserPlus = "power-user-plus"
	GroupPowerUser     = "power-user"
	GroupBasic         = "basic"
	GroupAuthenticated = "authenticated"
	GroupAPIKey        = "api-key" // API key users have restricted access to MCP-connect routes only
)

type Role int

var (
	roleMap = map[Role]Role{
		RoleOwner:         RoleOwner | RoleAdmin | RolePowerUserPlus | RolePowerUser | RoleBasic,
		RoleAdmin:         RoleAdmin | RolePowerUserPlus | RolePowerUser | RoleBasic,
		RolePowerUserPlus: RolePowerUserPlus | RolePowerUser | RoleBasic,
		RolePowerUser:     RolePowerUser | RoleBasic,
		RoleBasic:         RoleBasic,
	}
)

func (u Role) HasRole(role Role) bool {
	for _, r := range roleMap {
		if r, ok := roleMap[u&r]; ok && r&role == role {
			return true
		}
	}
	return u&role == role
}

func (u Role) IsExactBaseRole(role Role) bool {
	return u&role == role
}

func (u Role) SwitchBaseRole(role Role) Role {
	return role | (u & RoleAuditor)
}

// ExtractBaseRole removes the Auditor flag from a role to get the base role
func (u Role) ExtractBaseRole() Role {
	return u &^ RoleAuditor
}

// HasAuditorRole checks if the Auditor flag is set in the role
func (u Role) HasAuditorRole() bool {
	return u&RoleAuditor != 0
}

func (u Role) Groups() []string {
	var groups []string
	if u.HasRole(RoleOwner) {
		groups = append(groups, GroupOwner)
	}
	if u.HasRole(RoleAdmin) {
		groups = append(groups, GroupAdmin)
	}
	if u.HasRole(RolePowerUserPlus) {
		groups = append(groups, GroupPowerUserPlus)
	}
	if u.HasRole(RolePowerUser) {
		groups = append(groups, GroupPowerUser)
	}
	if u.HasRole(RoleBasic) {
		groups = append(groups, GroupBasic)
	}
	if u.HasRole(RoleAuditor) {
		groups = append(groups, GroupAuditor)
	}
	if u != RoleUnknown {
		groups = append(groups, GroupAuthenticated)
	}

	return groups
}

type User struct {
	Metadata
	Username                   string   `json:"username,omitempty"`
	Role                       Role     `json:"role,omitempty"`
	EffectiveRole              Role     `json:"effectiveRole,omitempty"`
	Groups                     []string `json:"groups,omitempty"`
	ExplicitRole               bool     `json:"explicitRole,omitempty"`
	Email                      string   `json:"email,omitempty"`
	IconURL                    string   `json:"iconURL,omitempty"`
	Timezone                   string   `json:"timezone,omitempty"`
	CurrentAuthProvider        string   `json:"currentAuthProvider,omitempty"`
	LastActiveDay              Time     `json:"lastActiveDay,omitzero"`
	Internal                   bool     `json:"internal,omitempty"`
	DailyPromptTokensLimit     int      `json:"dailyPromptTokensLimit,omitempty"`
	DailyCompletionTokensLimit int      `json:"dailyCompletionTokensLimit,omitempty"`
	DisplayName                string   `json:"displayName,omitempty"`
	DeletedAt                  *Time    `json:"deletedAt,omitempty"`
	OriginalEmail              string   `json:"originalEmail,omitempty"`
	OriginalUsername           string   `json:"originalUsername,omitempty"`
	AutonomousToolUseEnabled   *bool    `json:"autonomousToolUseEnabled,omitempty"`
}

type UserList List[User]
