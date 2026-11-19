import type { AccessControlRuleSubject, VMCPProfile } from '$lib/services';

// profileIsAgentCapable mirrors the server's VMCPProfile.IsAgentCapable: a profile applies to agents
// only when it names an agent by ID. An agent may be named by several profiles of a vMCP, and gets the
// union of what they grant, as a user does.
export function profileIsAgentCapable(
	subjects: Pick<AccessControlRuleSubject, 'type' | 'id'>[]
): boolean {
	return subjects.some((subject) => subject.type === 'agent' && subject.id !== '*');
}

// vmcpNamesAgent reports whether any saved profile of a vMCP names an agent, which is when the vMCP has
// agent setup.
export function vmcpNamesAgent(profiles: VMCPProfile[] | undefined): boolean {
	return (profiles ?? []).some((profile) => profileIsAgentCapable(profile.subjects ?? []));
}
