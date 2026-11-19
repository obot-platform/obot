import { m } from '$lib/i18n';
import type { AccessControlRuleSubject } from '$lib/services';

// The rule for the subjects of a vMCP's profiles that spans profiles (the server's agent rule): an
// agent is named by at most one profile. The server is the authority; this only reports a conflict
// while the profile is being edited.

export interface OtherProfile {
	name: string;
	subjects: AccessControlRuleSubject[];
}

// profileSubjectConflict explains why subject cannot be added to a profile next to the other
// profiles of the vMCP, or returns undefined when it can.
export function profileSubjectConflict(
	subject: AccessControlRuleSubject,
	others: OtherProfile[],
	agentName: (agentId: string) => string
): string | undefined {
	if (subject.type !== 'agent') return undefined;
	const other = others.find((candidate) =>
		candidate.subjects.some((s) => s.type === 'agent' && s.id === subject.id)
	);
	if (!other) return undefined;
	return m.vmcps_profile_error_agent_taken({ agent: agentName(subject.id), name: other.name });
}

// profileIsAgentCapable mirrors the server's VMCPProfile.IsAgentCapable: a profile applies to agents
// only when it names an agent by ID. Only such a profile has agent setup.
export function profileIsAgentCapable(
	subjects: Pick<AccessControlRuleSubject, 'type' | 'id'>[]
): boolean {
	return subjects.some((subject) => subject.type === 'agent');
}
