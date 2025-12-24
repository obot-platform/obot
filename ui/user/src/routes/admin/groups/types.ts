import type { OrgGroup, GroupRoleAssignment } from '$lib/services/admin/types';

export interface GroupAssignment {
	group: OrgGroup;
	assignment: GroupRoleAssignment;
}
