import { page as appPage } from '$app/state';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { worker } from '../../tests/mocks/worker';
import { preparePageData } from '../../tests/helpers/pageData';
import DeviceClients from './DeviceClients.svelte';

vi.mock('$app/navigation', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/navigation')>();
	return {
		...actual,
		replaceState: vi.fn()
	};
});

function mockDeviceClientApis(requests: URL[]) {
	worker.use(
		http.get('/api/users', () => HttpResponse.json({ items: [] })),
		http.get('/api/devices/clients', ({ request }) => {
			requests.push(new URL(request.url));
			return HttpResponse.json({
				items: [
					{
						name: 'claude-code',
						users: [],
						skills: [],
						mcpServers: []
					}
				],
				total: 1,
				offset: 0,
				limit: 25
			});
		})
	);
}

afterEach(() => {
	appPage.url.searchParams.delete('name');
	appPage.url.searchParams.delete('offset');
	appPage.url.searchParams.delete('sort');
	appPage.url.searchParams.delete('sortDirection');
	vi.restoreAllMocks();
});

describe('DeviceClients search', () => {
	it('shows the branded name and searches using the raw client ID', async () => {
		const requests: URL[] = [];
		mockDeviceClientApis(requests);
		await preparePageData();
		render(DeviceClients);

		await expect.element(page.getByText('Claude Code', { exact: true })).toBeVisible();

		await page.getByPlaceholder('Search by client name...').fill('Claude Code');

		await vi.waitFor(() => {
			expect(requests.at(-1)?.searchParams.get('name')).toBe('claude-code');
		});
	});
});
