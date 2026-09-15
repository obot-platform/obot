import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
import type { AuthProvider, LocalAuthUser } from '$lib/services';
import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import { worker } from '../../../tests/mocks/worker';
import LocalAuthConfigure from './LocalAuthConfigure.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';

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

async function renderConfiguredDialog(
	props: { readonly?: boolean; switching?: boolean; bootstrap?: boolean } = {}
) {
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
	it('limits bootstrap to one pending account and defaults to a ready-to-use password', async () => {
		const { createUser } = mockLocalUsers();
		const dialog = await renderConfiguredDialog({ bootstrap: true });

		await expect.element(dialog.getByText(/become Owner automatically/)).toBeVisible();
		await expect
			.element(dialog.getByRole('checkbox', { name: /Require the user to change/ }))
			.not.toBeChecked();

		await dialog.getByLabelText('Email', { exact: true }).fill('owner@example.com');
		await page.getByCSS('#local-user-password-draft').fill(validPassword);
		await userEvent.keyboard('{Enter}');

		await expect.element(dialog.getByText('owner@example.com', { exact: true })).toBeVisible();
		await expect
			.element(dialog.getByRole('button', { name: 'Add New User', exact: true }))
			.not.toBeInTheDocument();

		await dialog.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => {
			expect(createUser).toHaveBeenCalledExactlyOnceWith({
				email: 'owner@example.com',
				password: validPassword,
				requirePasswordChange: false
			});
		});
	});

	it('lets bootstrap require a password change for its initial account', async () => {
		const { createUser } = mockLocalUsers();
		const dialog = await renderConfiguredDialog({ bootstrap: true });

		await dialog.getByLabelText('Email', { exact: true }).fill('owner@example.com');
		await page.getByCSS('#local-user-password-draft').fill(validPassword);
		await dialog.getByRole('checkbox', { name: /Require the user to change/ }).click();
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => {
			expect(createUser).toHaveBeenCalledWith({
				email: 'owner@example.com',
				password: validPassword,
				requirePasswordChange: true
			});
		});
	});

	it('allows bootstrap to reset or replace its existing account but not add another', async () => {
		mockLocalUsers({ existing: [createdUser('owner@example.com')] });
		const dialog = await renderConfiguredDialog({ bootstrap: true });

		await expect.element(dialog.getByText('owner@example.com', { exact: true })).toBeVisible();
		await expect.element(dialog.getByRole('button', { name: 'Reset password' })).toBeVisible();
		await expect
			.element(dialog.getByRole('button', { name: 'Add New User', exact: true }))
			.not.toBeInTheDocument();

		await dialog.getByRole('button', { name: 'Delete user' }).click();
		await dialog.getByRole('button', { name: 'Add New User', exact: true }).click();
		await expect.element(dialog.getByLabelText('Email', { exact: true })).toBeVisible();

		await dialog.getByRole('button', { name: 'Undo delete' }).click();
		await expect
			.element(dialog.getByText('Remove the new account before restoring the existing account.'))
			.toBeVisible();
	});

	it('opens the new user draft on its own when no local users exist', async () => {
		mockLocalUsers();
		const dialog = await renderConfiguredDialog();

		await expect.element(dialog.getByText('New user', { exact: true })).toBeVisible();
		await expect.element(dialog.getByLabelText('Email', { exact: true })).toBeVisible();
	});

	it('leaves the new user draft closed when local users already exist', async () => {
		mockLocalUsers({ existing: [createdUser('ada@example.com')] });
		const dialog = await renderConfiguredDialog();

		await expect.element(dialog.getByText('ada@example.com', { exact: true })).toBeVisible();
		await expect.element(dialog.getByLabelText('Email', { exact: true })).not.toBeInTheDocument();

		await dialog.getByRole('button', { name: 'Add New User', exact: true }).click();
		await expect.element(dialog.getByLabelText('Email', { exact: true })).toBeVisible();
	});

	it('leaves the new user draft closed for a readonly admin with no local users', async () => {
		mockLocalUsers();
		const dialog = await renderConfiguredDialog({ readonly: true });

		await expect.element(dialog.getByText('No local users yet.', { exact: false })).toBeVisible();
		await expect.element(dialog.getByLabelText('Email', { exact: true })).not.toBeInTheDocument();
	});

	it('applies a valid new user when Enter is pressed in the draft fields', async () => {
		const { createUser } = mockLocalUsers();
		const dialog = await renderConfiguredDialog();

		await dialog.getByRole('button', { name: 'Add New User', exact: true }).click();
		await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
		await page.getByCSS('#local-user-password-draft').fill(validPassword);
		await userEvent.keyboard('{Enter}');

		await expect.element(dialog.getByText('ada@example.com', { exact: true })).toBeVisible();
		await expect.element(dialog.getByLabelText('Email', { exact: true })).not.toBeInTheDocument();
		expect(createUser).not.toHaveBeenCalled();
		await expect
			.element(
				dialog.getByText('Fill out the required email and password fields.', { exact: true })
			)
			.not.toBeInTheDocument();
	});

	it('saves a valid in-progress user when the Save button is clicked', async () => {
		const { createUser } = mockLocalUsers();
		const dialog = await renderConfiguredDialog();

		await dialog.getByRole('button', { name: 'Add New User', exact: true }).click();
		await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
		await page.getByCSS('#local-user-password-draft').fill(validPassword);
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => {
			expect(createUser).toHaveBeenCalledWith({
				email: 'ada@example.com',
				password: validPassword,
				requirePasswordChange: true
			});
		});
	});

	// During a switch the account created here is the one the administrator immediately signs in as
	// to prove the replacement works. A forced password change would put that sign-in behind the
	// restricted-session wall, so the switch could never complete on the first pass.
	it('does not force a password change on the owner created for a switch', async () => {
		const { createUser } = mockLocalUsers();
		const dialog = await renderConfiguredDialog({ switching: true });

		await dialog.getByRole('button', { name: 'Add New User', exact: true }).click();
		await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
		await page.getByCSS('#local-user-password-draft').fill(validPassword);
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => {
			expect(createUser).toHaveBeenCalledWith({
				email: 'ada@example.com',
				password: validPassword,
				requirePasswordChange: false
			});
		});
	});

	it('lets the administrator turn the password-change requirement off', async () => {
		const { createUser } = mockLocalUsers();
		const dialog = await renderConfiguredDialog();

		await dialog.getByRole('button', { name: 'Add New User', exact: true }).click();
		await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
		await page.getByCSS('#local-user-password-draft').fill(validPassword);
		await dialog
			.getByRole('checkbox', { name: /Require the user to change this password/ })
			.click();
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => {
			expect(createUser).toHaveBeenCalledWith({
				email: 'ada@example.com',
				password: validPassword,
				requirePasswordChange: false
			});
		});
	});

	it('warns that local users will not transfer to another auth provider', async () => {
		mockLocalUsers();
		const dialog = await renderConfiguredDialog();

		// Switching providers can mint a new Obot user, stranding whatever these users set up, so
		// the local user management page has to say so.
		await expect
			.element(
				dialog.getByText(
					/can create a new Obot user, and these users and their work will not\s+transfer/
				)
			)
			.toBeVisible();
	});

	it('marks only the users who still owe a password change', async () => {
		mockLocalUsers({
			existing: [
				{ ...createdUser('pending@example.com'), id: 'user-1', requirePasswordChange: true },
				{ ...createdUser('settled@example.com'), id: 'user-2', requirePasswordChange: false }
			]
		});
		const dialog = await renderConfiguredDialog();

		const pending = dialog.getByText('pending@example.com', { exact: true }).locator('..');
		const settled = dialog.getByText('settled@example.com', { exact: true }).locator('..');

		await expect
			.element(pending.getByText('Password change required', { exact: true }))
			.toBeVisible();
		await expect
			.element(settled.getByText('Password change required', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('keeps the draft open when Enter is pressed with incomplete fields', async () => {
		const { createUser } = mockLocalUsers();
		const dialog = await renderConfiguredDialog();

		await dialog.getByRole('button', { name: 'Add New User', exact: true }).click();
		await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
		await page.getByCSS('#local-user-password-draft').click();
		await userEvent.keyboard('{Enter}');

		await expect.element(dialog.getByLabelText('Email', { exact: true })).toBeVisible();
		expect(createUser).not.toHaveBeenCalled();
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
				.element(dialog.getByLabelText('Allowed Email Domains', { exact: true }))
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
