import { page as appPage } from '$app/state';
import type { ScheduledAuditLogExport } from '$lib/services';
import { preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import CreateScheduleForm from './CreateScheduleForm.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, test, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

afterEach(() => (appPage.url as URL).searchParams.delete('mcp_server'));

test('preserves comma-containing server names when creating, editing and viewing schedules', async () => {
	const name = 'Outlook, Calendar';
	(appPage.url as URL).searchParams.set('mcp_server', JSON.stringify([name]));
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
	await expect.element(page.getByCSS('#schedule-hour')).toHaveAttribute('tabindex', '-1');
	await expect.element(page.getByRole('button', { name: 'Save Changes' })).not.toBeInTheDocument();
});

// Picks options in the multi-select labelled `name`. Picking one closes the dropdown.
async function pickOptions(name: string, options: string[]) {
	for (const option of options) {
		await page.getByRole('combobox', { name, exact: true }).click();
		await page.getByRole('button', { name: option, exact: true }).last().click();
	}
}

test('offers the admin change type with only its own filters, and sends them when creating and editing', async () => {
	const filterOptions = vi.fn();
	const onSubmit = vi.fn();
	let created: Record<string, unknown> | undefined;
	let updated: Record<string, unknown> | undefined;
	worker.use(
		http.get('/api/mcp-audit-logs/filter-options/:filter', () => {
			filterOptions();
			return HttpResponse.json({ options: [] });
		}),
		http.get('/api/llm-audit-logs/filter-options/:filter', () => {
			filterOptions();
			return HttpResponse.json({ options: [] });
		}),
		http.post('/api/scheduled-audit-log-exports', async ({ request }) => {
			created = (await request.json()) as Record<string, unknown>;
			return HttpResponse.json({ ...created, id: 'schedule-1' });
		}),
		http.patch('/api/scheduled-audit-log-exports/schedule-1', async ({ request }) => {
			updated = (await request.json()) as Record<string, unknown>;
			return HttpResponse.json({ ...updated, id: 'schedule-1' });
		})
	);
	await preparePageData();
	const view = await render(CreateScheduleForm, { onCancel: vi.fn(), onSubmit, logType: 'admin' });

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

	await page.getByLabelText('Schedule Name').fill('Profile changes');
	await page.getByLabelText('Bucket Name').fill('audit-logs');
	await pickOptions('Resource Kinds', ['vMCP Profile']);
	await pickOptions('Actions', ['vmcp.profile.configuration.update']);
	await page.getByLabelText('Resource IDs').fill('profile-1');
	await page.getByRole('button', { name: 'Create Schedule', exact: true }).click();

	await vi.waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
	expect(created).toMatchObject({
		name: 'Profile changes',
		type: 'admin',
		bucket: 'audit-logs',
		adminFilters: {
			resourceKinds: ['vmcpProfile'],
			resourceIDs: ['profile-1'],
			actors: [],
			actions: ['vmcp.profile.configuration.update']
		}
	});
	expect(created).not.toHaveProperty('filters');
	expect(created).not.toHaveProperty('llmFilters');

	await view.unmount();
	await render(CreateScheduleForm, {
		onCancel: vi.fn(),
		onSubmit,
		mode: 'edit',
		logType: 'admin',
		initialData: onSubmit.mock.calls[0][0] as ScheduledAuditLogExport
	});
	await expect
		.element(page.getByCSS('#admin_action'))
		.toHaveTextContent('vmcp.profile.configuration.update');
	await pickOptions('Actors', ['System']);
	await page.getByRole('button', { name: 'Save Changes', exact: true }).click();
	await vi.waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));
	expect(updated).toMatchObject({
		type: 'admin',
		adminFilters: {
			resourceKinds: ['vmcpProfile'],
			resourceIDs: ['profile-1'],
			actors: ['system'],
			actions: ['vmcp.profile.configuration.update']
		}
	});
});

test('explains initial backfill and incremental exports when creating a schedule', async () => {
	await preparePageData();
	await render(CreateScheduleForm, { onCancel: vi.fn(), onSubmit: vi.fn() });
	await expect.element(page.getByRole('heading', { name: 'Initial Backfill' })).toBeVisible();
	await expect
		.element(page.getByText('Later exports include new and updated logs', { exact: false }))
		.toBeVisible();
	await expect
		.element(page.getByText('with a five-second overlap', { exact: false }))
		.toBeVisible();
});

test('explains that changing backfill on an initialized schedule does not replay history', async () => {
	await preparePageData();
	const initialData = {
		id: 'schedule',
		name: 'Daily logs',
		type: 'mcp',
		enabled: true,
		bucket: 'audit-logs',
		retentionPeriodInDays: 7,
		schedule: { interval: 'daily', hour: 3, minute: 0, day: 0, weekday: 0, timezone: 'UTC' },
		lastExportedThrough: '2026-07-01T00:00:00Z'
	} as ScheduledAuditLogExport;
	await render(CreateScheduleForm, {
		onCancel: vi.fn(),
		onSubmit: vi.fn(),
		mode: 'edit',
		initialData
	});
	await expect.element(page.getByRole('heading', { name: 'Initial Backfill' })).toBeVisible();
	await expect
		.element(
			page.getByText(
				'Changing this setting after the first successful export does not replay history.',
				{ exact: false }
			)
		)
		.toBeVisible();
});
