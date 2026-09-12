import { page as appPage } from '$app/state';
import type { AuditLogExport } from '$lib/services';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import CreateAuditLogExportForm from './CreateAuditLogExportForm.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

afterEach(() => appPage.url.searchParams.delete('mcp_server'));

test('preserves comma-containing server names when creating and viewing exports', async () => {
	const name = 'Outlook, Calendar';
	appPage.url.searchParams.set('mcp_server', JSON.stringify([name]));
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
	await page.getByRole('button', { name: 'Create Export', exact: true }).click();
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
