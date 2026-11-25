import { page as appPage } from '$app/state';
import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import AuditLogsPage from './+page.svelte';
import LLMExportsPage from './llm/exports/+page.svelte';
import MCPExportsPage from './mcp/exports/+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, onTestFinished, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function renderAuditLogsPage(groups: string[] = [Group.ADMIN], view = 'llm') {
	(appPage.url as URL).searchParams.set('view', view);
	await preparePageData({ profile: createMockProfile(groups) });
	return render(AuditLogsPage);
}

describe('Audit logs page model tab', () => {
	it.each([
		{ name: 'admins', groups: [Group.ADMIN] },
		{ name: 'auditors', groups: [Group.USER, Group.AUDITOR] }
	])('shows the model tab to $name, and no gateway requests tab', async ({ groups }) => {
		const auditLogs = vi.fn();
		worker.use(
			http.get('/api/llm-audit-logs', () => {
				auditLogs();
				return HttpResponse.json({ items: [], total: 0 });
			})
		);
		await renderAuditLogsPage(groups);

		await expect.element(page.getByRole('button', { name: 'LLM', exact: true })).toBeVisible();
		await vi.waitFor(() => expect(auditLogs).toHaveBeenCalled());
		// Obot's LLM audit log is the one record of gateway requests.
		await expect
			.element(page.getByRole('button', { name: 'Gateway Requests' }))
			.not.toBeInTheDocument();
	});

	it('hides the LLM tab from users without admin access', async () => {
		await renderAuditLogsPage([Group.USER], 'mcp');

		await expect
			.element(page.getByRole('button', { name: 'LLM', exact: true }))
			.not.toBeInTheDocument();
	});

	it.each(['mcp', 'llm'])('lets read-only admins open %s export records', async (view) => {
		await renderAuditLogsPage([Group.USER, Group.ADMIN_READONLY], view);

		await expect.element(page.getByRole('button', { name: 'Manage Exports' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Create Export', exact: true }))
			.not.toBeInTheDocument();
	});
});

describe('Audit logs page Claude Environments', () => {
	it.each([
		{ engine: 'docker' as const, enabled: false, visible: false },
		{ engine: 'kubernetes' as const, enabled: false, visible: false },
		{ engine: 'kubernetes' as const, enabled: true, visible: true }
	])(
		'shows Claude audit logs for $engine with enabled=$enabled',
		async ({ engine, enabled, visible }) => {
			(appPage.url as URL).searchParams.delete('view');
			await preparePageData({ version: { engine, claudeEnvironmentsEnabled: enabled } });
			await render(AuditLogsPage);
			const tab = page.getByRole('button', { name: 'Environments', exact: true });
			if (visible) await expect.element(tab).toBeVisible();
			else await expect.element(tab).not.toBeInTheDocument();
		}
	);
});

describe('Read-Only Admin export pages', () => {
	it.each([
		{ type: 'mcp', ExportsPage: MCPExportsPage },
		{ type: 'llm', ExportsPage: LLMExportsPage }
	])('shows $type jobs and schedules without write controls', async ({ type, ExportsPage }) => {
		const params = (appPage.url as URL).searchParams;
		onTestFinished(() => {
			params.delete('view');
			params.delete('form');
		});
		await preparePageData({ profile: createMockProfile([Group.USER, Group.ADMIN_READONLY]) });
		worker.use(
			http.get('/api/audit-log-exports', ({ request }) => {
				expect(new URL(request.url).searchParams.get('type')).toBe(type);
				return HttpResponse.json({
					items: [
						{
							id: 'job-1',
							name: 'Completed export',
							state: 'completed',
							createdAt: '2026-11-20T12:00:00Z'
						}
					]
				});
			}),
			http.get('/api/scheduled-audit-log-exports', ({ request }) => {
				expect(new URL(request.url).searchParams.get('type')).toBe(type);
				return HttpResponse.json({
					items: [
						{
							id: 'schedule-1',
							name: 'Daily export',
							enabled: true,
							schedule: { interval: 'daily', hour: 3, minute: 0 }
						}
					]
				});
			})
		);

		params.set('view', 'exports');
		params.set('form', 'export');
		const jobs = await render(ExportsPage);
		await expect.element(page.getByText('Completed export', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add Export' })).not.toBeInTheDocument();
		await jobs.unmount();

		params.set('view', 'scheduled');
		await render(ExportsPage);
		await expect.element(page.getByText('Daily export', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('checkbox')).not.toBeInTheDocument();
	});
});
