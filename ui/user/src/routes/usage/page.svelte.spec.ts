import { page as appPage } from '$app/state';
import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import UsagePage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function renderUsagePage(groups: string[], view: string) {
	(appPage.url as URL).searchParams.set('view', view);
	await preparePageData({ profile: createMockProfile(groups) });
	return render(UsagePage);
}

describe('Usage page model tab', () => {
	it('shows model usage, with request statistics, to admins and no gateway tab', async () => {
		const stats = vi.fn();
		worker.use(
			http.get('*/api/users', () => HttpResponse.json({ items: [] })),
			http.get('*/api/models', () => HttpResponse.json({ items: [] })),
			http.get('*/api/total-token-usage', () => HttpResponse.json({})),
			http.get('*/api/token-usage', () => HttpResponse.json({ items: [] })),
			http.get('*/api/token-usage-stats', () => {
				stats();
				return HttpResponse.json({
					start: '2026-10-01T00:00:00Z',
					end: '2026-10-08T00:00:00Z',
					bucketSeconds: 10800,
					totals: {
						requests: 0,
						successes: 0,
						fallbacks: 0,
						avgDurationMs: 0,
						inputTokens: 0,
						outputTokens: 0,
						totalTokens: 0,
						totalSpend: 0
					},
					buckets: [],
					providers: [],
					topModelsByRequests: [],
					topModelsBySpend: []
				});
			})
		);
		await renderUsagePage([Group.ADMIN], 'llm');

		await expect.element(page.getByRole('button', { name: 'Model', exact: true })).toBeVisible();
		await vi.waitFor(() => expect(stats).toHaveBeenCalled());
		await expect
			.element(page.getByRole('button', { name: 'Gateway', exact: true }))
			.not.toBeInTheDocument();
	});

	it('hides the model tab from users without admin access', async () => {
		const mcpStats = vi.fn();
		worker.use(
			http.get('/api/mcp-stats', () => {
				mcpStats();
				return HttpResponse.json({
					items: [],
					timeStart: '',
					timeEnd: '',
					totalCalls: 0,
					uniqueUsers: 0
				});
			})
		);
		await renderUsagePage([Group.USER], 'llm');

		await expect
			.element(page.getByRole('button', { name: 'Model', exact: true }))
			.not.toBeInTheDocument();
		// The MCP tab is shown instead; let its stats request settle inside this test.
		await vi.waitFor(() => expect(mcpStats).toHaveBeenCalled());
	});
});
