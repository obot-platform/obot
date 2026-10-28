import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
import type { AuthProvider, LocalAuthUser } from '$lib/services';
import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import { worker } from '../../../tests/mocks/worker';
import LocalAuthConfigure from './LocalAuthConfigure.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';

const localProvider: AuthProvider = {
	id: 'local-auth-provider',
	created: '2026-08-04T16:58:40-04:00',
	type: 'authprovider',
	name: 'Local',
	icon: '/admin/assets/local_icon_small.png',
	image: '',
	port: 0,
	configured: true,
	missingConfigurationParameters: []
};

const validPassword = 'a'.repeat(LOCAL_AUTH_MIN_PASSWORD_LENGTH);

function createdUser(email: string): LocalAuthUser {
	return { id: 'user-1', email, created: '2026-08-18T12:00:00Z', requirePasswordChange: false };
}

function mockLocalUsers({
	existing = [] as LocalAuthUser[],
	createUser = vi.fn()
}: {
	existing?: LocalAuthUser[];
	createUser?: (body: { email: string; password: string; requirePasswordChange: boolean }) => void;
} = {}) {
	let users = [...existing];

	worker.use(
		http.get('/api/local-auth/users', () => HttpResponse.json({ items: users })),
		http.post('/api/local-auth/users', async ({ request }) => {
			const body = (await request.json()) as {
				email: string;
				password: string;
				requirePasswordChange: boolean;
			};
			createUser(body);
			const user = createdUser(body.email);
			users = [...users, user];
			return HttpResponse.json(user);
		})
	);

	return { createUser };
}

async function renderConfiguredDialog(props: { readonly?: boolean } = {}) {
	const dialog = await renderOpenDialog(LocalAuthConfigure, {
		provider: localProvider,
		values: { OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*' },
		onConfigure: vi.fn(async () => undefined),
		...props
	});

	await expect.element(dialog.getByText('Set Up Local', { exact: true })).toBeVisible();
	return dialog;
}

describe('LocalAuthConfigure.svelte', () => {
	it('creates a verification account after staging Local during a provider switch', async () => {
		const { createUser } = mockLocalUsers();
		const onConfigure = vi.fn(async () => undefined);
		const onClose = vi.fn();
		const dialog = await renderOpenDialog(LocalAuthConfigure, {
			provider: { ...localProvider, configured: false },
			switchingProvider: true,
			onConfigure,
			onClose
		});

		await dialog.getByRole('button', { name: 'Continue', exact: true }).click();
		await expect.element(dialog.getByLabelText('Email', { exact: true })).toBeVisible();
		expect(onConfigure).toHaveBeenCalledWith({ OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*' });
		await dialog.getByLabelText('Email', { exact: true }).fill('owner@example.com');
		await page.getByCSS('#local-user-password-draft').fill(validPassword);
		await expect
			.element(dialog.getByRole('checkbox', { name: /Require the user to change this password/ }))
			.not.toBeChecked();
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => {
			expect(createUser).toHaveBeenCalledWith({
				email: 'owner@example.com',
				password: validPassword,
				requirePasswordChange: false
			});
			expect(onClose).toHaveBeenCalledWith(1);
		});
	});

	it('opens user management without a way back to configuration when bootstrapping', async () => {
		mockLocalUsers({ existing: [createdUser('owner@example.com')] });
		const dialog = await renderOpenDialog(LocalAuthConfigure, {
			provider: localProvider,
			bootstrap: true,
			onConfigure: vi.fn(async () => undefined)
		});

		await expect.element(dialog.getByText('owner@example.com', { exact: true })).toBeVisible();
		await expect.element(dialog.getByRole('heading', { name: 'Users', exact: true })).toBeVisible();
		await expect
			.element(dialog.getByRole('button', { name: 'Configuration', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(dialog.getByText('Allowed Email Domains', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('opens user management for a staged Local provider only during a switch', async () => {
		mockLocalUsers({ existing: [createdUser('owner@example.com')] });
		const dialog = await renderOpenDialog(LocalAuthConfigure, {
			provider: { ...localProvider, configured: false, staged: true },
			switchingProvider: true,
			onConfigure: vi.fn(async () => undefined)
		});

		await expect.element(dialog.getByText('owner@example.com', { exact: true })).toBeVisible();
		await expect
			.element(dialog.getByRole('button', { name: 'Add New User', exact: true }))
			.toBeVisible();
	});

	it('shows only the domain configuration for a provider that is already configured', async () => {
		const dialog = await renderConfiguredDialog();

		await expect.element(dialog.getByText('Allowed Email Domains', { exact: true })).toBeVisible();
		await expect
			.element(dialog.getByRole('button', { name: 'Continue', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(dialog.getByRole('button', { name: 'Save', exact: true })).toBeVisible();
		await expect.element(dialog.getByText('New user', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(dialog.getByRole('button', { name: 'Add New User', exact: true }))
			.not.toBeInTheDocument();
	});

	it('saves domain changes and closes the dialog', async () => {
		const onConfigure = vi.fn(async () => undefined);
		const dialog = await renderOpenDialog(LocalAuthConfigure, {
			provider: localProvider,
			values: { OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: 'example.com' },
			onConfigure
		});
		const dialogElement = dialog.element();

		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => {
			expect(onConfigure).toHaveBeenCalledWith({
				OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: 'example.com'
			});
		});
		await expect.element(dialogElement).not.toHaveAttribute('open');
	});

	it('hides actions for a readonly admin', async () => {
		const dialog = await renderConfiguredDialog({ readonly: true });

		await expect.element(dialog.getByText('Allowed Email Domains', { exact: true })).toBeVisible();
		await expect
			.element(dialog.getByRole('button', { name: 'Continue', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(dialog.getByRole('button', { name: 'Save', exact: true }))
			.not.toBeInTheDocument();
	});

	it('configures an unconfigured provider and closes the dialog', async () => {
		const onConfigure = vi.fn(async () => undefined);
		const dialog = await renderOpenDialog(LocalAuthConfigure, {
			provider: { ...localProvider, configured: false },
			onConfigure
		});
		const dialogElement = dialog.element();

		await expect
			.element(dialog.getByRole('button', { name: 'Continue', exact: true }))
			.toBeVisible();
		await dialog.getByRole('button', { name: 'Continue', exact: true }).click();

		await vi.waitFor(() => {
			expect(onConfigure).toHaveBeenCalledWith({
				OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*'
			});
		});
		await expect.element(dialogElement).not.toHaveAttribute('open');
	});

	describe('required onboarding', () => {
		async function renderRequiredDialog() {
			const onConfigure = vi.fn(async () => undefined);
			const onClose = vi.fn();
			const { createUser } = mockLocalUsers();
			const dialog = await renderOpenDialog(LocalAuthConfigure, {
				provider: { ...localProvider, configured: false },
				required: true,
				onConfigure,
				onClose
			});

			await expect.element(dialog.getByText('Set Up Owner Account', { exact: true })).toBeVisible();
			return { dialog, onConfigure, onClose, createUser };
		}

		it('auto-configures without collecting domains and asks for a single initial user', async () => {
			const { dialog, onConfigure } = await renderRequiredDialog();

			await vi.waitFor(() => {
				expect(onConfigure).toHaveBeenCalledWith({
					OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*'
				});
			});

			await expect.element(dialog.getByLabelText('Email', { exact: true })).toBeVisible();
			await expect.element(page.getByCSS('#initial-user-password')).toBeVisible();
			await expect.element(page.getByCSS('#initial-user-password-confirm')).toBeVisible();
			await expect
				.element(
					dialog.getByText('Set up your initial owner account to get started!', { exact: true })
				)
				.toBeVisible();
			await expect
				.element(dialog.getByText('Allowed Email Domains', { exact: true }))
				.not.toBeInTheDocument();
			await expect.element(dialog.getByText('Users', { exact: true })).not.toBeInTheDocument();
			await expect
				.element(dialog.getByRole('checkbox', { name: /Require the user to change this password/ }))
				.not.toBeInTheDocument();
		});

		it('rejects a mismatched confirmation without creating the user', async () => {
			const { dialog, createUser, onClose } = await renderRequiredDialog();

			await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
			await page.getByCSS('#initial-user-password').fill(validPassword);
			await page.getByCSS('#initial-user-password-confirm').fill(`${validPassword}x`);
			await dialog.getByRole('button', { name: 'Continue', exact: true }).click();

			await expect
				.element(dialog.getByText('The passwords do not match.', { exact: true }))
				.toBeVisible();
			expect(createUser).not.toHaveBeenCalled();
			expect(onClose).not.toHaveBeenCalled();
		});

		it('creates the initial user on save and continues the original close flow', async () => {
			const { dialog, createUser, onClose } = await renderRequiredDialog();

			await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
			await page.getByCSS('#initial-user-password').fill(validPassword);
			await page.getByCSS('#initial-user-password-confirm').fill(validPassword);
			await dialog.getByRole('button', { name: 'Continue', exact: true }).click();

			await vi.waitFor(() => {
				expect(createUser).toHaveBeenCalledWith({
					email: 'ada@example.com',
					password: validPassword,
					requirePasswordChange: false
				});
			});

			await vi.waitFor(() => {
				expect(onClose).toHaveBeenCalledWith(1);
			});
		});
	});
});
