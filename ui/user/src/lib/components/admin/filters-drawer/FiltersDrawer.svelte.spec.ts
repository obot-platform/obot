import type { AuditLogAPIKeyFilterOption, OrgUser } from '$lib/services';
import { getUserDisplayName } from '$lib/utils';
import FiltersDrawer from './FiltersDrawer.svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, test } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

test('recomputes API key owner labels when users load after filter options', async () => {
	const users = new SvelteMap<string, OrgUser>();
	const options: AuditLogAPIKeyFilterOption[] = [
		{
			value: '11',
			name: 'Active key',
			maskedKey: 'ok1-1-11-*****',
			userID: '1',
			userDisplayName: '',
			revoked: false
		},
		{
			value: '22',
			name: 'Deleted key',
			maskedKey: 'ok1-2-22-*****',
			userID: '2',
			userDisplayName: '',
			revoked: true
		}
	];

	await render(FiltersDrawer, {
		filters: { api_key_id: null },
		onClose: () => undefined,
		getFilterDisplayLabel: () => 'API Key',
		getUserDisplayName: (id: string) => getUserDisplayName(users, id),
		endpoint: async () => ({ options })
	});

	await page.getByCSS('#filter-api_key_id').click();
	await expect
		.element(page.getByText('Active key (ok1-1-11-*****) · Unknown User', { exact: true }))
		.toBeVisible();

	users.set('1', { id: '1', displayName: 'Active Owner' } as OrgUser);
	users.set('2', {
		id: '2',
		originalUsername: 'deleted-owner',
		deletedAt: '2026-08-20T00:00:00Z'
	} as OrgUser);

	await expect
		.element(page.getByText('Active key (ok1-1-11-*****) · Active Owner', { exact: true }))
		.toBeVisible();
	await expect
		.element(
			page.getByText('Deleted key (ok1-2-22-*****) · deleted-owner (Deleted) · Revoked', {
				exact: true
			})
		)
		.toBeVisible();
});
