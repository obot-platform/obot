import { page as appPage } from '$app/state';
import type { AuditLogExport } from '$lib/services';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import CreateAuditLogExportForm from './CreateAuditLogExportForm.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

afterEach(() => (appPage.url as URL).searchParams.delete('mcp_server'));

test('preserves comma-containing server names when creating and viewing exports', async () => {
	const name = 'Outlook, Calendar';
	(appPage.url as URL).searchParams.set('mcp_server', JSON.stringify([name]));
	const onSubmit = vi.fn();
	worker.use(
		http.get('/api/mcp-audit-logs/filter-options/:filter', () =>
			HttpResponse.json({ options: [name] })
		),
		http.post('/api/audit-log-exports', async ({ request }) =>
			HttpResponse.json(await request.json())
		)
	);
	await preparePageData();
	const view = await render(CreateAuditLogExportForm, { onCancel: vi.fn(), onSubmit });
	await expect.element(page.getByCSS('#mcp_server')).toHaveTextContent(name);
	await page.getByLabelText('Export Name').fill('Comma names');
	await page.getByLabelText('Bucket Name').fill('audit-logs');
	await vi.waitFor(() =>
		expect(page.getByRole('button', { name: 'Create Export', exact: true })).toBeEnabled()
	);
	// A forced pointer click lands wherever the button was, so a layout shift or overlay can
	// swallow it; clicking the element itself always submits the form.
	(
		page.getByRole('button', { name: 'Create Export', exact: true }).element() as HTMLElement
	).click();
	await vi.waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
	const exported = onSubmit.mock.calls[0][0] as AuditLogExport;
	expect(exported.filters?.mcpServers).toEqual([name]);
	await view.unmount();
	await render(CreateAuditLogExportForm, {
		onCancel: vi.fn(),
		onSubmit: vi.fn(),
		mode: 'view',
		initialData: exported
	});
	await expect.element(page.getByCSS('#mcp_server')).toHaveTextContent(name);
});

// Picks options in the multi-select labelled `name`. Picking one closes the dropdown.
async function pickOptions(name: string, options: string[]) {
	for (const option of options) {
		await page.getByRole('combobox', { name, exact: true }).click();
		await page.getByRole('button', { name: option, exact: true }).last().click();
	}
}

test('offers the admin change type with only its own filters, and sends them', async () => {
	const filterOptions = vi.fn();
	const onSubmit = vi.fn();
	let body: Record<string, unknown> | undefined;
	worker.use(
		http.get('/api/mcp-audit-logs/filter-options/:filter', () => {
			filterOptions();
			return HttpResponse.json({ options: [] });
		}),
		http.get('/api/llm-audit-logs/filter-options/:filter', () => {
			filterOptions();
			return HttpResponse.json({ options: [] });
		}),
		http.post('/api/audit-log-exports', async ({ request }) => {
			body = (await request.json()) as Record<string, unknown>;
			return HttpResponse.json({ ...body, id: 'export-1' });
		})
	);
	await preparePageData();
	const view = await render(CreateAuditLogExportForm, {
		onCancel: vi.fn(),
		onSubmit,
		logType: 'admin'
	});

	await expect
		.element(page.getByLabelText('Key Prefix (Optional)'))
		.toHaveAttribute('placeholder', expect.stringContaining('admin-audit-events'));
	await expect.element(page.getByText('Log Sources')).not.toBeInTheDocument();
	await page.getByRole('button', { name: 'Advanced Options' }).click();
	await expect.element(page.getByRole('combobox', { name: 'Resource Kinds' })).toBeVisible();
	await expect.element(page.getByRole('combobox', { name: 'Actions', exact: true })).toBeVisible();
	await expect.element(page.getByRole('combobox', { name: 'Actors', exact: true })).toBeVisible();
	await expect.element(page.getByLabelText('Resource IDs')).toBeVisible();
	await expect.element(page.getByLabelText('Search Query')).not.toBeInTheDocument();
	await expect.element(page.getByText('Model Providers')).not.toBeInTheDocument();
	await expect.element(page.getByText('MCP Servers')).not.toBeInTheDocument();
	expect(filterOptions).not.toHaveBeenCalled();

	await page.getByLabelText('Export Name').fill('Agent changes');
	await page.getByLabelText('Bucket Name').fill('audit-logs');
	await pickOptions('Resource Kinds', ['Agent', 'API Key']);
	await pickOptions('Actions', ['agent.key.revoke']);
	await pickOptions('Actors', ['System']);
	await page.getByLabelText('Resource IDs').fill('7, 12');
	// Clicking the element itself always submits the form; see the test above.
	(
		page.getByRole('button', { name: 'Create Export', exact: true }).element() as HTMLElement
	).click();

	await vi.waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
	expect(body).toMatchObject({
		name: 'Agent changes',
		type: 'admin',
		bucket: 'audit-logs',
		adminFilters: {
			// The order is the order picked in, which means nothing to the filter.
			resourceKinds: expect.arrayContaining(['agent', 'apiKey']),
			resourceIDs: ['7', '12'],
			actors: ['system'],
			actions: ['agent.key.revoke']
		}
	});
	expect((body?.adminFilters as { resourceKinds: string[] }).resourceKinds).toHaveLength(2);
	expect(body).not.toHaveProperty('filters');
	expect(body).not.toHaveProperty('llmFilters');

	await view.unmount();
	await render(CreateAuditLogExportForm, {
		onCancel: vi.fn(),
		onSubmit: vi.fn(),
		mode: 'view',
		logType: 'admin',
		initialData: onSubmit.mock.calls[0][0] as AuditLogExport
	});
	await expect.element(page.getByCSS('#admin_action')).toHaveTextContent('agent.key.revoke');
	await expect.element(page.getByLabelText('Resource IDs')).toHaveValue('7,12');
});
