import { CommonAuthProviderIds } from '$lib/constants';
import type { AuthProvider } from '$lib/services/admin/types';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import {
	initiateTempLoginResponse,
	listAuthProvidersResponse,
	listExplicitRoleEmailsResponse
} from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import AuthProvidersPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
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

async function renderAuthProvidersPage({
	authProviders = [googleProvider, entraProvider],
	bootstrap = false
}: {
	authProviders?: AuthProvider[];
	bootstrap?: boolean;
} = {}) {
	const profile = createMockProfile();
	if (bootstrap) {
		profile.username = 'bootstrap';
		profile.isBootstrapUser = () => true;
	}

	const data = await preparePageData<PageData>({
		authProviders,
		profile
	});

	return render(AuthProvidersPage, { data });
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

describe('Auth Providers Page', () => {
	describe('configure auth provider', () => {
		it('bootstrap user sees owner handoff dialog after configuring', async () => {
			const { configureAuthProvider } = mockConfigureFlow([googleConfigured]);
			await renderAuthProvidersPage({ authProviders: [googleProvider], bootstrap: true });

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
			await renderAuthProvidersPage({ authProviders: [googleProvider], bootstrap: false });

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
		it('shows license required and opens license dialog on Configure', async () => {
			await renderAuthProvidersPage({ authProviders: [entraProvider] });

			await expect
				.element(providerCard('Microsoft Entra').getByText('License Required', { exact: true }))
				.toBeVisible();

			await providerCard('Microsoft Entra')
				.getByRole('button', { name: 'Configure', exact: true })
				.click();

			await expect
				.element(page.getByRole('heading', { name: 'Microsoft Entra', exact: true }).first())
				.toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'License Required', exact: true }))
				.toBeVisible();
			await expect
				.element(
					page.getByText(/A valid license is required to use Microsoft Entra/, { exact: false })
				)
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Close', exact: true })).toBeVisible();
			await expect
				.element(page.getByText('Set Up Microsoft Entra', { exact: true }))
				.not.toBeInTheDocument();
		});
	});
});
