import { CommonAuthProviderIds, LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
import { Group } from '$lib/services';
import type { AuthProvider, LocalAuthUser } from '$lib/services/admin/types';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { initiateTempLoginResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import SetupPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

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
	missingEntitlements: [],
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
	createUser = vi.fn()
}: {
	users?: LocalAuthUser[];
	createUser?: (body: { email: string; password: string; requirePasswordChange: boolean }) => void;
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
		http.post('/api/setup/initiate-temp-login', initiateTempLogin),
		http.post('/api/setup/cancel-temp-login', () => new HttpResponse(null, { status: 404 })),
		http.get('/api/setup/explicit-role-emails', () =>
			HttpResponse.json({ owners: null, admins: null })
		)
	);

	return { createUser, initiateTempLogin };
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

	it('creates the initial owner and slides to the sign-in step', async () => {
		const { createUser, initiateTempLogin } = mockSetupApis();
		await renderSetupPage({ splashSeen: true, eulaAccepted: true, localProvider: localConfigured });

		await completeOwnerForm();

		await vi.waitFor(() => {
			expect(createUser).toHaveBeenCalledWith({
				email: 'ada@example.com',
				password: validPassword,
				requirePasswordChange: false
			});
			expect(initiateTempLogin).toHaveBeenCalledOnce();
		});

		await expect.element(page.getByText('Next Step: Owner Setup', { exact: true })).toBeVisible();
		const signIn = page.getByRole('link', { name: 'Sign in as ada@example.com' });
		await expect.element(signIn).toBeVisible();
		await expect.element(signIn).toHaveAttribute('href', initiateTempLoginResponse.redirectUrl);
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
			.element(page.getByRole('link', { name: 'Sign in as owner@example.com' }))
			.toBeVisible();
	});
});
