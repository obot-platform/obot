import type { MCPFilterResource, MCPFilterWebhookSelector } from '$lib/services';
import { createVMCP } from '../../../tests/helpers/mcp';
import { worker } from '../../../tests/mocks/worker';
import SelectorsAndResourcesFormSegment from './SelectorsAndResourcesFormSegment.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('searches for a vMCP and adds it as an mcpServer filter resource', async () => {
	const vmcp = createVMCP({ id: 'vmcp1search', displayName: 'Search gateway' });
	worker.use(
		http.get('/api/vmcps', ({ request }) =>
			HttpResponse.json({
				items: new URL(request.url).searchParams.get('all') === 'true' ? [vmcp] : []
			})
		)
	);
	const form = $state<{ selectors: MCPFilterWebhookSelector[]; resources: MCPFilterResource[] }>({
		selectors: [],
		resources: []
	});
	await render(SelectorsAndResourcesFormSegment, { form });
	await page.getByRole('button', { name: 'Add MCP Server', exact: true }).click();
	await page.getByPlaceholder('Search by name...').fill('Search gateway');
	await page.getByRole('button', { name: /Search gateway/ }).click();
	await page.getByRole('button', { name: 'Confirm', exact: true }).click();
	await expect
		.element(page.getByRole('cell', { name: 'Search gateway', exact: true }))
		.toBeVisible();
	expect(form.resources).toEqual([{ id: vmcp.id, name: vmcp.id, type: 'mcpServer' }]);

	await page.getByRole('button', { name: 'Add MCP Server', exact: true }).click();
	await expect
		.element(page.getByRole('button', { name: /Search gateway/ }))
		.not.toBeInTheDocument();
});

it('resolves an existing vMCP resource to its display name', async () => {
	const vmcp = createVMCP({ id: 'vmcp1existing', displayName: 'Existing gateway' });
	worker.use(http.get('/api/vmcps', () => HttpResponse.json({ items: [vmcp] })));
	await render(SelectorsAndResourcesFormSegment, {
		form: { selectors: [], resources: [{ id: vmcp.id, type: 'mcpServer' }] },
		readonly: true
	});
	await expect
		.element(page.getByRole('cell', { name: 'Existing gateway', exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Add MCP Server', exact: true }))
		.not.toBeInTheDocument();
});
