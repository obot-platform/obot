import type { OrgUser } from '$lib/services';
import type { Profile } from '$lib/services';
import { agentDisplayName, type AgentIdentity } from '$lib/services/agent-identities';
import {
	agentSubjectName,
	convertSubjectsToTableData,
	hasAccessWithinSubjects,
	resolveSubjectFromGroup,
	resolveSubjectPickerById,
	subjectPickerOptions,
	subjectsFromPicker
} from './subjectResolver';
import { describe, expect, it } from 'vitest';

describe('resolveSubjectFromGroup', () => {
	it('stores all users as a selector', () => {
		expect(resolveSubjectFromGroup({ id: '*', name: 'All Obot Users' })).toEqual({
			type: 'selector',
			id: '*'
		});
	});

	it('stores the Obot Admin picker row as an obot group', () => {
		expect(resolveSubjectFromGroup({ id: 'obot-admin', name: 'Obot Admin' })).toEqual({
			type: 'obotGroup',
			id: 'admin'
		});
	});

	it('stores a directory group whose id is admin as a group', () => {
		expect(resolveSubjectFromGroup({ id: 'admin', name: 'Admin' })).toEqual({
			type: 'group',
			id: 'admin'
		});
	});

	it('uses a synthetic picker id for Obot Admin', () => {
		expect(resolveSubjectPickerById({ type: 'obotGroup', id: 'admin' })).toBe('obot-admin');
		expect(resolveSubjectPickerById({ type: 'group', id: 'admin' })).toBe('admin');
	});

	it('stores a directory group as a group', () => {
		expect(resolveSubjectFromGroup({ id: 'entra/engineering', name: 'Engineering' })).toEqual({
			type: 'group',
			id: 'entra/engineering'
		});
	});
});

describe('convertSubjectsToTableData', () => {
	it('labels an obot group without a directory lookup', () => {
		expect(convertSubjectsToTableData([{ type: 'obotGroup', id: 'admin' }], [], [])).toEqual([
			{ id: 'obot-admin', displayName: 'Obot Admin', type: 'Group' }
		]);
	});

	it('uses the directory name for a group subject', () => {
		expect(
			convertSubjectsToTableData(
				[{ type: 'group', id: 'entra/engineering' }],
				[],
				[{ id: 'entra/engineering', name: 'Engineering' }]
			)
		).toEqual([{ id: 'entra/engineering', displayName: 'Engineering', type: 'Group' }]);
	});

	it('marks a disabled user', () => {
		expect(
			convertSubjectsToTableData(
				[
					{ type: 'user', id: 'u1' },
					{ type: 'user', id: 'u2' }
				],
				[
					{ id: 'u1', displayName: 'Alice', status: 'active' } as OrgUser,
					{ id: 'u2', displayName: 'Dan', status: 'disabled' } as OrgUser
				],
				[]
			)
		).toEqual([
			{ id: 'u1', displayName: 'Alice', type: 'User', disabled: false },
			{ id: 'u2', displayName: 'Dan', type: 'User', disabled: true }
		]);
	});
});

describe('agent subjects', () => {
	it('builds subjects from the picker, skipping those already added', () => {
		expect(
			subjectsFromPicker(
				['agent:5'],
				[{ id: '5', displayName: 'Alice' } as OrgUser],
				[{ id: '*', name: 'All Obot Users' }],
				[
					{ id: '5', displayName: 'Bot' },
					{ id: '6', displayName: 'Other bot' }
				] as never
			)
		).toEqual([
			{ type: 'user', id: '5' },
			{ type: 'selector', id: '*' },
			{ type: 'agent', id: '6' }
		]);
	});

	it('names agents, marks them and marks a deleted one', () => {
		const agents = new Map([
			['5', { displayName: 'Build bot', status: 'active' as const }],
			['6', { displayName: 'Old bot', status: 'deleted' as const }]
		]);
		expect(
			convertSubjectsToTableData(
				[
					{ type: 'agent', id: '5' },
					{ type: 'agent', id: '6' },
					{ type: 'agent', id: '7' }
				],
				[],
				[],
				{ agents }
			)
		).toEqual([
			{ id: 'agent:5', displayName: 'Build bot', type: 'Agent', agent: true },
			{ id: 'agent:6', displayName: 'Old bot (Deleted)', type: 'Agent', agent: true },
			{ id: 'agent:7', displayName: 'Agent 7', type: 'Agent', agent: true }
		]);
	});

	it('labels the selector wildcard All Obot Users, which never includes an agent', () => {
		const everyone = [{ type: 'selector' as const, id: '*' }];
		expect(convertSubjectsToTableData(everyone, [], [])[0]).toEqual({
			id: '*',
			displayName: 'All Obot Users',
			type: 'Selector'
		});
	});

	// Policies, profiles, owner lists, audit and usage all name agents with the same helper, so a
	// deleted agent reads the same everywhere: by its name when that is known, by its ID otherwise.
	it('labels a deleted agent the same way wherever it is named', () => {
		const agents = new Map([
			['6', { displayName: 'Old bot', status: 'deleted' as const }],
			['8', { displayName: '', status: 'deleted' as const }]
		]);
		expect(agentSubjectName('6', agents)).toBe('Old bot (Deleted)');
		expect(agentDisplayName(agents, '6', 'Stale row name')).toBe('Old bot (Deleted)');
		expect(agentSubjectName('8', agents)).toBe('Deleted agent (8)');
		// The row's name stands in for a deleted agent's name that the lookup lacks.
		expect(agentDisplayName(agents, '8', 'Row bot')).toBe('Row bot (Deleted)');
		expect(agentDisplayName(agents, '8')).toBe('Deleted agent (8)');
		// Without the agent list, nothing says the agent is deleted.
		expect(agentDisplayName(undefined, '6', 'Old bot')).toBe('Old bot');
		expect(agentDisplayName(undefined, '6')).toBe('Agent 6');
	});

	it('builds the same picker rows for the dialog and the inline picker', () => {
		const users = [
			{ id: '5', displayName: 'Alice', email: 'alice@example.com', status: 'active' } as OrgUser,
			{ id: '6', displayName: 'Dan', status: 'disabled' } as OrgUser
		];
		const agents = [
			{ id: '5', displayName: 'Build bot', status: 'active' },
			{ id: '7', displayName: 'Off bot', status: 'disabled' }
		] as AgentIdentity[];
		const options = subjectPickerOptions({
			users,
			groups: [
				{ id: 'g1', name: 'Engineering' },
				{ id: 'g2', name: 'Sales' }
			],
			agents,
			query: '',
			offerAgents: true,
			offerObotAdmin: false,
			existingPickerIds: ['g2']
		});
		// The one wildcard is All Obot Users: there is no row for all agents or all users.
		expect(options.map((option) => [option.key, option.label, option.subject])).toEqual([
			['*', 'All Obot Users', { type: 'selector', id: '*' }],
			['g1', 'Engineering', { type: 'group', id: 'g1' }],
			['agent:5', 'Build bot', { type: 'agent', id: '5' }],
			['5', 'Alice', { type: 'user', id: '5' }]
		]);

		const matching = subjectPickerOptions({
			users,
			groups: [],
			agents,
			query: 'ALICE@',
			offerAgents: false,
			offerObotAdmin: true,
			existingPickerIds: []
		});
		expect(matching.map((option) => option.key)).toEqual(['5']);
	});

	it('never treats an agent as the current user', () => {
		const profile = { id: '5', groups: [] } as unknown as Profile;
		expect(hasAccessWithinSubjects([{ type: 'agent', id: '5' }], profile)).toBe(false);
		expect(hasAccessWithinSubjects([{ type: 'user', id: '5' }], profile)).toBe(true);
		// The user ID "*" is an ordinary user subject that names no one.
		expect(hasAccessWithinSubjects([{ type: 'user', id: '*' }], profile)).toBe(false);
		expect(hasAccessWithinSubjects([{ type: 'selector', id: '*' }], profile)).toBe(true);
	});
});
