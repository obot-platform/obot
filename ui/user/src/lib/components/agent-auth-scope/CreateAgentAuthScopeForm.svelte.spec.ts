import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateAgentAuthScopeForm from './CreateAgentAuthScopeForm.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('offers vMCPs and submits their IDs as API-key scopes', async () => {
	const vmcp = createVMCP({ id: 'vmcp1mail', displayName: 'Mail vMCP' });
	const submitted = vi.fn();
	worker.use(
		http.get('/api/vmcps', () => HttpResponse.json({ items: [vmcp] })),
		http.post('/api/api-keys', async ({ request }) => {
			submitted(await request.json());
			return HttpResponse.json({ id: 1, key: 'test-key' });
		})
	);
	await preparePageData();
	render(CreateAgentAuthScopeForm, { onCreate: vi.fn(), onCancel: vi.fn() });
	await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Mail key');
	await page.getByRole('button', { name: /Mail vMCP/ }).click();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect
		.poll(() => submitted.mock.calls[0]?.[0])
		.toMatchObject({
			name: 'Mail key',
			mcpServerIds: [vmcp.id]
		});
});
