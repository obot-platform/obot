import { m } from '$lib/i18n';
import { formatTimeAgo } from '$lib/time';
import type { AgentDelegation, AgentIdentity, AgentIdentityStatus, AgentAPIKey } from './types';

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

// delegationTimes says when a delegation was first and last approved, and last used, as the lists and
// the user's page of a delegation show it.
export function delegationTimes(d: AgentDelegation) {
	return {
		firstApprovedAt: formatTimeAgo(d.firstApprovedAt).relativeTime,
		lastApprovedAt: formatTimeAgo(d.lastApprovedAt).relativeTime,
		lastUsedAt: d.lastUsedAt ? formatTimeAgo(d.lastUsedAt).relativeTime : m.core_never_used()
	};
}

// The part of an agent profile form that a validation error of the server (400 "invalid profiles: ...") is about.
export type ProfileErrorField = 'name' | 'subjects';

// profileError splits the server's error about a profile into the reason, without what the server puts before it
// ("invalid ...:" and the name of the entry, which the form already shows), and the field of the form the reason
// names, if any, so the error is shown next to it. The entry's name is taken out first, since it is free text that
// can hold any word.
export function profileError(reason: string): { field?: ProfileErrorField; text: string } {
	const text = reason
		.replace(/^invalid (profiles|allowedReturnHosts):\s*/, '')
		.replace(/^agent profile "(?:[^"\\]|\\.)*":\s*/, '');
	if (/^(duplicate agent profile name|name\b)/.test(text)) return { field: 'name', text };
	if (/subject/i.test(text)) return { field: 'subjects', text };
	return { text };
}

// agentApprovalSignInPath is the normal sign-in for a signed-out user who opens an agent's approval
// page at url: it returns them to the page, and passes on the agent's login hint, which pre-fills the
// sign-in. The hint is a convenience, never a check of who approves.
export function agentApprovalSignInPath(url: URL): string {
	const params = new URLSearchParams({ rd: url.pathname + url.search });
	const hint = url.searchParams.get('login_hint');
	if (hint) params.set('login_hint', hint);
	return `/?${params}`;
}

// isWebURL reports whether a URL the server returned is a web page the browser may be sent to.
export function isWebURL(url: string): boolean {
	try {
		const parsed = new URL(url, window.location.origin);
		return parsed.protocol === 'https:' || parsed.protocol === 'http:';
	} catch {
		return false;
	}
}
