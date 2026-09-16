import {
	UserService,
	type AccessControlRuleSubject,
	type OrgGroup,
	type OrgUser
} from '$lib/services';
import { getUserDisplayName } from '$lib/utils';

export interface SubjectTableRow {
	id: string;
	displayName: string;
	type: 'User' | 'Group' | 'Selector';
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
				.filter((id) => id !== '*' && !known.has(id))
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

export function convertSubjectsToTableData(
	subjects: AccessControlRuleSubject[],
	users: OrgUser[],
	groups: OrgGroup[]
): SubjectTableRow[] {
	const userMap = new Map(users?.map((user) => [user.id, user]));
	const groupMap = new Map(groups?.map((group) => [group.id, group]));

	return (
		subjects
			.map((subject): SubjectTableRow | undefined => {
				if (subject.type === 'user') {
					return {
						id: subject.id,
						displayName: getUserDisplayName(userMap, subject.id),
						type: 'User'
					};
				}

				if (subject.type === 'group') {
					const group = groupMap.get(subject.id);

					return {
						id: subject.id,
						displayName: group?.name ?? subject.id,
						type: 'Group'
					};
				}

				return {
					id: subject.id,
					displayName: subject.id === '*' ? 'All Obot Users' : subject.id,
					type: 'Selector'
				};
			})
			.filter((subject): subject is SubjectTableRow => subject !== undefined) ?? []
	);
}
