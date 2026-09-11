import { Group } from '$lib/services';
import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ViewModifyCatalogEntry from './ViewModifyCatalogEntry.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('catalog entry hydration', () => {
	it.each([
		{ role: 'user', groups: [], endpoint: '/api/all-mcps/entries' },
		{ role: 'admin', groups: [Group.ADMIN], endpoint: '/api/mcp-catalogs/default/entries' },
		{ role: 'auditor', groups: [Group.AUDITOR], endpoint: '/api/mcp-catalogs/default/entries' }
	])('loads details through the $role endpoint', async ({ groups, endpoint }) => {
		const entry = createMCPCatalogEntry({ id: 'entry-hydration', name: 'Initial entry' });
		const requested = vi.fn();
		worker.use(
			http.get(`${endpoint}/${entry.id}`, () => {
				requested();
				return HttpResponse.json({
					...entry,
					manifest: { ...entry.manifest, name: 'Hydrated entry' }
				});
			})
		);
		await preparePageData({ profile: createMockProfile(groups) });
		const { component } = await render(ViewModifyCatalogEntry);
		await component.open(entry);
		expect(requested).toHaveBeenCalledOnce();
		await expect
			.element(page.getByRole('heading', { name: 'Hydrated entry', exact: true }).first())
			.toBeVisible();
	});
});
