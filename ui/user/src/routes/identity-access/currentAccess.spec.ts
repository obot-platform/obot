import { AdminService } from '$lib/services';
import {
	collectAccessResources,
	grantsEverything,
	groupMcpAccessPolicies,
	hasAnyCurrentAccess,
	loadCurrentAccess,
	mcpAccessPolicyHref,
	subjectsApplyTo,
	type AccessPolicyResource,
	type CurrentAccessTarget,
	type MatchedAccessPolicy
} from './currentAccess';
import { afterEach, describe, expect, it, vi } from 'vitest';

const userTarget: CurrentAccessTarget = {
	kind: 'user',
	id: 'user-1',
	name: 'Ada',
	groupIds: ['engineering']
};

const groupTarget: CurrentAccessTarget = {
	kind: 'group',
	id: 'engineering',
	name: 'Engineering'
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe('subjectsApplyTo', () => {
	it('matches everyone, the user, and groups the user belongs to', () => {
		expect(subjectsApplyTo([{ type: 'selector', id: '*' }], userTarget)).toEqual(['everyone']);
		expect(subjectsApplyTo([{ type: 'user', id: 'user-1' }], userTarget)).toEqual(['direct-user']);
		expect(subjectsApplyTo([{ type: 'group', id: 'engineering' }], userTarget)).toEqual([
			'via-group'
		]);
		expect(subjectsApplyTo([{ type: 'user', id: 'other' }], userTarget)).toEqual([]);
		expect(subjectsApplyTo([{ type: 'group', id: 'sales' }], userTarget)).toEqual([]);
	});

	it('matches everyone and the group itself for a group target', () => {
		expect(subjectsApplyTo([{ type: 'selector', id: '*' }], groupTarget)).toEqual(['everyone']);
		expect(subjectsApplyTo([{ type: 'group', id: 'engineering' }], groupTarget)).toEqual([
			'direct-group'
		]);
		expect(subjectsApplyTo([{ type: 'user', id: 'user-1' }], groupTarget)).toEqual([]);
		expect(subjectsApplyTo([{ type: 'group', id: 'sales' }], groupTarget)).toEqual([]);
	});
});

describe('mcpAccessPolicyHref', () => {
	it('uses the workspace path for power-user workspace rules', () => {
		expect(
			mcpAccessPolicyHref({
				id: 'rule-1',
				displayName: 'Workspace',
				created: '2026-01-01T00:00:00Z',
				powerUserWorkspaceID: 'ws-1'
			})
		).toBe('/mcp-servers/access-policies/w/ws-1/r/rule-1');
	});
});

describe('collectAccessResources', () => {
	function policy(
		id: string,
		displayName: string,
		resources: AccessPolicyResource[]
	): MatchedAccessPolicy {
		return { id, displayName, href: `/policies/${id}`, reasons: ['everyone'], resources };
	}

	const describeResource = (resource: AccessPolicyResource) => ({ name: `Name ${resource.id}` });

	it('deduplicates resources and records every policy that grants them', () => {
		const first = policy('policy-1', 'First', [
			{ type: 'mcpServer', id: 'server-2' },
			{ type: 'mcpServer', id: 'server-1' }
		]);
		const second = policy('policy-2', 'Second', [{ type: 'mcpServer', id: 'server-1' }]);

		const resources = collectAccessResources([first, second], describeResource);

		expect(resources.map((resource) => resource.name)).toEqual(['Name server-1', 'Name server-2']);
		expect(resources[0]?.policies.map((granting) => granting.id)).toEqual(['policy-1', 'policy-2']);
		expect(resources[1]?.policies.map((granting) => granting.id)).toEqual(['policy-1']);
	});

	it('treats the same id under different types as separate resources', () => {
		const resources = collectAccessResources(
			[
				policy('policy-1', 'First', [
					{ type: 'skill', id: 'shared' },
					{ type: 'skillRepository', id: 'shared' }
				])
			],
			describeResource
		);

		expect(resources.map((resource) => resource.type)).toEqual(['skill', 'skillRepository']);
	});

	it('omits the everything resource, which is reported by grantsEverything instead', () => {
		const policies = [
			policy('policy-1', 'First', [
				{ type: 'selector', id: '*' },
				{ type: 'mcpServer', id: 'server-1' }
			])
		];

		expect(collectAccessResources(policies, describeResource).map((r) => r.id)).toEqual([
			'server-1'
		]);
		expect(grantsEverything(policies)).toBe(true);
		expect(grantsEverything([policy('policy-2', 'Second', [{ type: 'model', id: 'gpt-*' }])])).toBe(
			false
		);
	});
});

describe('groupMcpAccessPolicies', () => {
	const policy = (id: string, powerUserID?: string): MatchedAccessPolicy => ({
		id,
		displayName: id,
		href: `/policies/${id}`,
		reasons: ['everyone'],
		resources: [],
		powerUserID
	});

	it('keeps global policies and each power user registry in separate groups', () => {
		const groups = groupMcpAccessPolicies([
			policy('owner-b-policy', 'owner-b'),
			policy('global-policy'),
			policy('owner-a-first', 'owner-a'),
			policy('owner-a-second', 'owner-a')
		]);

		expect(
			groups.map((group) => ({
				key: group.key,
				policies: group.policies.map((item) => item.id)
			}))
		).toEqual([
			{ key: 'global', policies: ['global-policy'] },
			{ key: 'owner-a', policies: ['owner-a-first', 'owner-a-second'] },
			{ key: 'owner-b', policies: ['owner-b-policy'] }
		]);
	});
});

describe('loadCurrentAccess', () => {
	it('filters each policy type and includes everyone', async () => {
		vi.spyOn(AdminService, 'listAccessControlRules').mockResolvedValue([
			{
				id: 'mcp-user',
				displayName: 'User MCP',
				created: '2026-01-01T00:00:00Z',
				subjects: [{ type: 'user', id: 'user-1' }]
			},
			{
				id: 'mcp-other',
				displayName: 'Other MCP',
				created: '2026-01-01T00:00:00Z',
				subjects: [{ type: 'user', id: 'other' }]
			}
		]);
		vi.spyOn(AdminService, 'listAllUserWorkspaceAccessControlRules').mockResolvedValue([
			{
				id: 'mcp-ws',
				displayName: 'Workspace MCP',
				created: '2026-01-01T00:00:00Z',
				powerUserID: 'owner-1',
				powerUserWorkspaceID: 'ws-1',
				subjects: [{ type: 'group', id: 'engineering' }]
			}
		]);
		vi.spyOn(AdminService, 'listModelAccessPolicies').mockResolvedValue([
			{
				id: 'model-everyone',
				displayName: 'Everyone Models',
				created: '2026-01-01T00:00:00Z',
				subjects: [{ type: 'selector', id: '*' }]
			}
		]);
		vi.spyOn(AdminService, 'listSkillAccessPolicies').mockResolvedValue([
			{
				id: 'skill-sales',
				displayName: 'Sales Skills',
				created: '2026-01-01T00:00:00Z',
				subjects: [{ type: 'group', id: 'sales' }],
				resources: []
			}
		]);
		vi.spyOn(AdminService, 'listHostedAgentAccessPolicies').mockResolvedValue([
			{
				id: 'hosted-user',
				displayName: 'User Hosted',
				created: '2026-01-01T00:00:00Z',
				subjects: [{ type: 'user', id: 'user-1' }],
				resources: []
			}
		]);

		const sections = await loadCurrentAccess(userTarget);

		expect(sections.mcp.map((policy) => policy.id)).toEqual(['mcp-user', 'mcp-ws']);
		expect(sections.mcp[1]?.href).toBe('/mcp-servers/access-policies/w/ws-1/r/mcp-ws');
		expect(sections.mcp[1]?.powerUserID).toBe('owner-1');
		expect(sections.models.map((policy) => policy.displayName)).toEqual(['Everyone Models']);
		expect(sections.skills).toEqual([]);
		expect(sections.hostedAgents.map((policy) => policy.id)).toEqual(['hosted-user']);
		expect(hasAnyCurrentAccess(sections)).toBe(true);
	});

	it('normalizes the resources each policy grants', async () => {
		vi.spyOn(AdminService, 'listAccessControlRules').mockResolvedValue([
			{
				id: 'mcp-user',
				displayName: 'User MCP',
				created: '2026-01-01T00:00:00Z',
				subjects: [{ type: 'user', id: 'user-1' }],
				resources: [{ type: 'mcpServerCatalogEntry', id: 'entry-1' }]
			}
		]);
		vi.spyOn(AdminService, 'listAllUserWorkspaceAccessControlRules').mockResolvedValue([]);
		vi.spyOn(AdminService, 'listModelAccessPolicies').mockResolvedValue([
			{
				id: 'model-everyone',
				displayName: 'Everyone Models',
				created: '2026-01-01T00:00:00Z',
				subjects: [{ type: 'selector', id: '*' }],
				models: [{ id: 'model-1' }, { id: '*' }]
			}
		]);
		vi.spyOn(AdminService, 'listSkillAccessPolicies').mockResolvedValue([]);
		vi.spyOn(AdminService, 'listHostedAgentAccessPolicies').mockResolvedValue([]);

		const sections = await loadCurrentAccess(userTarget);

		expect(sections.mcp[0]?.resources).toEqual([{ type: 'mcpServerCatalogEntry', id: 'entry-1' }]);
		// A model policy stores bare ids, so the wildcard becomes a selector resource.
		expect(sections.models[0]?.resources).toEqual([
			{ type: 'model', id: 'model-1' },
			{ type: 'selector', id: '*' }
		]);
	});
});
