import { beforeNavigate } from '$app/navigation';
import { page as appPage } from '$app/state';
import type { MCPFilter } from '$lib/services';
import { goto } from '$lib/url';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import FilterPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', async (importOriginal) => ({
	...(await importOriginal<typeof import('$app/navigation')>()),
	beforeNavigate: vi.fn()
}));
vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	goto: vi.fn()
}));

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
	vi.clearAllMocks();
	vi.restoreAllMocks();
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

it('keeps the saved filter editor open with the updated name and saved status', async () => {
	mockFilterApis();
	const original = createFilter({ name: 'Webhook filter', url: 'https://example.com/hook' });
	worker.use(
		http.post('/api/mcp-webhook-validations/:id/reveal', () => HttpResponse.json({})),
		http.put('/api/mcp-webhook-validations/:id', async ({ request }) =>
			HttpResponse.json({ ...original, ...((await request.json()) as object) })
		),
		http.post('/api/mcp-webhook-validations/:id/launch', () => HttpResponse.json({}))
	);
	await renderFilterPage(original);
	await page.getByLabelText('Name', { exact: true }).fill('Updated webhook filter');
	await page.getByRole('button', { name: 'Save & apply changes' }).click();
	await expect.element(page.getByRole('status').getByText('Saved', { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('heading', { name: 'Updated webhook filter', exact: true }).first())
		.toBeVisible();
	await expect
		.element(page.getByLabelText('Name', { exact: true }))
		.toHaveValue('Updated webhook filter');
	await expect.element(page.getByRole('button', { name: 'Save & apply changes' })).toBeDisabled();
});

it.each([false, true])(
	'confirmed deletion bypasses the dirty editor guard (guardrail=%s)',
	async (guardrail) => {
		mockFilterApis();
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/mcp-webhook-validations/:id', () => {
				deleted();
				return HttpResponse.json({});
			})
		);
		await renderFilterPage(
			createFilter(
				guardrail
					? { guardrail: { guardrailIDs: ['gp1secrets'], action: 'block' } }
					: { url: 'https://example.com/hook' }
			)
		);
		const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
		await page.getByLabelText('Name', { exact: true }).fill('Unsaved edits');
		await page.getByRole('button', { name: 'Delete Filter', exact: true }).click();
		await page
			.getByRole('dialog')
			.getByRole('button', { name: "Yes, I'm sure", exact: true })
			.click();
		await vi.waitFor(() => expect(goto).toHaveBeenCalled());
		expect(deleted).toHaveBeenCalledOnce();
		const cancel = vi.fn();
		for (const [before] of vi.mocked(beforeNavigate).mock.calls) {
			before({
				type: 'goto',
				from: null,
				to: null,
				complete: Promise.resolve(),
				willUnload: false,
				cancel
			});
		}
		expect(confirm).not.toHaveBeenCalled();
		expect(cancel).not.toHaveBeenCalled();
	}
);
