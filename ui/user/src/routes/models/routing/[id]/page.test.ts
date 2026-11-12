import { Group } from '$lib/services';
import { createMockProfile, createPageData } from '../../../../tests/helpers/pageData';
import { load } from './+page';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const getRoutingRule = vi.hoisted(() => vi.fn());

vi.mock('$lib/services/llm-gateway/gateway', () => ({
	getRoutingRule
}));

const rule = {
	id: 'r1',
	name: 'Cheaper GPT',
	cel_expression: 'model == "gpt-4o"',
	targets: [{ provider: null, model: 'gpt-4o-mini', weight: 1 }]
};

function loadPage(groups: string[]) {
	return load({
		params: { id: 'r1' },
		fetch: vi.fn(),
		parent: async () => createPageData({ profile: createMockProfile(groups) }),
		url: new URL('http://localhost/models/routing/r1')
	} as unknown as Parameters<NonNullable<typeof load>>[0]);
}

describe('models/routing/[id] loader', () => {
	beforeEach(() => {
		getRoutingRule.mockReset();
		getRoutingRule.mockResolvedValue(rule);
	});

	it('loads the rule for administrators and auditors', async () => {
		await expect(loadPage([Group.ADMIN])).resolves.toEqual({ rule });
		await expect(loadPage([Group.USER, Group.AUDITOR])).resolves.toEqual({ rule });
		expect(getRoutingRule).toHaveBeenCalledWith(
			'r1',
			expect.objectContaining({ fetch: expect.any(Function) })
		);
	});

	it('sends everyone else home', async () => {
		await expect(loadPage([Group.USER])).rejects.toMatchObject({ status: 307, location: '/' });
		expect(getRoutingRule).not.toHaveBeenCalled();
	});
});
