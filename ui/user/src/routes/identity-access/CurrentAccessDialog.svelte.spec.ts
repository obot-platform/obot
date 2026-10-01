import { AdminService, type Model, type ModelAccessPolicy } from '$lib/services';
import { version } from '$lib/stores';
import { getVersionResponse } from '../../tests/mocks/data';
import CurrentAccessDialog from './CurrentAccessDialog.svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

afterEach(() => {
	vi.restoreAllMocks();
	version.initialize();
});

function mockAccessPolicyLists() {
	vi.spyOn(AdminService, 'listAccessControlRules').mockResolvedValue([]);
	vi.spyOn(AdminService, 'listAllUserWorkspaceAccessControlRules').mockResolvedValue([]);
	vi.spyOn(AdminService, 'listModelAccessPolicies').mockResolvedValue([]);
	vi.spyOn(AdminService, 'listSkillAccessPolicies').mockResolvedValue([]);
	vi.spyOn(AdminService, 'listHostedAgentAccessPolicies').mockResolvedValue([]);
	vi.spyOn(AdminService, 'listAllVMCPs').mockResolvedValue([]);
}

describe('CurrentAccessDialog.svelte', () => {
	it('shows an error when the current view fails to load', async () => {
		mockAccessPolicyLists();
		vi.spyOn(AdminService, 'listAllVMCPs').mockRejectedValue(new Error('vmcps unavailable'));

		const result = await render(CurrentAccessDialog);
		result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

		await expect.element(page.getByRole('alert')).toHaveTextContent('vmcps unavailable');
		await expect
			.element(page.getByText('No policies currently apply to this user.'))
			.not.toBeInTheDocument();
	});

	it('shows the empty state when no policies apply to the current view', async () => {
		mockAccessPolicyLists();

		const result = await render(CurrentAccessDialog);
		result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

		await expect.element(page.getByRole('button', { name: 'MCP Servers' })).toBeVisible();
		await expect.element(page.getByText('No policies currently apply to this user.')).toBeVisible();
	});

	describe('when Hosted Agents are disabled', () => {
		it('hides the Hosted Agents tab', async () => {
			version.initialize({
				...getVersionResponse,
				hostedAgentsEnabled: false
			});
			mockAccessPolicyLists();

			const result = await render(CurrentAccessDialog);
			result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

			await expect.element(page.getByRole('button', { name: 'MCP Servers' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hosted Agents' }))
				.not.toBeInTheDocument();
		});
	});

	describe('when Hosted Agents are enabled', () => {
		it('shows the Hosted Agents tab', async () => {
			version.initialize({
				...getVersionResponse,
				hostedAgentsEnabled: true
			});
			mockAccessPolicyLists();

			const result = await render(CurrentAccessDialog);
			result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

			await expect.element(page.getByRole('button', { name: 'Hosted Agents' })).toBeVisible();
		});
	});

	it("lists resources granted by the user's Obot groups and auth provider groups", async () => {
		mockAccessPolicyLists();
		vi.spyOn(AdminService, 'listModelAccessPolicies').mockResolvedValue([
			modelPolicy(
				'model-obot',
				'Admin Models',
				[{ type: 'obotGroup', id: 'admin' }],
				['model-admin']
			),
			modelPolicy(
				'model-idp',
				'Engineering Models',
				[{ type: 'group', id: 'engineering' }],
				['model-eng']
			),
			modelPolicy('model-other', 'Sales Models', [{ type: 'group', id: 'sales' }], ['model-sales'])
		]);
		vi.spyOn(AdminService, 'listModels').mockResolvedValue([
			model('model-admin', 'Admin Model'),
			model('model-eng', 'Engineering Model'),
			model('model-sales', 'Sales Model')
		]);

		const result = await render(CurrentAccessDialog);
		result.component.open({
			kind: 'user',
			id: 'user-1',
			name: 'Ada',
			obotGroups: ['admin'],
			authProviderGroups: ['engineering']
		});

		await page.getByRole('button', { name: 'Models' }).click();

		await expect.element(page.getByText('Admin Model')).toBeVisible();
		await expect.element(page.getByText('Engineering Model')).toBeVisible();
		await expect.element(page.getByText('Sales Model')).not.toBeInTheDocument();
	});

	it('lists each resource covered by an everything grant', async () => {
		mockAccessPolicyLists();
		vi.spyOn(AdminService, 'listModelAccessPolicies').mockResolvedValue([
			modelPolicy('model-all', 'All Models', [{ type: 'obotGroup', id: 'admin' }], ['*'])
		]);
		vi.spyOn(AdminService, 'listModels').mockResolvedValue([
			model('model-1', 'Specific Model'),
			model('model-2', 'Another Model')
		]);

		const result = await render(CurrentAccessDialog);
		result.component.open({
			kind: 'user',
			id: 'user-1',
			name: 'Ada',
			obotGroups: ['admin'],
			authProviderGroups: ['engineering']
		});

		await page.getByRole('button', { name: 'Models' }).click();

		await expect.element(page.getByText('Specific Model')).toBeVisible();
		await expect.element(page.getByText('Another Model')).toBeVisible();
		await expect.element(page.getByRole('link', { name: 'All Models' }).first()).toBeVisible();
		await expect.element(page.getByText('Everything')).not.toBeInTheDocument();
	});

	it('shows the wildcard rule when the catalog is empty', async () => {
		mockAccessPolicyLists();
		vi.spyOn(AdminService, 'listModelAccessPolicies').mockResolvedValue([
			modelPolicy('model-all', 'All Models', [{ type: 'selector', id: '*' }], ['*'])
		]);
		vi.spyOn(AdminService, 'listModels').mockResolvedValue([]);

		const result = await render(CurrentAccessDialog);
		result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

		await page.getByRole('button', { name: 'Models' }).click();

		await expect.element(page.getByText('All models')).toBeVisible();
		await expect.element(page.getByRole('link', { name: 'All Models' })).toBeVisible();
		await expect
			.element(page.getByText(/do not grant access to any models/))
			.not.toBeInTheDocument();
	});

	it('shows the wildcard rule when the catalog fails to load', async () => {
		mockAccessPolicyLists();
		vi.spyOn(AdminService, 'listModelAccessPolicies').mockResolvedValue([
			modelPolicy('model-all', 'All Models', [{ type: 'selector', id: '*' }], ['*'])
		]);
		vi.spyOn(AdminService, 'listModels').mockRejectedValue(new Error('models unavailable'));

		const result = await render(CurrentAccessDialog);
		result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

		await page.getByRole('button', { name: 'Models' }).click();

		await expect.element(page.getByText('All models')).toBeVisible();
		await expect.element(page.getByRole('link', { name: 'All Models' })).toBeVisible();
		await expect.element(page.getByText('The catalog could not be loaded')).not.toBeInTheDocument();
	});
});

function modelPolicy(
	id: string,
	displayName: string,
	subjects: ModelAccessPolicy['subjects'],
	modelIds: string[]
): ModelAccessPolicy {
	return {
		id,
		displayName,
		created: '2026-01-01T00:00:00Z',
		subjects,
		models: modelIds.map((modelId) => ({ id: modelId }))
	};
}

function model(id: string, displayName: string): Model {
	return {
		id,
		active: true,
		aliasAssigned: false,
		created: 0,
		modelProvider: 'provider',
		modelProviderName: 'Provider',
		name: id,
		displayName,
		targetModel: id,
		usage: 'llm'
	};
}
