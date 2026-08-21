import { ApiKeysService, Group, UserService } from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import { reroute } from '../../hooks';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { listUsersResponse } from '../../tests/mocks/data';
import type { PageData } from './$types';
import { load } from './+page';
import AgentAuthScopesPage from './+page.svelte';
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

function loadAgentAuthScopes(pathname: string) {
	const profile = createMockProfile();
	return load({
		fetch: vi.fn(),
		parent: vi.fn(async () => ({ profile })),
		url: new URL(pathname, 'http://localhost')
	} as unknown as Parameters<typeof load>[0]);
}

async function renderAgentAuthScopesPage({
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
	const data = await preparePageData<PageData>({
		apiKeys: [apiKey],
		users: isAdmin ? listUsersResponse : [],
		isAdmin,
		profile
	});

	return render(AgentAuthScopesPage, { data });
}

async function openRowActions() {
	await page.getByRole('button', { name: 'Row actions' }).click();
}

afterEach(() => {
	vi.restoreAllMocks();
});

describe('Agent Auth Scopes route selection', () => {
	it('reroutes admin list and detail URLs to root route files', () => {
		const rerouteUrl = (pathname: string) =>
			reroute({ url: new URL(pathname, 'http://localhost') } as Parameters<typeof reroute>[0]);

		expect(rerouteUrl('/admin/agent-auth-scopes')).toBe('/agent-auth-scopes');
		expect(rerouteUrl('/admin/agent-auth-scopes/42')).toBe('/agent-auth-scopes/42');
		expect(rerouteUrl('/agent-auth-scopes')).toBeUndefined();
	});

	it('loads current-user scopes for non-admin URL', async () => {
		const listApiKeys = vi.spyOn(ApiKeysService, 'listApiKeys').mockResolvedValue([apiKey]);
		const listAllApiKeys = vi.spyOn(ApiKeysService, 'listAllApiKeys');
		const listUsers = vi.spyOn(UserService, 'listUsers');

		const result = await loadAgentAuthScopes('/agent-auth-scopes');

		expect(result).toMatchObject({ apiKeys: [apiKey], users: [], isAdmin: false });
		expect(listApiKeys).toHaveBeenCalledOnce();
		expect(listAllApiKeys).not.toHaveBeenCalled();
		expect(listUsers).not.toHaveBeenCalled();
	});

	it('loads all scopes and users for admin URL', async () => {
		const listApiKeys = vi.spyOn(ApiKeysService, 'listApiKeys');
		const listAllApiKeys = vi.spyOn(ApiKeysService, 'listAllApiKeys').mockResolvedValue([apiKey]);
		const listUsers = vi.spyOn(UserService, 'listUsers').mockResolvedValue(listUsersResponse);

		const result = await loadAgentAuthScopes('/admin/agent-auth-scopes');

		expect(result).toMatchObject({
			apiKeys: [apiKey],
			users: listUsersResponse,
			isAdmin: true
		});
		expect(listApiKeys).not.toHaveBeenCalled();
		expect(listAllApiKeys).toHaveBeenCalledOnce();
		expect(listUsers).toHaveBeenCalledOnce();
	});
});

describe('Agent Auth Scopes page variants', () => {
	it('shows user table without admin-only creator column', async () => {
		await renderAgentAuthScopesPage({ isAdmin: false });

		await expect.element(page.getByText(apiKey.name, { exact: true })).toBeVisible();
		await expect.element(page.getByText('Created By', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Create Agent Auth Scope', exact: true }))
			.toBeVisible();
	});

	it('shows admin creator column and admin create action', async () => {
		await renderAgentAuthScopesPage({ isAdmin: true });

		await expect.element(page.getByText('Created By', { exact: true }).first()).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Create Agent Auth Scope', exact: true }))
			.toBeVisible();
	});

	it('hides admin mutations for readonly admin URL', async () => {
		await renderAgentAuthScopesPage({ isAdmin: true, readonly: true, profileId: 'auditor-1' });

		await expect
			.element(page.getByRole('button', { name: 'Create Agent Auth Scope', exact: true }))
			.not.toBeInTheDocument();

		await openRowActions();
		await expect.element(page.getByText('View Related Logs', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: apiKeyPrefix, exact: true }))
			.toHaveAttribute('href', `/admin/agent-auth-scopes/${apiKey.id}/${apiKeyPrefix}`);
		await expect
			.element(page.getByRole('button', { name: 'Delete', exact: true }))
			.not.toBeInTheDocument();
	});

	it('shows related log link and delete in the row menu for admins', async () => {
		await renderAgentAuthScopesPage({ isAdmin: true });

		await openRowActions();
		await expect.element(page.getByText('View Related Logs', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: apiKeyPrefix, exact: true }))
			.toHaveAttribute('href', `/admin/agent-auth-scopes/${apiKey.id}/${apiKeyPrefix}`);
		await expect.element(page.getByRole('button', { name: 'Delete', exact: true })).toBeVisible();
	});

	it('shows delete but not related logs in the row menu for non-admin owners', async () => {
		await renderAgentAuthScopesPage({ isAdmin: false, groups: [Group.USER] });

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
