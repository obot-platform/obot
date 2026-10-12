import { page as appPage } from '$app/state';
import { TOKEN_USAGE_PARAMS } from '$lib/components/admin/token-usage/constants';
import { worker } from '../../tests/mocks/worker';
import LlmUsageView from './LlmUsageView.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('LlmUsageView', () => {
	afterEach(() => {
		appPage.url.searchParams.delete(TOKEN_USAGE_PARAMS.USER);
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
		appPage.url.searchParams.set(TOKEN_USAGE_PARAMS.USER, 'u2');

		render(LlmUsageView);

		// The user filter's pill, rather than the option of the same name in the closed dropdown.
		await expect
			.element(page.getByCSS('.filter-primary'))
			.toHaveTextContent('User:Disabled Dan (disabled)');
	});
});
