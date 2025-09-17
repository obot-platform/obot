package types

const (
	RoleBasic Role = 1 << (iota + 1)
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
	return role | (role & RoleAuditor)
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
}

type UserList List[User]
