import type { AuthProvider } from '$lib/services';
import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import ProviderDeconfigureConfirm from './ProviderDeconfigureConfirm.svelte';
import { describe, expect, it } from 'vitest';

const oktaProvider: AuthProvider = {
	id: 'okta-auth-provider',
	created: '2026-08-04T16:58:40-04:00',
	type: 'authprovider',
	name: 'Okta',
	icon: '/admin/assets/okta_icon_small.png',
	image: '',
	port: 0,
	configured: true,
	missingConfigurationParameters: []
};

async function renderDialog(provider: AuthProvider) {
	return renderOpenDialog(ProviderDeconfigureConfirm, {
		providers: [provider],
		onConfirm: () => {},
		onCancel: () => {}
	});
}

describe('ProviderDeconfigureConfirm', () => {
	it('says what deconfiguring deletes when the provider provisions through SCIM', async () => {
		const dialog = await renderDialog({ ...oktaProvider, scimState: 'enforced' });

		await expect
			.element(
				dialog.getByText(
					/Its\s+SCIM\s+connection\s+is\s+deleted,\s+with\s+its\s+groups,\s+group\s+memberships,\s+and\s+group\s+role\s+assignments,\s+and\s+its\s+groups\s+are\s+removed\s+from\s+access\s+policies\./
				)
			)
			.toBeVisible();
		await expect
			.element(
				dialog.getByText(
					/Users\s+that\s+SCIM\s+disabled\s+stay\s+disabled\s+until\s+an\s+administrator\s+enables\s+them\./
				)
			)
			.toBeVisible();
		await expect.element(dialog.getByText(/starts\s+over\s+with\s+a\s+new\s+token/)).toBeVisible();
	});

	it('says nothing about SCIM when the provider does not provision through it', async () => {
		const dialog = await renderDialog(oktaProvider);

		await expect.element(dialog.getByText(/Deconfiguring/)).toBeVisible();
		await expect.element(dialog.getByText(/SCIM connection/)).not.toBeInTheDocument();
	});
});
