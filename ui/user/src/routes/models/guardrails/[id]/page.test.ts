import { Group } from '$lib/services';
import { createMockProfile, createPageData } from '../../../../tests/helpers/pageData';
import { load } from './+page';
import { describe, expect, it, vi } from 'vitest';

function loadPage(groups: string[]) {
	return load({
		fetch: vi.fn(),
		parent: async () => createPageData({ profile: createMockProfile(groups) }),
		url: new URL('http://localhost/models/guardrails/[id]')
	} as unknown as Parameters<NonNullable<typeof load>>[0]);
}

describe('models/guardrails/[id] loader', () => {
	it('lets administrators and auditors in', async () => {
		await expect(loadPage([Group.ADMIN])).resolves.toEqual({});
		await expect(loadPage([Group.USER, Group.AUDITOR])).resolves.toEqual({});
	});

	it('sends everyone else home', async () => {
		await expect(loadPage([Group.USER])).rejects.toMatchObject({ status: 307, location: '/' });
	});
});
