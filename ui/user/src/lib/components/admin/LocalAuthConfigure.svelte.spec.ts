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
	return { id: 'user-1', email, created: '2026-08-18T12:00:00Z' };
}

function mockLocalUsers({
	existing = [] as LocalAuthUser[],
	createUser = vi.fn()
}: {
	existing?: LocalAuthUser[];
	createUser?: (body: { email: string; password: string }) => void;
} = {}) {
	let users = [...existing];

	worker.use(
		http.get('/api/local-auth/users', () => HttpResponse.json({ items: users })),
		http.post('/api/local-auth/users', async ({ request }) => {
			const body = (await request.json()) as { email: string; password: string };
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
				password: validPassword
			});
		});
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
});
