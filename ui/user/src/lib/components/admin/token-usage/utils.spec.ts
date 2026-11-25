import type { OrgUser, TokenUsage } from '$lib/services';
import { getUserLabels, tokenUsageGroupKey } from './utils';
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

describe('usage of an agent that acted for a user', () => {
	const row = (overrides: Partial<TokenUsage>) =>
		({ date: '2026-01-01', ...overrides }) as TokenUsage;
	const agentRow = row({
		principal: { kind: 'agent', id: '7' },
		onBehalfOf: { kind: 'user', id: 'u1' }
	});

	it('groups it apart from the agent own usage, and from other users', () => {
		const own = row({ principal: { kind: 'agent', id: '7' } });
		const other = row({
			principal: { kind: 'agent', id: '7' },
			onBehalfOf: { kind: 'user', id: 'u2' }
		});

		expect(new Set([agentRow, own, other].map(tokenUsageGroupKey)).size).toBe(3);
		expect(tokenUsageGroupKey(own)).toBe('agent:7');
		expect(tokenUsageGroupKey(row({ userID: 'u1' }))).toBe('u1');
	});

	it('is named "<agent> for <user>"', () => {
		const users = new Map<string, OrgUser>([
			['u1', { id: 'u1', displayName: 'Alice', status: 'active' } as OrgUser]
		]);
		const agents = new Map([['7', { displayName: 'Build Bot', status: 'active' as const }]]);
		const key = tokenUsageGroupKey(agentRow)!;

		expect(getUserLabels(users, [key, 'agent:7'], agents)).toEqual(
			new Map([
				[key, 'Build Bot for Alice'],
				['agent:7', 'Build Bot (Agent)']
			])
		);
	});
});
