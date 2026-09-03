import { type OrgGroup, type GroupRoleAssignment } from '$lib/services';

export interface GroupAssignment {
	group: OrgGroup;
	assignment: GroupRoleAssignment;
}
