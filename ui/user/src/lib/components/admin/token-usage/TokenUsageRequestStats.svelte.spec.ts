import { worker } from '../../../../tests/mocks/worker';
import TokenUsageRequestStats from './TokenUsageRequestStats.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function totals(overrides: Record<string, number> = {}) {
	return {
		requests: 0,
		successes: 0,
		fallbacks: 0,
		avgDurationMs: 0,
		inputTokens: 0,
		outputTokens: 0,
		totalTokens: 0,
		totalSpend: 0,
		...overrides
	};
}

describe('TokenUsageRequestStats', () => {
	it('shows request statistics, spend by provider and the top models', async () => {
		const requests = vi.fn();
		worker.use(
			http.get('*/api/token-usage-stats', ({ request }) => {
				requests(new URL(request.url).searchParams);
				return HttpResponse.json({
					start: '2026-10-01T00:00:00Z',
					end: '2026-10-01T06:00:00Z',
					bucketSeconds: 3600,
					totals: totals({ requests: 4, successes: 3, fallbacks: 1, avgDurationMs: 250 }),
					buckets: [
						{ time: '2026-10-01T00:00:00Z', ...totals({ requests: 3, successes: 3 }) },
						{ time: '2026-10-01T01:00:00Z', ...totals({ requests: 1 }) }
					],
					providers: [
						{ provider: 'anthropic', ...totals({ requests: 2, totalSpend: 1.5 }) },
						{ provider: 'openai', ...totals({ requests: 2, totalSpend: 0.25 }) }
					],
					topModelsByRequests: [
						{
							model: 'openai/gpt-5',
							provider: 'openai',
							...totals({ requests: 2, successes: 1, totalTokens: 1200, avgDurationMs: 300 })
						}
					],
					topModelsBySpend: [
						{
							model: 'anthropic/claude-sonnet-4-5',
							provider: 'anthropic',
							...totals({ requests: 2, successes: 2, totalSpend: 1.5 })
						}
					]
				});
			})
		);

		render(TokenUsageRequestStats, {
			startDate: new Date('2026-10-01T00:00:00Z'),
			endDate: new Date('2026-10-01T06:00:00Z'),
			filters: { userIDs: ['u1'], models: ['openai/gpt-5'], apiKeyIDs: ['7'] },
			modelLabel: (model: string) => (model === 'openai/gpt-5' ? 'GPT-5' : model)
		});

		await vi.waitFor(() => expect(requests).toHaveBeenCalled());
		const params = requests.mock.calls[0][0] as URLSearchParams;
		expect(params.getAll('user_id')).toEqual(['u1']);
		expect(params.getAll('model')).toEqual(['openai/gpt-5']);
		expect(params.getAll('api_key_id')).toEqual(['7']);

		await expect.element(page.getByText('75.0%')).toBeVisible();
		await expect.element(page.getByText('$1.50').first()).toBeVisible();
		await expect.element(page.getByRole('cell', { name: 'GPT-5' })).toBeVisible();
		await expect.element(page.getByRole('cell', { name: '50.0%' })).toBeVisible();

		await page.getByRole('button', { name: 'Top Models by Spend' }).click();
		await expect
			.element(page.getByRole('cell', { name: 'anthropic/claude-sonnet-4-5' }))
			.toBeVisible();
	});

	it('says when there were no requests', async () => {
		render(TokenUsageRequestStats, {
			startDate: new Date('2026-10-01T00:00:00Z'),
			endDate: new Date('2026-10-08T00:00:00Z')
		});

		await expect.element(page.getByText('No requests in this period.').first()).toBeVisible();
	});
});
