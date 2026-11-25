import { hasAgentOwner, instanceOwner } from './instanceOwners';
import type { OrgUser } from './services';
import { describe, expect, it } from 'vitest';

const ada = { id: '1', displayName: 'Ada Admin', email: 'ada@example.com' } as OrgUser;

describe('instance owners', () => {
	it('splits owners by principal kind, so an agent is never looked up as a user', () => {
		expect(hasAgentOwner(['1', 'agent:7', undefined])).toBe(true);
		expect(hasAgentOwner(['1', '7', undefined])).toBe(false);
	});

	it('names a user, an agent and a deleted agent', () => {
		const agents = new Map([
			['7', { displayName: 'Build Bot', status: 'active' as const }],
			['9', { displayName: 'Old Bot', status: 'deleted' as const }]
		]);
		const users = new Map([[ada.id, ada]]);

		expect(instanceOwner('1', users, agents)).toMatchObject({ name: 'Ada Admin', agent: false });
		expect(instanceOwner('agent:7', users, agents)).toEqual({
			id: 'agent:7',
			name: 'Build Bot',
			agent: true,
			disabled: false
		});
		expect(instanceOwner('agent:9', users, agents).name).toBe('Old Bot (Deleted)');
		// Without the agent list (anyone but admins and auditors), the agent is shown by its ID.
		expect(instanceOwner('agent:7', users).name).toBe('Agent 7');
	});
});
