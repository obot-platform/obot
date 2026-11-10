import { CommonAuthProviderIds, LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
import type { AuthProvider } from '$lib/services/admin/types';
import LocalAuthInitialUserForm from './LocalAuthInitialUserForm.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const provider: AuthProvider = {
	id: CommonAuthProviderIds.LOCAL,
	created: '2026-08-04T16:58:40-04:00',
	type: 'authprovider',
	name: 'Local',
	icon: '/admin/assets/local_icon_small.png',
	image: '',
	port: 0,
	configured: true,
	missingConfigurationParameters: [],
	missingEntitlements: [],
	namespace: 'default'
};

const validPassword = 'a'.repeat(LOCAL_AUTH_MIN_PASSWORD_LENGTH);

async function submitForm() {
	await page.getByLabelText('Email', { exact: true }).fill('ada@example.com');
	await page.getByCSS('#initial-user-password').fill(validPassword);
	await page.getByCSS('#initial-user-password-confirm').fill(validPassword);
	await page.getByRole('button', { name: 'Continue', exact: true }).click();
}

describe('LocalAuthInitialUserForm', () => {
	it('stays disabled when a custom createUser takes over', async () => {
		const createUser = vi.fn(async () => true);
		render(LocalAuthInitialUserForm, { provider, onConfigure: vi.fn(), createUser });

		await submitForm();

		await vi.waitFor(() =>
			expect(createUser).toHaveBeenCalledWith('ada@example.com', validPassword)
		);
		await expect.element(page.getByLabelText('Email', { exact: true })).toBeDisabled();
	});

	it('re-enables the form when a custom createUser does not take over', async () => {
		const createUser = vi.fn(async () => false);
		render(LocalAuthInitialUserForm, { provider, onConfigure: vi.fn(), createUser });

		await submitForm();

		await vi.waitFor(() => expect(createUser).toHaveBeenCalledOnce());
		await expect.element(page.getByLabelText('Email', { exact: true })).toBeEnabled();
		await expect.element(page.getByRole('button', { name: 'Continue', exact: true })).toBeEnabled();
	});
});
