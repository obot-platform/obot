export type PublishedArtifactSubject = {
	type: 'user' | 'group' | 'selector';
	id: string;
};

export function hasAllUsersSubject(subjects?: PublishedArtifactSubject[]): boolean {
	return !!subjects?.some((subject) => subject.type === 'selector' && subject.id === '*');
}

export function sharingLabel(subjects?: PublishedArtifactSubject[]): string {
	if (!subjects || subjects.length === 0) {
		return 'Owner Only';
	}
	if (hasAllUsersSubject(subjects)) {
		return 'All Obot Users';
	}

	const users = subjects.filter((subject) => subject.type === 'user').length;
	const groups = subjects.filter((subject) => subject.type === 'group').length;
	const parts = [];
	if (users > 0) {
		parts.push(`${users} ${users === 1 ? 'user' : 'users'}`);
	}
	if (groups > 0) {
		parts.push(`${groups} ${groups === 1 ? 'group' : 'groups'}`);
	}
	return parts.join(', ');
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
