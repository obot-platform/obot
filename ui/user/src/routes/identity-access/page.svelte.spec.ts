import { page as appPage } from '$app/state';
import { CommonAuthProviderIds } from '$lib/constants';
import { Group } from '$lib/services';
import type { AuthProvider } from '$lib/services/admin/types';
import type { APIKey } from '$lib/services/api-keys/types';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import {
	initiateTempLoginResponse,
	listAuthProvidersResponse,
	listExplicitRoleEmailsResponse,
	listUsersResponse
} from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import IdentityAccessPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const googleProvider = listAuthProvidersResponse.find(
	(provider) => provider.id === CommonAuthProviderIds.GOOGLE
)!;
const entraProvider = listAuthProvidersResponse.find(
	(provider) => provider.id === CommonAuthProviderIds.ENTRA
)!;

const googleConfigured: AuthProvider = {
	...googleProvider,
	configured: true,
	missingConfigurationParameters: []
};

function providerCard(name: string) {
	return page.getByRole('heading', { name, exact: true }).locator('..');
}

async function renderIdentityAccessPage({
	authProviders = [googleProvider, entraProvider],
	authEnabled = true,
	bootstrap = false,
	view = 'auth-providers',
	groups,
	apiKeys = [],
	users = []
}: {
	authProviders?: AuthProvider[];
	authEnabled?: boolean;
	bootstrap?: boolean;
	view?: string;
	groups?: string[];
	apiKeys?: APIKey[];
	users?: PageData['users'];
} = {}) {
	const profile = createMockProfile(groups);
	if (bootstrap) {
		profile.username = 'bootstrap';
		profile.isBootstrapUser = () => true;
	}

	appPage.url.searchParams.set('view', view);
	// Layout's bootstrap splash would otherwise sit on top of the Auth Providers tab.
	localStorage.setItem('seenSplashDialog', new Date().toISOString());

	const data = await preparePageData<PageData>({
		users,
		groups: [],
		groupRoleAssignments: [],
		defaultUsersRole: undefined,
		authProviders,
		authEnabled,
		apiKeys,
		profile
	});

	return render(IdentityAccessPage, { data });
}

function mockConfigureFlow(configuredProviders: AuthProvider[]) {
	const configureAuthProvider = vi.fn(async ({ request }) => {
		const body = await request.json();
		expect(body).toMatchObject({
			OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_ID: 'test-client-id',
			OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_SECRET: 'test-client-secret',
			OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*'
		});
		return new HttpResponse(null, { status: 204 });
	});

	worker.use(
		http.post(`/api/auth-providers/${googleProvider.id}/reveal`, () =>
			HttpResponse.json(null, { status: 404 })
		),
		http.post(`/api/auth-providers/${googleProvider.id}/configure`, configureAuthProvider),
		http.get('/api/auth-providers', () => HttpResponse.json({ items: configuredProviders })),
		http.post('/api/setup/cancel-temp-login', () => new HttpResponse(null, { status: 404 })),
		http.get('/api/setup/explicit-role-emails', () =>
			HttpResponse.json(listExplicitRoleEmailsResponse)
		),
		http.post('/api/setup/initiate-temp-login', () => HttpResponse.json(initiateTempLoginResponse))
	);

	return { configureAuthProvider };
}

async function configureGoogleProvider() {
	await providerCard('Google').getByRole('button', { name: 'Configure', exact: true }).click();

	const dialog = page.getByRole('dialog');
	await expect.element(dialog.getByText('Set Up Google', { exact: true })).toBeVisible();

	await dialog.getByLabelText('Client ID', { exact: true }).fill('test-client-id');
	await dialog.getByLabelText('Client Secret', { exact: true }).fill('test-client-secret');
	await dialog.getByRole('button', { name: 'Confirm', exact: true }).click();
}

afterEach(() => {
	appPage.url.searchParams.delete('view');
});

describe('Identity & Access Page', () => {
	describe('agents tab', () => {
		const apiKey: APIKey = {
			id: 42,
			userId: Number(listUsersResponse[0].id),
			name: 'Test Agent Scope',
			canAccessAPI: false,
			canAccessLLMProxy: true,
			canAccessSkills: false,
			canAccessDeviceScans: false,
			createdAt: '2026-01-01T00:00:00.000Z'
		};

		it('non-admin users only see the Agents tab and can create a scope', async () => {
			await renderIdentityAccessPage({
				view: 'agents',
				groups: [Group.USER],
				apiKeys: [apiKey]
			});

			await expect
				.element(page.getByRole('button', { name: 'Users', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Groups', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Roles', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Auth Providers', exact: true }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Create Agent Auth Scope', exact: true }))
				.toBeVisible();
			await expect.element(page.getByText(apiKey.name, { exact: true })).toBeVisible();
		});

		it('hides create for readonly admins', async () => {
			await renderIdentityAccessPage({
				view: 'agents',
				groups: [Group.AUDITOR],
				apiKeys: [apiKey]
			});

			await expect
				.element(page.getByRole('button', { name: 'Create Agent Auth Scope', exact: true }))
				.not.toBeInTheDocument();
		});
	});

	describe('auth providers tab', () => {
		describe('configure auth provider', () => {
			it('bootstrap user sees owner handoff dialog after configuring', async () => {
				const { configureAuthProvider } = mockConfigureFlow([googleConfigured]);
				await renderIdentityAccessPage({ authProviders: [googleProvider], bootstrap: true });

				await expect
					.element(page.getByRole('button', { name: 'Auth Providers', exact: true }))
					.toBeVisible();
				await configureGoogleProvider();

				await vi.waitFor(() => {
					expect(configureAuthProvider).toHaveBeenCalledOnce();
				});

				await expect
					.element(
						page.getByRole('dialog').getByText('Next Step: Owner Login Setup', { exact: true })
					)
					.toBeVisible();
				await expect
					.element(page.getByRole('dialog').getByRole('link', { name: /Continue with Google/ }))
					.toBeVisible();
				await expect
					.element(page.getByRole('dialog').getByRole('link', { name: /Continue with Google/ }))
					.toHaveAttribute('href', initiateTempLoginResponse.redirectUrl);
			});

			it('non-bootstrap user does not see handoff and provider shows as configured', async () => {
				const { configureAuthProvider } = mockConfigureFlow([googleConfigured]);
				await renderIdentityAccessPage({ authProviders: [googleProvider], bootstrap: false });

				await configureGoogleProvider();

				await vi.waitFor(() => {
					expect(configureAuthProvider).toHaveBeenCalledOnce();
				});

				await expect
					.element(page.getByRole('dialog').filter({ hasText: 'Next Step: Owner Login Setup' }))
					.not.toBeInTheDocument();

				await expect
					.element(providerCard('Google').getByText('Configured', { exact: true }))
					.toBeVisible();
				await expect
					.element(providerCard('Google').getByRole('button', { name: 'Modify', exact: true }))
					.toBeVisible();
			});
		});

		describe('license required auth provider', () => {
			it('offers Obot Community signup in the license dialog on Configure', async () => {
				await renderIdentityAccessPage({ authProviders: [entraProvider] });

				await expect
					.element(
						providerCard('Microsoft Entra').getByText('Registration Required', { exact: true })
					)
					.toBeVisible();

				await providerCard('Microsoft Entra')
					.getByRole('button', { name: 'Configure', exact: true })
					.click();

				await expect
					.element(page.getByRole('heading', { name: 'Microsoft Entra', exact: true }).first())
					.toBeVisible();
				await expect
					.element(page.getByRole('heading', { name: 'Get Access Now!', exact: true }))
					.toBeVisible();
				await expect
					.element(
						page.getByText(
							/Register to unlock all remaining providers and to subscribe to the free Obot Community Newsletter/,
							{
								exact: false
							}
						)
					)
					.toBeVisible();
				await expect.element(page.getByLabelText('Name', { exact: true })).toBeVisible();
				await expect.element(page.getByLabelText('Email', { exact: true })).toBeVisible();
				await expect.element(page.getByLabelText('Company', { exact: false })).toBeVisible();
				await expect
					.element(page.getByRole('button', { name: 'Register', exact: true }))
					.toBeVisible();
				await expect
					.element(page.getByText('Set Up Microsoft Entra', { exact: true }))
					.not.toBeInTheDocument();
			});
		});
	});
});
