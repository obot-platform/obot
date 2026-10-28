import { page as appPage } from '$app/state';
import type { MCPFilter } from '$lib/services';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import FilterPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function createFilter(overrides: Partial<MCPFilter>): MCPFilter {
	return {
		id: 'mwv1filter',
		created: '2026-01-01T00:00:00Z',
		type: 'mcpwebhookvalidation',
		hasSecret: false,
		configured: true,
		name: 'My filter',
		resources: [{ id: 'default', type: 'mcpCatalog' }],
		selectors: [],
		...overrides
	};
}

function mockFilterApis() {
	const serverCalls = vi.fn();
	worker.use(
		http.get('/api/llm-gateway/guardrails', () =>
			HttpResponse.json({ items: [{ id: 'gp1secrets', name: 'Secrets', type: 'secrets' }] })
		),
		http.get('/api/llm-gateway/guardrails/providers', () => HttpResponse.json({ items: [] })),
		http.post('/api/mcp-webhook-validations/:id/:action', ({ params }) => {
			serverCalls(params.action);
			return HttpResponse.json({ error: 'not found' }, { status: 404 });
		}),
		http.get('/api/mcp-webhook-validations/:id/:action', ({ params }) => {
			serverCalls(params.action);
			return HttpResponse.json({ error: 'not found' }, { status: 404 });
		})
	);
	return serverCalls;
}

async function renderFilterPage(filter: MCPFilter) {
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	const data = await preparePageData<{ filter: MCPFilter }>({ filter });
	return render(FilterPage, { data });
}

afterEach(() => {
	appPage.url.searchParams.delete('view');
});

describe('/mcp-servers/filters/[id]', () => {
	it('shows only the guardrail form for a guardrail filter', async () => {
		const serverCalls = mockFilterApis();
		await renderFilterPage(
			createFilter({
				name: 'Guardrail filter',
				guardrail: { guardrailIDs: ['gp1secrets'], action: 'block' }
			})
		);

		// The configuration is all there is, so the page has no tab bar.
		await expect
			.element(page.getByRole('button', { name: 'Audit Logs', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Server Details', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Usage', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('checkbox', { name: /Secrets/ })).toBeChecked();
		await expect.element(page.getByLabelText('Webhook URL')).not.toBeInTheDocument();
		expect(serverCalls).not.toHaveBeenCalled();
	});

	it('keeps server tabs for a webhook filter', async () => {
		const serverCalls = mockFilterApis();
		await renderFilterPage(
			createFilter({ name: 'Webhook filter', url: 'https://example.com/hook' })
		);

		await expect
			.element(page.getByRole('button', { name: 'Server Details', exact: true }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Usage', exact: true })).toBeVisible();
		await expect
			.element(page.getByLabelText('Webhook URL'))
			.toHaveValue('https://example.com/hook');
		// The form reveals the filter's saved values; let that finish while the mock answers it.
		await vi.waitFor(() => expect(serverCalls).toHaveBeenCalledWith('reveal'));
	});
});
