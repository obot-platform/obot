import { page as appPage } from '$app/state';
import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import AuditLogsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function renderAuditLogsPage(groups: string[] = [Group.ADMIN], view = 'llm') {
	appPage.url.searchParams.set('view', view);
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
});

describe('Audit logs page Claude Environments', () => {
	it.each([
		{ engine: 'docker' as const, enabled: false, visible: false },
		{ engine: 'kubernetes' as const, enabled: false, visible: false },
		{ engine: 'kubernetes' as const, enabled: true, visible: true }
	])(
		'shows Claude audit logs for $engine with enabled=$enabled',
		async ({ engine, enabled, visible }) => {
			appPage.url.searchParams.delete('view');
			await preparePageData({ version: { engine, claudeEnvironmentsEnabled: enabled } });
			await render(AuditLogsPage);
			const tab = page.getByRole('button', { name: 'Environments', exact: true });
			if (visible) await expect.element(tab).toBeVisible();
			else await expect.element(tab).not.toBeInTheDocument();
		}
	);
});
