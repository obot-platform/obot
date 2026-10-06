import { m } from '$lib/i18n';
import type { AccessControlRuleSubject } from '$lib/services';

export type PublishedArtifactSubject = AccessControlRuleSubject;

export function hasAllUsersSubject(subjects?: PublishedArtifactSubject[]): boolean {
	return !!subjects?.some((subject) => subject.type === 'selector' && subject.id === '*');
}

export function sharingLabel(subjects?: PublishedArtifactSubject[]): string {
	if (!subjects || subjects.length === 0) {
		return m.chat_sharing_owner_only();
	}
	if (hasAllUsersSubject(subjects)) {
		return m.core_all_obot_users();
	}

	const users = subjects.filter((subject) => subject.type === 'user').length;
	const groups = subjects.filter((subject) => subject.type === 'group').length;
	const parts = [];
	if (users > 0) {
		parts.push(
			users === 1
				? m.chat_sharing_users_one({ count: users })
				: m.chat_sharing_users_other({ count: users })
		);
	}
	if (groups > 0) {
		parts.push(
			groups === 1
				? m.chat_sharing_groups_one({ count: groups })
				: m.chat_sharing_groups_other({ count: groups })
		);
	}
	return parts.join(m.chat_list_separator());
}

export function latestVersionSubjects<
	T extends { version: number; subjects?: PublishedArtifactSubject[] }
>(versions?: T[], latestVersion?: number): PublishedArtifactSubject[] {
	if (!versions || versions.length === 0) return [];
	const match =
		latestVersion != null
			? versions.find((version) => version.version === latestVersion)
			: versions.reduce((latest, current) => (current.version > latest.version ? current : latest));
	return match?.subjects ?? [];
}
