import { Group, type APIKey } from '$lib/services';
import { preparePageData, createMockProfile } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import Details from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/url', async (original) => ({
	...(await original<typeof import('$lib/url')>()),
	goto: vi.fn()
}));

it.each([[Group.USER], [Group.ADMIN_READONLY, Group.USER], [Group.USER, Group.AUDITOR]])(
	'revokes an owned identity through its personal endpoint for %s',
	async (...groups) => {
		const profile = { ...createMockProfile(groups as string[]), id: '7' };
		const ownDelete = vi.fn();
		const adminDelete = vi.fn();
		worker.use(
			http.delete('*/api/api-keys/42', () => {
				ownDelete();
				return new HttpResponse(null, { status: 204 });
			}),
			http.delete('*/api/admin-api-keys/42', () => {
				adminDelete();
				return new HttpResponse(null, { status: 403 });
			})
		);
		const apiKey = {
			id: 42,
			userId: Number(profile.id),
			name: 'Own identity',
			canAccessAPI: false,
			canAccessLLMProxy: false,
			canAccessSkills: true,
			canAccessDeviceScans: false,
			mcpServerIds: [],
			createdAt: '2026-01-01T00:00:00Z'
		} as APIKey;
		const data = await preparePageData({
			profile,
			apiKey,
			isAdmin: groups.includes(Group.ADMIN_READONLY) || groups.includes(Group.AUDITOR)
		});
		render(Details, { data: data as never });
		await page.getByRole('button', { name: 'Delete Own identity', exact: true }).click();
		await page
			.getByRole('dialog')
			.getByRole('button', { name: "Yes, I'm sure", exact: true })
			.click();
		await vi.waitFor(() => expect(ownDelete).toHaveBeenCalledOnce());
		expect(adminDelete).not.toHaveBeenCalled();
	}
);
