package types

// GroupRoleAssignment represents a role assigned to all members of an authentication provider group.
// Roles can be combined using bitwise OR, e.g., Admin | Auditor.
//
// Permission levels:
//   - Owners can assign any role or combination: Owner, Auditor, Admin, PowerUserPlus, PowerUser
//   - Admins can assign: Admin, PowerUserPlus, PowerUser (NO Owner or Auditor)
//
// Common role combinations (Owner-only):
//   - Admin | Auditor (48)
//   - PowerUserPlus | Auditor (96)
//   - PowerUser | Auditor (160)
//   - Owner | Auditor (40)
type GroupRoleAssignment struct {
	// GroupName is the authentication provider group ID (e.g., "github:org/team", "entra:group-uuid"), NOT the display name
	GroupName string `json:"groupName"`

	// Role is the role(s) assigned to all group members. Can be a single role or combination using bitwise OR.
	// Valid values: Owner(8), Admin(16), Auditor(32), PowerUserPlus(64), PowerUser(128)
	Role Role `json:"role"`

	// Description is an optional explanation for this role assignment
	Description string `json:"description,omitempty"`
}

// GroupRoleAssignmentList is a list of group role assignments.
type GroupRoleAssignmentList List[GroupRoleAssignment]
