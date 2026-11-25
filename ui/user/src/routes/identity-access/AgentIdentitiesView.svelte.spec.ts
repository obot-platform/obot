import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { createAgent } from '../../tests/mocks/agentIdentities';
import { listUsersResponse } from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import AgentIdentitiesView from './AgentIdentitiesView.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const active = createAgent({ id: '7', displayName: 'Build Bot' });
const other = createAgent({ id: '8', displayName: 'Deploy Bot', activeAPIKeyCount: 0 });

async function renderView(groups: string[] = [Group.ADMIN]) {
	await preparePageData({ profile: createMockProfile(groups) });
	render(AgentIdentitiesView, { agents: [active, other], users: listUsersResponse });
}

async function openRowActions(name: string) {
	const row = page.getByRole('row').filter({ hasText: name });
	await row.getByRole('button', { name: 'Row actions' }).click();
}

beforeEach(() => {
	worker.use(
		http.post('/api/agents/7/disable', () =>
			HttpResponse.json({ ...active, status: 'disabled', disabledAt: '2026-01-02T00:00:00Z' })
		),
		http.post('/api/agents/7/enable', () => HttpResponse.json(active)),
		http.delete('/api/agents/8', () => new HttpResponse(null, { status: 204 }))
	);
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe('AgentIdentitiesView', () => {
	it('lists agents with their status and active key count', async () => {
		await renderView();

		await expect.element(page.getByText('Build Bot', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Deploy Bot', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Active', { exact: true }).first()).toBeVisible();
	});

	it('disables an agent after a confirmation and shows it as disabled', async () => {
		await renderView();

		await openRowActions('Build Bot');
		await page.getByRole('button', { name: 'Disable', exact: true }).click();
		// The confirmation is in the page, not a window.confirm.
		await expect.element(page.getByText('Disable "Build Bot"?')).toBeVisible();
		await page.getByRole('button', { name: 'Disable', exact: true }).last().click();

		await expect
			.element(page.getByRole('row').filter({ hasText: 'Build Bot' }).getByText('Disabled'))
			.toBeVisible();
	});

	it('deletes an agent after a confirmation and removes it from the list', async () => {
		await renderView();

		await openRowActions('Deploy Bot');
		await page.getByRole('button', { name: 'Delete', exact: true }).click();
		await expect
			.element(page.getByText('Are you sure you want to delete "Deploy Bot"?'))
			.toBeVisible();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await expect.element(page.getByText('Deploy Bot', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('Build Bot', { exact: true })).toBeVisible();
	});

	it.each([
		{ name: 'auditors', groups: [Group.USER, Group.AUDITOR] },
		{ name: 'read-only admins', groups: [Group.USER, Group.ADMIN_READONLY] }
	])('leaves out mutations for $name', async ({ groups }) => {
		await renderView(groups);

		await openRowActions('Build Bot');
		await expect
			.element(page.getByRole('button', { name: 'Delete', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Disable', exact: true }))
			.not.toBeInTheDocument();
	});
});
