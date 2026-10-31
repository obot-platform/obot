import { Role } from './services';
import { getUserRoleLabel } from './utils';
import { describe, expect, it } from 'vitest';

describe('getUserRoleLabel', () => {
	it.each([
		{ role: Role.ADMIN_READONLY, label: 'Admin Read-Only' },
		{ role: Role.ADMIN_READONLY | Role.AUDITOR, label: 'Admin Read-Only, Auditor' },
		{ role: Role.ADMIN_READONLY | Role.POWERUSER_PLUS, label: 'Admin Read-Only' },
		{ role: Role.ADMIN_READONLY | Role.POWERUSER, label: 'Admin Read-Only' },
		{
			role: Role.ADMIN_READONLY | Role.POWERUSER_PLUS | Role.AUDITOR,
			label: 'Admin Read-Only, Auditor'
		},
		{ role: Role.POWERUSER_PLUS, label: 'Power User Plus' }
	])('labels role $role as "$label"', ({ role, label }) => {
		expect(getUserRoleLabel(role)).toBe(label);
	});
});
