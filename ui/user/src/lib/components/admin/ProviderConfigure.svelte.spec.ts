import type { AuthProvider } from '$lib/services';
import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import ProviderConfigure from './ProviderConfigure.svelte';
import { describe, expect, it, vi } from 'vitest';

const emailPolicyParameter = 'OBOT_KEYCLOAK_AUTH_PROVIDER_INSECURE_ALLOW_UNVERIFIED_EMAIL';
const values = {
	OBOT_KEYCLOAK_AUTH_PROVIDER_ISSUER_URL: 'https://identity.example.com/realms/employees',
	OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID: 'login-client'
};
const provider: AuthProvider = {
	id: 'keycloak-auth-provider',
	type: 'authprovider',
	name: 'Keycloak',
	created: '2026-10-20T00:00:00Z',
	image: '',
	port: 0,
	configured: true,
	requiredConfigurationParameters: [
		{ name: 'OBOT_KEYCLOAK_AUTH_PROVIDER_ISSUER_URL', friendlyName: 'Realm Issuer URL' },
		{ name: 'OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID', friendlyName: 'Client ID' }
	],
	optionalConfigurationParameters: [
		{ name: emailPolicyParameter, friendlyName: 'Allow Unverified Emails' }
	]
};

async function openWith(saved: string | undefined) {
	const onConfigure = vi.fn(async () => undefined);
	const dialog = await renderOpenDialog(ProviderConfigure, {
		provider,
		values: { ...values, ...(saved === undefined ? {} : { [emailPolicyParameter]: saved }) },
		onConfigure
	});
	return { dialog, onConfigure };
}

describe('ProviderConfigure Keycloak email verification', () => {
	for (const saved of [undefined, 'false', 'true']) {
		it(`loads ${saved ?? 'the default'} as a switch and submits the changed policy`, async () => {
			const { dialog, onConfigure } = await openWith(saved);
			const toggle = dialog.getByRole('switch', { name: 'Allow Unverified Emails' });
			if (saved === 'true') {
				await expect.element(toggle).toBeChecked();
			} else {
				await expect.element(toggle).not.toBeChecked();
			}
			await toggle.click();
			await dialog.getByRole('button', { name: 'Confirm', exact: true }).click();
			await vi.waitFor(() => {
				expect(onConfigure).toHaveBeenCalledExactlyOnceWith({
					...values,
					[emailPolicyParameter]: saved === 'true' ? 'false' : 'true'
				});
			});
		});
	}

	for (const saved of [undefined, 'false']) {
		it(`submits ${saved ?? 'the default'} unchanged when the switch is not touched`, async () => {
			const { dialog, onConfigure } = await openWith(saved);
			await dialog.getByRole('button', { name: 'Confirm', exact: true }).click();
			await vi.waitFor(() => {
				expect(onConfigure).toHaveBeenCalledExactlyOnceWith({
					...values,
					...(saved === undefined ? {} : { [emailPolicyParameter]: saved })
				});
			});
		});
	}

	it('keeps an enabled policy when another setting changes', async () => {
		const { dialog, onConfigure } = await openWith('true');
		const clientID = dialog.getByLabelText('Client ID', { exact: true });
		await clientID.clear();
		await clientID.fill('renamed-client');
		await dialog.getByRole('button', { name: 'Confirm', exact: true }).click();
		await vi.waitFor(() => {
			expect(onConfigure).toHaveBeenCalledExactlyOnceWith({
				...values,
				OBOT_KEYCLOAK_AUTH_PROVIDER_CLIENT_ID: 'renamed-client',
				[emailPolicyParameter]: 'true'
			});
		});
	});
});
