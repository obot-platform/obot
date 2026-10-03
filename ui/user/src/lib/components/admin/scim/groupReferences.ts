import type { GroupReference } from '$lib/services/admin/types';

const referenceLabels: Record<GroupReference['kind'], string> = {
	accessControlRule: 'access control rule',
	modelAccessPolicy: 'model access policy',
	skillAccessRule: 'skill access rule',
	messagePolicy: 'message policy',
	hostedAgentAccessRule: 'hosted agent access rule',
	publishedArtifact: 'published artifact',
	groupRoleAssignment: 'group role assignment',
	vmcpProfile: 'virtual MCP server'
};

// describeGroupReference names what references a group, such as a policy that grants the group access.
export function describeGroupReference(ref: GroupReference) {
	const label = referenceLabels[ref.kind] ?? ref.kind;
	if (ref.kind === 'groupRoleAssignment') {
		return ref.detail ? `${label} (${ref.detail})` : label;
	}
	let description = ref.displayName ? `${label} “${ref.displayName}”` : `${label} ${ref.id}`;
	if (ref.detail) {
		description += `, ${ref.detail}`;
	}
	return description;
}
