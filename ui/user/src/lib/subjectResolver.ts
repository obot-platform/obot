import { m } from '$lib/i18n';
import {
	Group,
	UserService,
	type AccessControlRuleSubject,
	type OrgGroup,
	type OrgUser,
	type Profile
} from '$lib/services';
import {
	agentActorValue,
	agentDisplayName,
	type AgentIdentity,
	type AgentLookup
} from '$lib/services/agent-identities';
import { getUserDisplayName, getUserRoleLabel, isDisabledUser } from '$lib/utils';

const EVERYONE_SUBJECT_ID = '*';
export const OBOT_ADMIN_PICKER_ID = 'obot-admin';

// agentPickerId keeps an agent apart from a user: both have decimal IDs, so "5" can be either. It is
// the same "agent:<id>" the audit and usage APIs use.
function agentPickerId(agentId: string): string {
	return agentActorValue(agentId);
}

export function resolveSubjectFromGroup(group: OrgGroup): AccessControlRuleSubject {
	if (group.id === EVERYONE_SUBJECT_ID) {
		return { type: 'selector', id: group.id };
	}
	if (group.id === OBOT_ADMIN_PICKER_ID) {
		return { type: 'obotGroup', id: Group.ADMIN };
	}
	return { type: 'group', id: group.id };
}

export function resolveSubjectPickerById(
	subject: Pick<AccessControlRuleSubject, 'type' | 'id'>
): string {
	if (subject.type === 'obotGroup' && subject.id === Group.ADMIN) {
		return OBOT_ADMIN_PICKER_ID;
	}
	if (subject.type === 'agent') {
		return agentPickerId(subject.id);
	}
	return subject.id;
}

// subjectsFromPicker turns what the picker returned into subjects, leaving out those already added.
export function subjectsFromPicker(
	existingPickerIds: Iterable<string>,
	users: OrgUser[],
	groups: OrgGroup[],
	agents: AgentIdentity[] = []
): AccessControlRuleSubject[] {
	const existing = new Set(existingPickerIds);
	return [
		...users
			.filter((user) => !existing.has(user.id))
			.map((user): AccessControlRuleSubject => ({ type: 'user', id: user.id })),
		...groups.filter((group) => !existing.has(group.id)).map(resolveSubjectFromGroup),
		...agents
			.filter((agent) => !existing.has(agentPickerId(agent.id)))
			.map((agent): AccessControlRuleSubject => ({ type: 'agent', id: agent.id }))
	];
}

// hasNamedAgent is true when a subject names an agent, which then needs a name to be shown.
export function hasNamedAgent(subjects: AccessControlRuleSubject[] | undefined): boolean {
	return subjects?.some((s) => s.type === 'agent') ?? false;
}

// agentLookupWith adds agents, such as those just picked, to a lookup of agent names.
export function agentLookupWith(
	lookup: Map<string, AgentLookup> | undefined,
	agents: AgentIdentity[]
): Map<string, AgentLookup> {
	return new Map([...(lookup ?? []), ...agents.map((a): [string, AgentLookup] => [a.id, a])]);
}

// isEveryonePickerId is true for the picker ID of "All Obot Users" ({selector, "*"}).
export function isEveryonePickerId(pickerId: string): boolean {
	return pickerId === EVERYONE_SUBJECT_ID;
}

// isAgentPickerId is true for the picker ID of an agent (agentPickerId).
export function isAgentPickerId(pickerId: string): boolean {
	return pickerId.startsWith(agentPickerId(''));
}

// agentSubjectName names an agent subject. A deleted agent keeps its subjects in policies but matches
// nobody, so it is marked; an agent that is not known (the lookup is admin-only) shows its ID.
export function agentSubjectName(agentId: string, lookup?: Map<string, AgentLookup>): string {
	return agentDisplayName(lookup, agentId);
}

// One row of a subject picker. key is the picker ID (see resolveSubjectPickerById), unique across
// kinds: a user and an agent can share a decimal ID.
export interface SubjectPickerOption {
	key: string;
	label: string;
	// The kind of row: a user's role, Group or Agent.
	kind: string;
	subject: AccessControlRuleSubject;
	user?: OrgUser;
	group?: OrgGroup;
	agent?: AgentIdentity;
}

// subjectPickerOptions builds the rows a subject picker offers for a query: All Obot Users, the Obot
// Admin group, the groups (searched by the caller, as the directory pages them), and the active agents
// and enabled users that match. Those already added are left out.
export function subjectPickerOptions(opts: {
	users: OrgUser[];
	groups: OrgGroup[];
	agents?: AgentIdentity[];
	query: string;
	// offerAgents offers the active agents, one by one, since All Obot Users never includes an agent.
	offerAgents: boolean;
	offerObotAdmin: boolean;
	existingPickerIds: Iterable<string>;
}): SubjectPickerOption[] {
	const query = opts.query.trim().toLowerCase();
	const matches = (...values: (string | undefined)[]) =>
		!query || values.some((value) => (value ?? '').toLowerCase().includes(query));
	const specialGroups: OrgGroup[] = [
		{ id: EVERYONE_SUBJECT_ID, name: m.core_all_obot_users() },
		...(opts.offerObotAdmin
			? [{ id: OBOT_ADMIN_PICKER_ID, name: m.identity_access_users_obot_admin() }]
			: [])
	].filter((group) => matches(group.name));
	// Disabled users cannot sign in and disabled agents cannot connect, so neither is offered.
	const users = opts.users.filter(
		(user) => !isDisabledUser(user) && matches(user.displayName, user.email, user.username)
	);
	const agents = opts.offerAgents
		? (opts.agents ?? []).filter((agent) => agent.status === 'active' && matches(agent.displayName))
		: [];
	const existing = new Set(opts.existingPickerIds);
	return [
		...[...specialGroups, ...opts.groups].map(
			(group): SubjectPickerOption => ({
				key: group.id,
				label: group.name,
				kind: m.core_col_group(),
				subject: resolveSubjectFromGroup(group),
				group
			})
		),
		...agents.map(
			(agent): SubjectPickerOption => ({
				key: agentPickerId(agent.id),
				label: agent.displayName,
				kind: m.core_col_agent(),
				subject: { type: 'agent', id: agent.id },
				agent
			})
		),
		...users.map(
			(user): SubjectPickerOption => ({
				key: user.id,
				label: user.displayName ?? user.email ?? user.username ?? user.id,
				kind: user.effectiveRole ? getUserRoleLabel(user.effectiveRole) : m.core_col_user(),
				subject: { type: 'user', id: user.id },
				user
			})
		)
	].filter((option) => !existing.has(option.key));
}

export function obotGroupDisplayName(id: string): string {
	return `Obot ${id.charAt(0).toUpperCase()}${id.slice(1)}`;
}

export interface SubjectTableRow {
	id: string;
	displayName: string;
	type: string;
	/** Set for a user who is disabled. */
	disabled?: boolean;
	/** Set for an agent, which is shown with a badge. */
	agent?: boolean;
}

export interface ResolvedSubjects {
	users: OrgUser[];
	groups: OrgGroup[];
}

/**
 * Loads the users and groups needed to render a policy's or rule's subjects.
 *
 * Groups are resolved by ID rather than listed. A directory can hold tens of thousands of groups,
 * so fetching the whole collection to build a lookup map would both be wasteful and silently drop
 * any subject whose group fell outside the first page.
 *
 * `existing` lets a caller keep whatever it already has: users are fetched only once, and groups
 * only for IDs that are not already known.
 *
 * Pass `signal` from a caller that can ask again before the first answer arrives, so that a
 * superseded request is dropped rather than left to land last and overwrite current state.
 */
export async function resolveSubjects(
	subjects: AccessControlRuleSubject[] | undefined,
	existing?: Partial<ResolvedSubjects>,
	opts?: { signal?: AbortSignal }
): Promise<ResolvedSubjects> {
	const resolved: ResolvedSubjects = {
		users: existing?.users ?? [],
		groups: existing?.groups ?? []
	};

	if (!subjects || subjects.length === 0) {
		return resolved;
	}

	const known = new Set(resolved.groups.map((group) => group.id));
	const missingGroupIds = [
		...new Set(
			subjects
				.filter((subject) => subject.type === 'group')
				.map((subject) => subject.id)
				// The "all users" pseudo-group is client-side only and has no directory entry.
				.filter((id) => id !== EVERYONE_SUBJECT_ID && !known.has(id))
		)
	];

	const [users, groups] = await Promise.all([
		existing?.users ? Promise.resolve(undefined) : UserService.listUsers(opts),
		missingGroupIds.length > 0
			? UserService.resolveGroups(missingGroupIds, opts)
			: Promise.resolve([])
	]);

	if (users) {
		resolved.users = users;
	}
	if (groups.length > 0) {
		resolved.groups = [...resolved.groups, ...groups];
	}

	return resolved;
}

export interface SubjectTableOptions {
	/** Agents by ID, to name agent subjects. */
	agents?: Map<string, AgentLookup>;
}

export function convertSubjectsToTableData(
	subjects: AccessControlRuleSubject[],
	users: OrgUser[],
	groups: OrgGroup[],
	options: SubjectTableOptions = {}
): SubjectTableRow[] {
	const userMap = new Map(users?.map((user) => [user.id, user]));
	const groupMap = new Map(groups?.map((group) => [group.id, group]));

	return (
		subjects
			.map((subject): SubjectTableRow | undefined => {
				if (subject.type === 'agent') {
					return {
						id: resolveSubjectPickerById(subject),
						displayName: agentSubjectName(subject.id, options.agents),
						type: m.core_col_agent(),
						agent: true
					};
				}

				if (subject.type === 'user') {
					return {
						id: subject.id,
						displayName: getUserDisplayName(userMap, subject.id),
						type: m.core_col_user(),
						disabled: isDisabledUser(userMap.get(subject.id))
					};
				}

				if (subject.type === 'group') {
					const group = groupMap.get(subject.id);

					return {
						id: subject.id,
						displayName: group?.name ?? subject.id,
						type: m.core_col_group()
					};
				}

				if (subject.type === 'obotGroup') {
					return {
						id: resolveSubjectPickerById(subject),
						displayName: obotGroupDisplayName(subject.id),
						type: m.core_col_group()
					};
				}

				return {
					id: subject.id,
					displayName: subject.id === EVERYONE_SUBJECT_ID ? m.core_all_obot_users() : subject.id,
					type: m.identity_access_users_selector()
				};
			})
			.filter((subject): subject is SubjectTableRow => subject !== undefined) ?? []
	);
}

export function hasAccessWithinSubjects(
	subjects: AccessControlRuleSubject[],
	profile: Profile
): boolean {
	for (const subject of subjects) {
		if (subject.type === 'selector' && subject.id === EVERYONE_SUBJECT_ID) {
			return true;
		}

		if (subject.type === 'user' && subject.id === profile.id) {
			return true;
		}

		if (
			subject.type === 'group' &&
			profile.authProviderGroups &&
			profile.authProviderGroups.some((group) => group === subject.id)
		) {
			return true;
		}

		if (
			subject.type === 'obotGroup' &&
			profile.groups &&
			profile.groups.some((group) => group === subject.id)
		) {
			return true;
		}
	}
	return false;
}
