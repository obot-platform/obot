import { m } from '$lib/i18n';
import type { GroupReference } from '$lib/services/admin/types';

const referenceLabels: Record<GroupReference['kind'], () => string> = {
	accessControlRule: m.identity_access_scim_ref_access_control_rule,
	modelAccessPolicy: m.identity_access_scim_ref_model_access_policy,
	skillAccessRule: m.identity_access_scim_ref_skill_access_rule,
	messagePolicy: m.identity_access_scim_ref_message_policy,
	hostedAgentAccessRule: m.identity_access_scim_ref_hosted_agent_access_rule,
	groupRoleAssignment: m.identity_access_scim_ref_group_role,
	vmcpProfile: m.identity_access_scim_ref_vmcp
};

// describeGroupReference names what references a group, such as a policy that grants the group access.
export function describeGroupReference(ref: GroupReference) {
	const label = referenceLabels[ref.kind]?.() ?? ref.kind;
	if (ref.kind === 'groupRoleAssignment') {
		return ref.detail
			? m.identity_access_scim_ref_role_detail({ label, detail: ref.detail })
			: label;
	}
	let description = ref.displayName
		? m.identity_access_scim_ref_named({ label, name: ref.displayName })
		: m.identity_access_scim_ref_id({ label, id: ref.id });
	if (ref.detail) {
		description = m.identity_access_scim_ref_with_detail({ description, detail: ref.detail });
	}
	return description;
}
