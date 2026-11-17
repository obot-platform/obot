import { m } from '$lib/i18n';
import type { AccessControlRuleSubject } from '$lib/services';
import { subjectWildcardKind, type WildcardKind } from '$lib/subjectResolver';

// The rules for the subjects of a vMCP's profiles that span profiles (the server's
// ValidateWildcardProfiles and its agent rule). The server is the authority; these only report a
// conflict while the profile is being edited. The rules within one profile are not repeated here:
// the picker does not offer a wildcard that conflicts with the profile's other wildcards.
//
// - Everyone ("*") is named by at most one profile, and then no profile names all users or all agents.
// - Otherwise at most one profile names all users, and at most one names all agents.
// - An agent is named by at most one profile.

export interface OtherProfile {
	name: string;
	subjects: AccessControlRuleSubject[];
}

const TAKEN_MESSAGE: Record<WildcardKind, (name: string) => string> = {
	everyone: (name) => m.vmcps_profile_error_selector_taken({ name }),
	users: (name) => m.vmcps_profile_error_users_taken({ name }),
	agents: (name) => m.vmcps_profile_error_agents_taken({ name })
};

// profileSubjectConflict explains why subject cannot be added to a profile next to the other
// profiles of the vMCP, or returns undefined when it can.
export function profileSubjectConflict(
	subject: AccessControlRuleSubject,
	others: OtherProfile[],
	agentName: (agentId: string) => string
): string | undefined {
	const kind = subjectWildcardKind(subject);
	if (kind) {
		for (const other of others) {
			const otherKinds = new Set(
				other.subjects.map(subjectWildcardKind).filter((k): k is WildcardKind => k !== undefined)
			);
			if (otherKinds.has(kind)) return TAKEN_MESSAGE[kind](other.name);
			// All users in one profile and all agents in another is allowed; everyone is not combined
			// with either.
			if (kind === 'everyone' ? otherKinds.size > 0 : otherKinds.has('everyone')) {
				return m.vmcps_profile_error_wildcard_mixed({ name: other.name });
			}
		}
		return undefined;
	}

	if (subject.type === 'agent') {
		const other = others.find((candidate) =>
			candidate.subjects.some((s) => s.type === 'agent' && s.id === subject.id)
		);
		if (other) {
			return m.vmcps_profile_error_agent_taken({
				agent: agentName(subject.id),
				name: other.name
			});
		}
	}
	return undefined;
}

// profileIsAgentCapable mirrors the server's VMCPProfile.IsAgentCapable: a profile applies to agents
// when it names an agent, all agents, or everyone. Only such a profile has agent setup.
export function profileIsAgentCapable(
	subjects: Pick<AccessControlRuleSubject, 'type' | 'id'>[]
): boolean {
	return subjects.some(
		(subject) => subject.type === 'agent' || (subject.type === 'selector' && subject.id === '*')
	);
}
