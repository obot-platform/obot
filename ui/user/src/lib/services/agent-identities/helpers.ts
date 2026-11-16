import { m } from '$lib/i18n';
import type { AgentIdentity, AgentIdentityStatus, AgentAPIKey } from './types';

// Agents appear in the audit and usage APIs as the storage encoding of a principal, "agent:<id>",
// next to users' plain IDs. It must never be looked up as a user.
const AGENT_ACTOR_PREFIX = 'agent:';

export function agentActorValue(agentId: string): string {
	return `${AGENT_ACTOR_PREFIX}${agentId}`;
}

// parseAgentActor returns the agent ID of an "agent:<id>" actor, or undefined for anything else.
export function parseAgentActor(value: string | undefined | null): string | undefined {
	if (!value?.startsWith(AGENT_ACTOR_PREFIX)) return undefined;
	return value.slice(AGENT_ACTOR_PREFIX.length) || undefined;
}

// agentKeyMask is how an agent's key is shown: its secret is never available after creation.
export function agentKeyMask(key: Pick<AgentAPIKey, 'agentId' | 'id'>): string {
	return `oak1-${key.agentId}-${key.id}-*****`;
}

export function agentStatusLabel(status: AgentIdentityStatus): string {
	switch (status) {
		case 'active':
			return m.identity_access_users_status_active();
		case 'disabled':
			return m.core_status_disabled();
		case 'deleted':
			return m.identity_access_users_status_deleted();
	}
}

export type AgentLookup = Pick<AgentIdentity, 'displayName' | 'status'>;

// agentDisplayName names an agent wherever one is shown. The name comes from the agent's own record
// when it is known, so that it follows a rename, and otherwise from the row, which the server fills
// in from the record even after the agent is deleted. A deleted agent is marked, and keeps its name
// when that is known.
export function agentDisplayName(
	lookup: Map<string, AgentLookup> | undefined,
	agentId: string,
	rowName?: string
): string {
	const known = lookup?.get(agentId);
	const name = known?.displayName || rowName;
	if (known?.status === 'deleted') {
		return name ? m.identity_access_name_deleted({ name }) : m.core_deleted_agent({ id: agentId });
	}
	return name || m.audit_usage_agent_fallback_name({ id: agentId });
}
