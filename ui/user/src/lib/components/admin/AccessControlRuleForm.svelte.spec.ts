import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import AccessControlRuleForm from './AccessControlRuleForm.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

beforeEach(async () => {
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
});

describe('AccessControlRuleForm', () => {
	// Access control rules reject agent subjects, so their picker is the plain one.
	it('never offers agents, and offers the wildcard as All Obot Users', async () => {
		worker.use(
			http.get('*/api/agents', () =>
				HttpResponse.json({
					items: [{ id: '5', displayName: 'Build bot', status: 'active', activeAPIKeyCount: 1 }]
				})
			),
			http.get('*/api/users', () =>
				HttpResponse.json({ items: [{ id: '7', displayName: 'Alice', status: 'active' }] })
			)
		);
		render(AccessControlRuleForm, {
			mcpEntriesContextFn: () => ({ entries: [], servers: [], loading: false })
		});

		await page.getByRole('button', { name: 'Add User/Group' }).click();
		await expect.element(page.getByText('Alice')).toBeVisible();

		await expect.element(page.getByText('Build bot')).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /All Agents/ })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /All Users/ })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /All Obot Users/ })).toBeVisible();
		await expect.element(page.getByRole('button', { name: /and Agents/ })).not.toBeInTheDocument();
	});
});
