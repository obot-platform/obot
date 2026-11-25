import { page as appPage } from '$app/state';
import { TOKEN_USAGE_PARAMS } from '$lib/components/admin/token-usage/constants';
import { worker } from '../../tests/mocks/worker';
import LlmUsageView from './LlmUsageView.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

// The URL is reactive, and picking a filter changes it, so a filter can be followed from the select to the request.
vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

vi.mock('$lib/url', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/url')>();
	const { applyTestGoto } = await import('../../tests/helpers/navigation.svelte');
	return {
		...actual,
		goto: (url: string | URL) => applyTestGoto(url),
		replaceState: (url: string | URL) => applyTestGoto(url)
	};
});

afterEach(() => {
	// Unmounted first, so that resetting the URL does not make it ask for anything after the handlers are reset.
	cleanup();
	(appPage.url as URL).searchParams.delete(TOKEN_USAGE_PARAMS.USER);
	(appPage.url as URL).searchParams.delete(TOKEN_USAGE_PARAMS.ON_BEHALF_OF);
});

function mockApis() {
	const stats: URLSearchParams[] = [];
	worker.use(
		http.get('*/api/users', () =>
			HttpResponse.json({
				items: [
					{ id: '3', displayName: 'Ada Admin', status: 'active' },
					{ id: '4', displayName: 'Bea Builder', status: 'active' }
				]
			})
		),
		http.get('*/api/models', () => HttpResponse.json({ items: [] })),
		http.get('*/api/total-token-usage', () => HttpResponse.json({})),
		http.get('*/api/token-usage', () => HttpResponse.json({ items: [] })),
		http.get('*/api/token-usage-stats', ({ request }) => {
			stats.push(new URL(request.url).searchParams);
			return HttpResponse.json({
				start: '2026-10-01T00:00:00Z',
				end: '2026-10-08T00:00:00Z',
				bucketSeconds: 10800,
				totals: {},
				buckets: [],
				providers: [],
				topModelsByRequests: [],
				topModelsBySpend: []
			});
		})
	);
	return stats;
}

const onBehalfOfPill = () => page.getByCSS('.filter-primary').filter({ hasText: 'On behalf of:' });

describe('LlmUsageView on-behalf-of filter', () => {
	it('puts the user picked in the URL and asks for the requests made on their behalf', async () => {
		const stats = mockApis();
		render(LlmUsageView);

		await page.getByRole('combobox', { name: 'Filter by who agents acted for...' }).click();
		await page.getByRole('button', { name: 'Bea Builder', exact: true }).last().click();

		await vi.waitFor(() =>
			expect(appPage.url.searchParams.getAll(TOKEN_USAGE_PARAMS.ON_BEHALF_OF)).toEqual(['4'])
		);
		await vi.waitFor(() => expect(stats.at(-1)?.getAll('on_behalf_of')).toEqual(['4']));
		await expect.element(onBehalfOfPill().filter({ hasText: 'Bea Builder' })).toBeVisible();

		await onBehalfOfPill().getByRole('button').click();
		await vi.waitFor(() =>
			expect(appPage.url.searchParams.has(TOKEN_USAGE_PARAMS.ON_BEHALF_OF)).toBe(false)
		);
		await expect.element(onBehalfOfPill()).not.toBeInTheDocument();
	});

	it('reads the users from the URL, alongside the user filter', async () => {
		const stats = mockApis();
		(appPage.url as URL).searchParams.append(TOKEN_USAGE_PARAMS.USER, 'agent:7');
		(appPage.url as URL).searchParams.append(TOKEN_USAGE_PARAMS.ON_BEHALF_OF, '3');
		(appPage.url as URL).searchParams.append(TOKEN_USAGE_PARAMS.ON_BEHALF_OF, '4');

		render(LlmUsageView);

		await vi.waitFor(() => expect(stats.length).toBeGreaterThan(0));
		const params = stats.at(-1)!;
		expect(params.getAll('user_id')).toEqual(['agent:7']);
		expect(params.getAll('on_behalf_of')).toEqual(['3', '4']);
		await expect.element(onBehalfOfPill().filter({ hasText: 'Ada Admin' })).toBeVisible();
		await expect.element(onBehalfOfPill().filter({ hasText: 'Bea Builder' })).toBeVisible();
	});
});
