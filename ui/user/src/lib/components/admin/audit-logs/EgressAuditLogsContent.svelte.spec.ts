import { Group, UserService } from '$lib/services';
import { profile } from '$lib/stores';
import { formatTimeAgo } from '$lib/time';
import { createMockProfile } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import EgressAuditLogsContent from './EgressAuditLogsContent.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

for (const auditor of [false, true]) {
	it(`shows egress attribution and ${auditor ? 'reveals' : 'hides'} payloads for ${auditor ? 'auditors' : 'admins'}`, async () => {
		profile.initialize(createMockProfile(auditor ? [Group.ADMIN, Group.AUDITOR] : [Group.ADMIN]));
		vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
		const entry = {
			id: 'egress-one',
			createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
			userID: '7',
			claudeAccount: 'user_claude',
			environmentID: 'env',
			claudeSession: 'session_test',
			method: 'GET',
			host: 'example.com',
			path: '/test',
			responseStatus: 200,
			outcome: 'complete',
			requestBytes: 0,
			responseBytes: 8,
			durationMs: 42
		};
		worker.use(
			http.get('*/api/egress-audit-logs', () => HttpResponse.json({ items: [entry], total: 1 })),
			http.get('*/api/egress-audit-logs/egress-one', () =>
				HttpResponse.json({ ...entry, responseBody: btoa('private response') })
			),
			http.get('*/api/claude-environments', () =>
				HttpResponse.json({ environments: [{ id: 'env', displayName: 'Development' }] })
			)
		);
		render(EgressAuditLogsContent);
		await expect.element(page.getByText('user_claude', { exact: true })).toBeVisible();
		await expect.element(page.getByText('session_test', { exact: true })).toBeVisible();
		const time = formatTimeAgo(entry.createdAt);
		await expect
			.element(page.getByRole('cell', { name: time.relativeTime, exact: true }))
			.toHaveAttribute('title', time.fullDate);

		await page.getByRole('button', { name: 'Details', exact: true }).click();
		await expect
			.poll(() =>
				document.querySelector('#egress-audit-logs-details-sidebar')?.matches(':popover-open')
			)
			.toBe(true);
		if (auditor) {
			await expect.element(page.getByText('private response', { exact: false })).toBeVisible();
		} else {
			await expect
				.element(page.getByText('private response', { exact: false }))
				.not.toBeInTheDocument();
		}
	});
}
