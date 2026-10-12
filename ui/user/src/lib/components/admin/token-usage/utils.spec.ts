import type { OrgUser } from '$lib/services';
import { getUserLabels } from './utils';
import { describe, expect, it } from 'vitest';

describe('getUserLabels', () => {
	it('marks disabled users', () => {
		const users = new Map<string, OrgUser>([
			['u1', { id: 'u1', displayName: 'Alice', status: 'active' } as OrgUser],
			['u2', { id: 'u2', displayName: 'Dan', status: 'disabled' } as OrgUser]
		]);

		expect(getUserLabels(users, ['u1', 'u2'])).toEqual(
			new Map([
				['u1', 'Alice'],
				['u2', 'Dan (disabled)']
			])
		);
	});
});
