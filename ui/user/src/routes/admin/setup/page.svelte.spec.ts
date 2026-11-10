import { CommonAuthProviderIds, LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
import * as navigation from '$lib/navigation';
import { Group } from '$lib/services';
import type { AuthProvider, LocalAuthUser } from '$lib/services/admin/types';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { initiateTempLoginResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import SetupPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

afterEach(() => {
	vi.mocked(navigation.navigateTo).mockRestore();
});

const localProvider: AuthProvider = {
	id: CommonAuthProviderIds.LOCAL,
	created: '2026-08-04T16:58:40-04:00',
	type: 'authprovider',
	name: 'Local',
	icon: '/admin/assets/local_icon_small.png',
	image: '',
	port: 0,
	configured: false,
	missingConfigurationParameters: [],
	entitlementsMissing: false,
	namespace: 'default'
};

const localConfigured: AuthProvider = {
	...localProvider,
	configured: true
};

const validPassword = 'a'.repeat(LOCAL_AUTH_MIN_PASSWORD_LENGTH);

function createdUser(email: string): LocalAuthUser {
	return { id: 'user-1', email, created: '2026-01-01T00:00:00.000Z', requirePasswordChange: false };
}

function mockSetupApis({
	users = [] as LocalAuthUser[],
	createUser = vi.fn(),
	createOwner = vi.fn()
}: {
	users?: LocalAuthUser[];
	createUser?: (body: { email: string; password: string; requirePasswordChange: boolean }) => void;
	createOwner?: (body: { email: string; password: string }) => void;
} = {}) {
	let localUsers = [...users];
	const initiateTempLogin = vi.fn(() => HttpResponse.json(initiateTempLoginResponse));

	worker.use(
		http.put('/api/eula', () => HttpResponse.json({ accepted: true })),
		http.get('/api/auth-providers', () => HttpResponse.json({ items: [localConfigured] })),
		http.post(
			`/api/auth-providers/${localProvider.id}/configure`,
			() => new HttpResponse(null, { status: 204 })
		),
		http.get('/api/local-auth/users', () => HttpResponse.json({ items: localUsers })),
		http.post('/api/local-auth/users', async ({ request }) => {
			const body = (await request.json()) as {
				email: string;
				password: string;
				requirePasswordChange: boolean;
			};
			createUser(body);
			const user = createdUser(body.email);
			localUsers = [...localUsers, user];
			return HttpResponse.json(user);
		}),
		http.post('/api/setup/local-owner', async ({ request }) => {
			const body = (await request.json()) as { email: string; password: string };
			createOwner(body);
			localUsers = [...localUsers, createdUser(body.email)];
			return HttpResponse.json({ email: body.email, redirectUrl: '/api/setup/oauth-complete' });
		}),
		http.post('/api/setup/initiate-temp-login', initiateTempLogin),
		http.post('/api/setup/cancel-temp-login', () => new HttpResponse(null, { status: 404 }))
	);

	return { createUser, createOwner, initiateTempLogin };
}

async function renderSetupPage(
	overrides: Partial<PageData> = {},
	profileOverrides: { bootstrap?: boolean } = { bootstrap: true }
) {
	const profile = createMockProfile([Group.ADMIN]);
	if (profileOverrides.bootstrap !== false) {
		profile.username = 'bootstrap';
		profile.isBootstrapUser = () => true;
	}

	const data = await preparePageData<PageData>({
		profile,
		localProvider,
		localUsers: [],
		eulaAccepted: false,
		splashSeen: false,
		...overrides
	});

	return render(SetupPage, { data });
}

async function completeOwnerForm(email = 'ada@example.com') {
	await page.getByLabelText('Email', { exact: true }).fill(email);
	await page.getByCSS('#initial-user-password').fill(validPassword);
	await page.getByCSS('#initial-user-password-confirm').fill(validPassword);
	await page.getByRole('button', { name: 'Continue', exact: true }).click();
}

describe('Setup page', () => {
	it('starts on the welcome splash and slides to the owner form after continue', async () => {
		mockSetupApis();
		await renderSetupPage();

		await expect
			.element(page.getByRole('heading', { name: 'Welcome to Obot!', exact: true }))
			.toBeVisible();
		await expect.element(page.getByLabelText('Email', { exact: true })).not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Continue', exact: true }).click();

		await expect
			.element(page.getByRole('heading', { name: 'Set Up Owner Account', exact: true }))
			.toBeVisible();
		await expect.element(page.getByLabelText('Email', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Welcome to Obot!', exact: true }))
			.not.toBeInTheDocument();
	});

	it('starts on the owner form when the splash has already been seen', async () => {
		mockSetupApis();
		await renderSetupPage({ splashSeen: true, eulaAccepted: true });

		await expect
			.element(page.getByRole('heading', { name: 'Set Up Owner Account', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Welcome to Obot!', exact: true }))
			.not.toBeInTheDocument();
	});

	it('creates the initial owner, signs them in, and finishes setup without a second login', async () => {
		vi.mocked(navigation.navigateTo).mockImplementation(() => {});
		const { createUser, createOwner, initiateTempLogin } = mockSetupApis();
		await renderSetupPage({ splashSeen: true, eulaAccepted: true, localProvider: localConfigured });

		await completeOwnerForm();

		await vi.waitFor(() => {
			expect(createOwner).toHaveBeenCalledWith({
				email: 'ada@example.com',
				password: validPassword
			});
			expect(navigation.navigateTo).toHaveBeenCalledWith('/api/setup/oauth-complete');
		});
		expect(createUser).not.toHaveBeenCalled();
		expect(initiateTempLogin).not.toHaveBeenCalled();
		await expect
			.element(page.getByText('Next Step: Owner Setup', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('falls back to the sign-in step when the owner was created but could not be signed in', async () => {
		const { initiateTempLogin } = mockSetupApis();
		worker.use(
			http.post(
				'/api/setup/local-owner',
				() => new HttpResponse('session failed', { status: 500 })
			),
			http.get('/api/local-auth/users', () =>
				HttpResponse.json({ items: [createdUser('ada@example.com')] })
			)
		);
		await renderSetupPage({ splashSeen: true, eulaAccepted: true, localProvider: localConfigured });

		await completeOwnerForm();

		await expect.element(page.getByText('Next Step: Owner Setup', { exact: true })).toBeVisible();
		await vi.waitFor(() => expect(initiateTempLogin).toHaveBeenCalledOnce());
		await expect
			.element(page.getByRole('link', { name: /Sign in as ada@example.com/ }))
			.toBeVisible();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it('lists local users only once when falling back to the sign-in step', async () => {
		mockSetupApis();
		const listUsers = vi.fn(() => HttpResponse.json({ items: [createdUser('ada@example.com')] }));
		worker.use(
			http.post(
				'/api/setup/local-owner',
				() => new HttpResponse('session failed', { status: 500 })
			),
			http.get('/api/local-auth/users', listUsers)
		);
		await renderSetupPage({ splashSeen: true, eulaAccepted: true, localProvider: localConfigured });

		await completeOwnerForm();

		await expect
			.element(page.getByRole('link', { name: /Sign in as ada@example.com/ }))
			.toBeVisible();
		expect(listUsers).toHaveBeenCalledOnce();
	});

	it('shows the conflict instead of signing in when another account was created first', async () => {
		const { initiateTempLogin } = mockSetupApis();
		worker.use(
			http.post(
				'/api/setup/local-owner',
				() => new HttpResponse('an initial local account already exists', { status: 409 })
			),
			http.get('/api/local-auth/users', () =>
				HttpResponse.json({ items: [createdUser('other@example.com')] })
			)
		);
		await renderSetupPage({ splashSeen: true, eulaAccepted: true, localProvider: localConfigured });

		await completeOwnerForm('ada@example.com');

		await expect
			.element(
				page
					.getByRole('alert')
					.filter({ hasText: 'an initial local account already exists' })
					.first()
			)
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Set Up Owner Account', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: /Sign in as other@example.com/ }))
			.not.toBeInTheDocument();
		expect(initiateTempLogin).not.toHaveBeenCalled();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it('keeps the owner form with an error when the owner could not be created', async () => {
		mockSetupApis();
		worker.use(
			http.post(
				'/api/setup/local-owner',
				() => new HttpResponse('email is not allowed', { status: 400 })
			)
		);
		await renderSetupPage({ splashSeen: true, eulaAccepted: true, localProvider: localConfigured });

		await completeOwnerForm();

		await expect
			.element(page.getByRole('alert').filter({ hasText: 'email is not allowed' }).first())
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Set Up Owner Account', exact: true }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Continue', exact: true })).toBeEnabled();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it('opens user management without configuration when changing the owner account', async () => {
		const owner = createdUser('owner@example.com');
		mockSetupApis({ users: [owner] });
		await renderSetupPage({
			splashSeen: true,
			eulaAccepted: true,
			localProvider: localConfigured,
			localUsers: [owner]
		});

		await page.getByRole('button', { name: 'Click here', exact: true }).click();

		await expect.element(page.getByRole('heading', { name: 'Users', exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Configuration', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByText('Allowed Email Domains', { exact: true }))
			.not.toBeInTheDocument();
	});

	it('starts on owner sign-in when a local account already exists', async () => {
		const owner = createdUser('owner@example.com');
		const { initiateTempLogin } = mockSetupApis({ users: [owner] });
		await renderSetupPage({
			splashSeen: true,
			eulaAccepted: true,
			localProvider: localConfigured,
			localUsers: [owner]
		});

		await expect.element(page.getByText('Next Step: Owner Setup', { exact: true })).toBeVisible();
		await vi.waitFor(() => {
			expect(initiateTempLogin).toHaveBeenCalledOnce();
		});
		await expect
			.element(page.getByRole('link', { name: /Sign in as owner@example.com/ }))
			.toBeVisible();
	});
});
