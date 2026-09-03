import { Group } from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { listUsersResponse } from '../../tests/mocks/data';
import AgentsView from './AgentsView.svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const apiKey: APIKey = {
	id: 42,
	userId: Number(listUsersResponse[0].id),
	name: 'Test Agent Scope',
	description: 'Test scope',
	canAccessAPI: false,
	canAccessLLMProxy: true,
	canAccessSkills: false,
	canAccessDeviceScans: false,
	createdAt: '2026-01-01T00:00:00.000Z'
};

const apiKeyPrefix = `ok1-${apiKey.userId}-${apiKey.id}-*****`;

async function renderAgentsView({
	isAdmin,
	readonly = false,
	profileId,
	groups
}: {
	isAdmin: boolean;
	readonly?: boolean;
	profileId?: string;
	groups?: string[];
}) {
	const profile = createMockProfile(groups ?? (readonly ? [Group.AUDITOR] : [Group.ADMIN]));
	if (profileId) profile.id = profileId;
	await preparePageData({ profile });

	return render(AgentsView, {
		apiKeys: [apiKey],
		users: isAdmin ? listUsersResponse : [],
		isAdmin
	});
}

async function openRowActions() {
	const row = page.getByRole('row').filter({ hasText: apiKey.name });
	await row.getByRole('button', { name: 'Row actions' }).click();
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('AgentsView', () => {
	it('shows user table without admin-only creator column', async () => {
		await renderAgentsView({ isAdmin: false });

		await expect.element(page.getByText(apiKey.name, { exact: true })).toBeVisible();
		await expect.element(page.getByText('Created By', { exact: true })).not.toBeInTheDocument();
	});

	it('shows admin creator column', async () => {
		await renderAgentsView({ isAdmin: true });

		await expect.element(page.getByText('Created By', { exact: true }).first()).toBeVisible();
	});

	it('hides mutations for readonly admin', async () => {
		await renderAgentsView({ isAdmin: true, readonly: true, profileId: 'auditor-1' });

		await openRowActions();
		await expect.element(page.getByText('View Related Logs', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: apiKeyPrefix, exact: true }))
			.toHaveAttribute(
				'href',
				`/identity-access/agents/${apiKey.id}/${encodeURIComponent(apiKeyPrefix)}`
			);
		await expect
			.element(page.getByRole('button', { name: 'Delete', exact: true }))
			.not.toBeInTheDocument();
	});

	it('shows related log link and delete in the row menu for admins', async () => {
		await renderAgentsView({ isAdmin: true });

		await openRowActions();
		await expect.element(page.getByText('View Related Logs', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: apiKeyPrefix, exact: true }))
			.toHaveAttribute(
				'href',
				`/identity-access/agents/${apiKey.id}/${encodeURIComponent(apiKeyPrefix)}`
			);
		await expect.element(page.getByRole('button', { name: 'Delete', exact: true })).toBeVisible();
	});

	it('shows delete but not related logs in the row menu for non-admin owners', async () => {
		await renderAgentsView({ isAdmin: false, groups: [Group.USER] });

		await openRowActions();
		await expect
			.element(page.getByText('View Related Logs', { exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: apiKeyPrefix, exact: true }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Delete', exact: true })).toBeVisible();
	});
});
