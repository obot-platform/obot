import { AdminService } from '$lib/services';
import CurrentAccessDialog from './CurrentAccessDialog.svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

afterEach(() => {
	vi.restoreAllMocks();
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
		vi.spyOn(AdminService, 'listAccessControlRules').mockRejectedValue(
			new Error('mcp unavailable')
		);

		const result = await render(CurrentAccessDialog);
		result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

		await expect.element(page.getByRole('alert')).toHaveTextContent('mcp unavailable');
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
});
