import { AgentIdentitiesService, Group, UserService } from '$lib/services';
import { createMockProfile } from '../../../../tests/helpers/pageData';
import { createAgent } from '../../../../tests/mocks/agentIdentities';
import { load } from './+page';
import { isRedirect } from '@sveltejs/kit';
import { afterEach, describe, expect, it, vi } from 'vitest';

function loadAgent(groups: string[]) {
	return load({
		fetch: vi.fn(),
		params: { id: '7' },
		parent: vi.fn(async () => ({ profile: createMockProfile(groups) }))
	} as unknown as Parameters<typeof load>[0]);
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('agent page load', () => {
	it.each([
		{ name: 'admins', groups: [Group.ADMIN_READONLY, Group.ADMIN], readonly: false },
		{ name: 'owners', groups: [Group.ADMIN_READONLY, Group.ADMIN, Group.OWNER], readonly: false },
		{ name: 'read-only admins', groups: [Group.USER, Group.ADMIN_READONLY], readonly: true },
		{ name: 'auditors', groups: [Group.USER, Group.AUDITOR], readonly: true }
	])('shows the agent to $name, read-only: $readonly', async ({ groups, readonly }) => {
		vi.spyOn(AgentIdentitiesService, 'getAgent').mockResolvedValue(createAgent());
		vi.spyOn(AgentIdentitiesService, 'listAgentAPIKeys').mockResolvedValue([]);
		vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);

		const result = await loadAgent(groups);

		expect(result).toMatchObject({ agent: { id: '7' }, readonly });
	});

	it('sends anyone without admin access to their API keys', async () => {
		const getAgent = vi.spyOn(AgentIdentitiesService, 'getAgent');

		const err = await Promise.resolve(loadAgent([Group.USER])).catch((e: unknown) => e);

		expect(isRedirect(err)).toBe(true);
		expect(err).toMatchObject({ location: '/identity-access?view=api-keys' });
		expect(getAgent).not.toHaveBeenCalled();
	});
});
