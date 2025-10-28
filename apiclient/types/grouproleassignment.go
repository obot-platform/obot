package types

// GroupRoleAssignment represents a role assignment to an auth provider group.
type GroupRoleAssignment struct {
	// GroupName is the name of the auth provider group
	GroupName string `json:"groupName"`

	// Role is the role assigned to all members of this group
	Role Role `json:"role"`

	// Description explains why this assignment exists
	Description string `json:"description,omitempty"`
}

// GroupRoleAssignmentList is a list of group role assignments.
type GroupRoleAssignmentList List[GroupRoleAssignment]
