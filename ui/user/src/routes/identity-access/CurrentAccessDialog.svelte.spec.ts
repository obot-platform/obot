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
}

describe('CurrentAccessDialog.svelte', () => {
	it('shows an error when access policies fail to load', async () => {
		mockAccessPolicyLists();
		vi.spyOn(AdminService, 'listModelAccessPolicies').mockRejectedValue(
			new Error('models unavailable')
		);

		const result = await render(CurrentAccessDialog);
		result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

		await expect.element(page.getByRole('alert')).toHaveTextContent('models unavailable');
		await expect
			.element(page.getByText('No access policies currently apply to this user.'))
			.not.toBeInTheDocument();
	});

	it('shows the empty state when no policies apply', async () => {
		mockAccessPolicyLists();

		const result = await render(CurrentAccessDialog);
		result.component.open({ kind: 'user', id: 'user-1', name: 'Ada' });

		await expect
			.element(page.getByText('No access policies currently apply to this user.'))
			.toBeVisible();
	});
});
