import { page as appPage } from '$app/state';
import type { ScheduledAuditLogExport } from '$lib/services';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import CreateScheduleForm from './CreateScheduleForm.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

afterEach(() => appPage.url.searchParams.delete('mcp_server'));

test('preserves comma-containing server names when creating, editing and viewing schedules', async () => {
	const name = 'Outlook, Calendar';
	appPage.url.searchParams.set('mcp_server', JSON.stringify([name]));
	const onSubmit = vi.fn();
	worker.use(
		http.get('/api/mcp-audit-logs/filter-options/:filter', () =>
			HttpResponse.json({ options: [name] })
		),
		http.post('/api/scheduled-audit-log-exports', async ({ request }) =>
			HttpResponse.json({ ...((await request.json()) as object), id: 'schedule-1' })
		),
		http.patch('/api/scheduled-audit-log-exports/schedule-1', async ({ request }) =>
			HttpResponse.json(await request.json())
		)
	);
	await preparePageData();
	const view = await render(CreateScheduleForm, { onCancel: vi.fn(), onSubmit });
	await expect.element(page.getByCSS('#mcp_server')).toHaveTextContent(name);
	await page.getByLabelText('Schedule Name').fill('Comma names');
	await page.getByLabelText('Bucket Name').fill('audit-logs');
	await page.getByRole('button', { name: 'Create Schedule', exact: true }).click();
	await vi.waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
	const exported = onSubmit.mock.calls[0][0] as ScheduledAuditLogExport;
	expect(exported.filters?.mcpServers).toEqual([name]);
	await view.unmount();
	const edit = await render(CreateScheduleForm, {
		onCancel: vi.fn(),
		onSubmit,
		mode: 'edit',
		initialData: exported
	});
	await expect.element(page.getByCSS('#mcp_server')).toHaveTextContent(name);
	await page.getByRole('button', { name: 'Save Changes', exact: true }).click();
	await vi.waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));
	expect(onSubmit.mock.calls[1][0].filters.mcpServers).toEqual([name]);
	await edit.unmount();
	await render(CreateScheduleForm, {
		onCancel: vi.fn(),
		onSubmit: vi.fn(),
		mode: 'view',
		initialData: exported
	});
	await expect.element(page.getByCSS('#mcp_server')).toHaveTextContent(name);
});
