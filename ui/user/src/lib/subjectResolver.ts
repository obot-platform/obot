import { m } from '$lib/i18n';
import {
	Group,
	UserService,
	type AccessControlRuleSubject,
	type OrgGroup,
	type OrgUser,
	type Profile
} from '$lib/services';
import { getUserDisplayName } from '$lib/utils';

const EVERYONE_SUBJECT_ID = '*';
export const OBOT_ADMIN_PICKER_ID = 'obot-admin';

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
	return subject.id;
}

export function obotGroupDisplayName(id: string): string {
	return `Obot ${id.charAt(0).toUpperCase()}${id.slice(1)}`;
}

export interface SubjectTableRow {
	id: string;
	displayName: string;
	type: string;
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
						type: m.core_col_user()
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
