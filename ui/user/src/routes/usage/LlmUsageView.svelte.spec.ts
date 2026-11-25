import { page as appPage } from '$app/state';
import { TOKEN_USAGE_PARAMS } from '$lib/components/admin/token-usage/constants';
import { worker } from '../../tests/mocks/worker';
import LlmUsageView from './LlmUsageView.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('LlmUsageView', () => {
	afterEach(() => {
		(appPage.url as URL).searchParams.delete(TOKEN_USAGE_PARAMS.USER);
		(appPage.url as URL).searchParams.delete('include_on_behalf_of');
	});

	it('keeps a selected user marked as disabled in the active filter', async () => {
		worker.use(
			http.get('*/api/users', () =>
				HttpResponse.json({
					items: [{ id: 'u2', displayName: 'Disabled Dan', status: 'disabled' }]
				})
			),
			http.get('*/api/models', () => HttpResponse.json({ items: [] })),
			http.get('*/api/total-token-usage', () => HttpResponse.json({})),
			http.get('*/api/token-usage', () => HttpResponse.json({ items: [] }))
		);
		(appPage.url as URL).searchParams.set(TOKEN_USAGE_PARAMS.USER, 'u2');

		render(LlmUsageView);

		// The user filter's pill, rather than the option of the same name in the closed dropdown.
		await expect
			.element(page.getByCSS('.filter-primary'))
			.toHaveTextContent('User:Disabled Dan (disabled)');
	});

	describe('on behalf of a user', () => {
		function mockApis() {
			const stats = vi.fn();
			worker.use(
				http.get('*/api/users', () =>
					HttpResponse.json({ items: [{ id: '3', displayName: 'Ada Admin', status: 'active' }] })
				),
				http.get('*/api/models', () => HttpResponse.json({ items: [] })),
				http.get('*/api/total-token-usage', () => HttpResponse.json({})),
				http.get('*/api/token-usage', () => HttpResponse.json({ items: [] })),
				http.get('*/api/token-usage-stats', ({ request }) => {
					stats(new URL(request.url).searchParams);
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

		const toggle = () =>
			page.getByRole('checkbox', { name: 'Include actions on behalf of this user' });

		it('asks for the requests made on behalf of the selected user when the toggle is on', async () => {
			const stats = mockApis();
			(appPage.url as URL).searchParams.set(TOKEN_USAGE_PARAMS.USER, '3');
			(appPage.url as URL).searchParams.set('include_on_behalf_of', 'true');

			render(LlmUsageView);

			await expect.element(toggle()).toBeChecked();
			await vi.waitFor(() =>
				expect(
					stats.mock.calls.some(([params]) => params.get('include_on_behalf_of') === 'true')
				).toBe(true)
			);
		});

		it('offers the toggle only with a selected user, and leaves the parameter out when it is off', async () => {
			const stats = mockApis();

			render(LlmUsageView);

			await vi.waitFor(() => expect(stats).toHaveBeenCalled());
			await expect.element(toggle()).not.toBeInTheDocument();
			expect(stats.mock.calls.every(([params]) => !params.has('include_on_behalf_of'))).toBe(true);
		});
	});
});
