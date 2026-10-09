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
// Orthogonal roles:
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
	GroupMCP           = "mcp"
	GroupCompositeMCP  = "composite-mcp"
	GroupSkills        = "skills"
	GroupAPI           = "api"
	GroupLLM           = "llm"
	GroupDeviceScans   = "device-scans"
	GroupDeviceEnroll  = "device-enroll"
	GroupTunnel        = "obot-tunnel"
	GroupTunnelBridge  = "obot-tunnel-bridge"
	GroupTunnelPeer    = "obot-tunnel-peer"
	// GroupSCIM is the group of a SCIM connection's principal, which may only use that connection's SCIM endpoint.
	GroupSCIM = "obot-scim"
)

var (
	roleMap = map[Role]Role{
		RoleOwner:         RoleOwner | RoleAdmin | RolePowerUserPlus | RolePowerUser | RoleBasic,
		RoleAdmin:         RoleAdmin | RolePowerUserPlus | RolePowerUser | RoleBasic,
		RolePowerUserPlus: RolePowerUserPlus | RolePowerUser | RoleBasic,
		RolePowerUser:     RolePowerUser | RoleBasic,
		RoleBasic:         RoleBasic,
	}
)

type Role int

type User struct {
	Metadata
	Username               string   `json:"username,omitempty"`
	Role                   Role     `json:"role,omitempty"`
	EffectiveRole          Role     `json:"effectiveRole,omitempty"`
	Groups                 []string `json:"groups,omitempty"`
	AuthProviderGroups     []string `json:"authProviderGroups,omitempty"`
	ExplicitRole           bool     `json:"explicitRole,omitempty"`
	Email                  string   `json:"email,omitempty"`
	IconURL                string   `json:"iconURL,omitempty"`
	Timezone               string   `json:"timezone,omitempty"`
	CurrentAuthProvider    string   `json:"currentAuthProvider,omitempty"`
	LastActiveDay          Time     `json:"lastActiveDay,omitzero"`
	Internal               bool     `json:"internal,omitempty"`
	DailyInputTokensLimit  int      `json:"dailyInputTokensLimit,omitempty"`
	DailyOutputTokensLimit int      `json:"dailyOutputTokensLimit,omitempty"`
	DisplayName            string   `json:"displayName,omitempty"`
	DeletedAt              *Time    `json:"deletedAt,omitempty"`
	OriginalEmail          string   `json:"originalEmail,omitempty"`
	OriginalUsername       string   `json:"originalUsername,omitempty"`
	RequirePasswordChange  bool     `json:"requirePasswordChange,omitempty"`

	// Status is the user's lifecycle status. A disabled user keeps their account and data but is denied access.
	Status UserStatus `json:"status,omitempty"`
	// DisabledAt is when the user was disabled. It is set only while the user is disabled.
	DisabledAt *Time `json:"disabledAt,omitempty"`
	// DisabledReason explains why the user is disabled. It is set only while the user is disabled.
	DisabledReason string `json:"disabledReason,omitempty"`
	// ManagementSource is what controls the user's lifecycle status.
	ManagementSource UserManagementSource `json:"managementSource,omitempty"`
}

// UserStatus is a user's lifecycle status.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusDeleted  UserStatus = "deleted"
)

// UserManagementSource is what controls a user's lifecycle status.
type UserManagementSource string

const (
	// UserManagementSourceObot means Obot controls the user's status.
	UserManagementSourceObot UserManagementSource = "obot"
	// UserManagementSourceSCIM means an identity provider controls the user's status through SCIM.
	UserManagementSourceSCIM UserManagementSource = "scim"
)

type UserList List[User]

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

// ExtractBaseRole removes orthogonal role flags to get the base role
func (u Role) ExtractBaseRole() Role {
	return u &^ RoleAuditor
}

// HasAuditorRole checks if the Auditor flag is set in the role
func (u Role) HasAuditorRole() bool {
	return u&RoleAuditor != 0
}

func (u Role) RoleGroups() []string {
	return u.groups(true)
}

func (u Role) Groups() []string {
	return u.groups(false)
}

func (u Role) groups(onlyRoleGroups bool) []string {
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

		if !onlyRoleGroups {
			groups = append(groups, GroupAPI, GroupLLM, GroupSkills, GroupMCP, GroupDeviceScans)
		}
	}

	return groups
}
