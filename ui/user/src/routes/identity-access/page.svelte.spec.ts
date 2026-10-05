import { replaceState } from '$app/navigation';
import { page as appPage } from '$app/state';
import { CommonAuthProviderIds, LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
import * as navigation from '$lib/navigation';
import { Group } from '$lib/services';
import type { AuthProvider } from '$lib/services/admin/types';
import type { APIKey } from '$lib/services/api-keys/types';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import {
	getProfileResponse,
	initiateTempLoginResponse,
	listAuthProvidersResponse,
	listExplicitRoleEmailsResponse,
	listUsersResponse
} from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import IdentityAccessPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/navigation')>();
	return {
		...actual,
		replaceState: vi.fn()
	};
});

vi.mock(import('$lib/navigation'), { spy: true });

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

const oktaProvider: AuthProvider = {
	id: 'okta-auth-provider',
	created: googleProvider.created,
	type: 'authprovider',
	name: 'Okta',
	icon: '/admin/assets/okta_icon_small.png',
	image: '',
	port: 0,
	configured: false,
	missingConfigurationParameters: [],
	missingEntitlements: [],
	namespace: 'default',
	requiredConfigurationParameters: [
		{
			name: 'OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID',
			friendlyName: 'Client ID'
		},
		{
			name: 'OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL',
			friendlyName: 'Org URL'
		}
	],
	optionalConfigurationParameters: [
		{
			name: 'OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID',
			friendlyName: 'API Services Client ID',
			description:
				'Leave this and the private key empty to provision users and groups through SCIM instead.'
		},
		{
			name: 'OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY',
			friendlyName: 'API Services Private Key',
			sensitive: true
		}
	],
	scim: {
		directoryParameters: [
			'OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID',
			'OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY'
		],
		issuerParameter: 'OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL'
	}
};

function providerCard(name: string) {
	return page.getByRole('heading', { name, exact: true }).locator('..');
}

async function renderIdentityAccessPage({
	authProviders = [googleProvider, entraProvider],
	authEnabled = true,
	bootstrap = false,
	view = 'auth-providers',
	provider,
	groups,
	apiKeys = [],
	users = [],
	currentAuthProvider
}: {
	authProviders?: AuthProvider[];
	authEnabled?: boolean;
	bootstrap?: boolean;
	view?: string;
	provider?: string;
	groups?: string[];
	apiKeys?: APIKey[];
	users?: PageData['users'];
	currentAuthProvider?: string;
} = {}) {
	const profile = createMockProfile(groups);
	profile.currentAuthProvider = currentAuthProvider;
	if (bootstrap) {
		profile.username = 'bootstrap';
		profile.isBootstrapUser = () => true;
	}

	appPage.url.searchParams.set('view', view);
	if (provider) {
		appPage.url.searchParams.set('provider', provider);
	}
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

const localProvider: AuthProvider = {
	id: CommonAuthProviderIds.LOCAL,
	created: googleProvider.created,
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

function mockLocalOnboarding({
	owners = null as string[] | null,
	redirectUrl = initiateTempLoginResponse.redirectUrl
} = {}) {
	const initiateTempLogin = vi.fn(() =>
		HttpResponse.json({ ...initiateTempLoginResponse, redirectUrl })
	);
	let users: { id: string; email: string; created: string; requirePasswordChange: boolean }[] = [];

	worker.use(
		http.post(`/api/auth-providers/${localProvider.id}/reveal`, () =>
			HttpResponse.json(null, { status: 404 })
		),
		http.post(
			`/api/auth-providers/${localProvider.id}/configure`,
			() => new HttpResponse(null, { status: 204 })
		),
		http.get('/api/auth-providers', () => HttpResponse.json({ items: [localConfigured] })),
		http.get('/api/local-auth/users', () => HttpResponse.json({ items: users })),
		http.post('/api/local-auth/users', async ({ request }) => {
			const body = (await request.json()) as { email: string };
			const user = {
				id: 'user-1',
				email: body.email,
				created: '2026-01-01T00:00:00.000Z',
				requirePasswordChange: false
			};
			users = [...users, user];
			return HttpResponse.json(user);
		}),
		http.post('/api/setup/cancel-temp-login', () => new HttpResponse(null, { status: 404 })),
		http.get('/api/setup/explicit-role-emails', () => HttpResponse.json({ owners, admins: null })),
		http.post('/api/setup/initiate-temp-login', initiateTempLogin)
	);

	return { initiateTempLogin };
}

async function createInitialLocalUser() {
	const dialog = page.getByRole('dialog');
	await expect.element(dialog.getByLabelText('Email', { exact: true })).toBeVisible();
	await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
	await page.getByCSS('#initial-user-password').fill('a'.repeat(LOCAL_AUTH_MIN_PASSWORD_LENGTH));
	await page
		.getByCSS('#initial-user-password-confirm')
		.fill('a'.repeat(LOCAL_AUTH_MIN_PASSWORD_LENGTH));
	await dialog.getByRole('button', { name: 'Continue', exact: true }).click();
}

async function configureGoogleProvider() {
	await providerCard('Google').getByRole('button', { name: 'Configure', exact: true }).click();

	const dialog = page.getByRole('dialog');
	await expect.element(dialog.getByText('Set Up Google', { exact: true })).toBeVisible();

	await dialog.getByLabelText('Client ID', { exact: true }).fill('test-client-id');
	await dialog.getByLabelText('Client Secret', { exact: true }).fill('test-client-secret');
	await dialog.getByRole('button', { name: 'Confirm', exact: true }).click();
}

beforeEach(() => {
	vi.mocked(navigation.reloadPage).mockImplementation(() => {});
});

afterEach(() => {
	appPage.url.searchParams.delete('view');
	appPage.url.searchParams.delete('subview');
	appPage.url.searchParams.delete('provider');
	if (window.location.hash) {
		window.history.replaceState(null, '', window.location.pathname + window.location.search);
	}
	vi.mocked(navigation.reloadPage).mockRestore();
});

describe('Identity & Access Page', () => {
	describe('local bootstrap setup', () => {
		const localConfigured: AuthProvider = {
			...googleProvider,
			id: CommonAuthProviderIds.LOCAL,
			name: 'Local',
			configured: true,
			missingConfigurationParameters: []
		};

		function mockLocalSetup(emails: string[]) {
			let users = emails.map((email, i) => ({
				id: String(i + 1),
				email,
				created: '2026-09-15T00:00:00Z',
				requirePasswordChange: false
			}));
			const initiate = vi.fn(() => HttpResponse.json(initiateTempLoginResponse));
			const setPassword = vi.fn(() => new HttpResponse(null, { status: 204 }));
			vi.mocked(replaceState).mockImplementation(() => {});

			worker.use(
				http.get('/api/auth-providers', () => HttpResponse.json({ items: [localConfigured] })),
				http.post(`/api/auth-providers/${CommonAuthProviderIds.LOCAL}/reveal`, () =>
					HttpResponse.json({ OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*' })
				),
				http.get('/api/local-auth/users', () => HttpResponse.json({ items: users })),
				http.post('/api/local-auth/users', async ({ request }) => {
					const body = (await request.json()) as { email: string };
					const user = {
						id: '1',
						email: body.email,
						created: '2026-09-15T00:00:00Z',
						requirePasswordChange: false
					};
					users = [...users, user];
					return HttpResponse.json(user);
				}),
				http.post('/api/local-auth/users/:id/password', setPassword),
				http.post('/api/setup/initiate-temp-login', initiate),
				http.post('/api/setup/cancel-temp-login', () => new HttpResponse(null, { status: 404 })),
				http.get('/api/setup/explicit-role-emails', () =>
					HttpResponse.json(listExplicitRoleEmailsResponse)
				)
			);

			return { initiate, setPassword };
		}

		function ownerLoginPrompt() {
			return page.getByText('Next Step: Owner Setup');
		}

		it('offers the owner login as a button rather than redirecting to it', async () => {
			mockLocalSetup(['owner@example.com']);
			await renderIdentityAccessPage({ authProviders: [localConfigured], bootstrap: true });

			await expect.element(ownerLoginPrompt()).toBeVisible();
			const signIn = page.getByRole('link', { name: /Sign in as owner@example.com/ });
			await expect.element(signIn).toHaveAttribute('href', initiateTempLoginResponse.redirectUrl);
		});

		it('prompts for the owner login after saving the initial account', async () => {
			const { initiate } = mockLocalSetup([]);
			await renderIdentityAccessPage({ authProviders: [localConfigured], bootstrap: true });

			await providerCard('Local').getByRole('button', { name: 'Modify', exact: true }).click();
			const dialog = page.getByRole('dialog').filter({ hasText: 'Set Up Local' });
			await dialog.getByLabelText('Email', { exact: true }).fill('owner@example.com');
			await page.getByCSS('#local-user-password-draft').fill('initial-owner-password');
			expect(initiate).not.toHaveBeenCalled();
			await dialog.getByRole('button', { name: 'Save', exact: true }).click();

			await expect.element(ownerLoginPrompt()).toBeVisible();
			await expect
				.element(page.getByRole('link', { name: /Sign in as owner@example.com/ }))
				.toBeVisible();
		});

		it('lets the bootstrap user reset a forgotten password from the owner login prompt', async () => {
			const { setPassword } = mockLocalSetup(['owner@example.com']);
			await renderIdentityAccessPage({ authProviders: [localConfigured], bootstrap: true });

			await expect.element(ownerLoginPrompt()).toBeVisible();
			await page.getByRole('button', { name: 'Click here' }).click();

			const dialog = page.getByRole('dialog').filter({ hasText: 'Set Up Local' });
			await expect.element(dialog.getByText('owner@example.com')).toBeVisible();
			await dialog.getByRole('button', { name: 'Reset password' }).click();
			await page.getByCSS('#reset-password-1').fill('a-password-i-will-remember');
			await dialog.getByRole('button', { name: 'Save', exact: true }).click();

			await vi.waitFor(() => expect(setPassword).toHaveBeenCalledOnce());
			await expect.element(ownerLoginPrompt()).toBeVisible();
		});

		it('keeps explicit handoff for a legacy setup with multiple local accounts', async () => {
			mockLocalSetup(['owner@example.com', 'legacy@example.com']);
			await renderIdentityAccessPage({ authProviders: [localConfigured], bootstrap: true });

			await expect.element(ownerLoginPrompt()).toBeVisible();
			await expect.element(page.getByRole('link', { name: /Continue with Local/ })).toBeVisible();
		});
	});

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
				.element(page.getByRole('button', { name: 'Create Agent Identity', exact: true }))
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
				.element(page.getByRole('button', { name: 'Create Agent Identity', exact: true }))
				.not.toBeInTheDocument();
		});
	});

	describe('auth providers tab', () => {
		describe('sub-tabs', () => {
			function subTabs() {
				return page.getByRole('navigation', { name: 'Auth Providers' });
			}

			it('shows the providers, and links to SCIM, which has no tab of its own', async () => {
				await renderIdentityAccessPage({ groups: [Group.OWNER, Group.ADMIN] });

				await expect.element(providerCard('Google')).toBeVisible();
				await expect
					.element(subTabs().getByRole('link', { name: 'Providers', exact: true }))
					.toHaveAttribute('aria-current', 'page');
				await expect
					.element(subTabs().getByRole('link', { name: 'SCIM', exact: true }))
					.toHaveAttribute('href', '/identity-access?view=auth-providers&subview=scim');
				await expect
					.element(page.getByRole('button', { name: 'SCIM', exact: true }))
					.not.toBeInTheDocument();
			});

			it('shows SCIM on its sub-tab', async () => {
				appPage.url.searchParams.set('subview', 'scim');
				await renderIdentityAccessPage({ groups: [Group.OWNER, Group.ADMIN] });

				await expect
					.element(page.getByRole('heading', { name: 'SCIM provisioning is not set up' }))
					.toBeVisible();
				await expect
					.element(subTabs().getByRole('link', { name: 'SCIM', exact: true }))
					.toHaveAttribute('aria-current', 'page');
				await expect
					.element(subTabs().getByRole('link', { name: 'Providers', exact: true }))
					.toHaveAttribute('href', '/identity-access?view=auth-providers');
				await expect.element(providerCard('Google')).not.toBeInTheDocument();
			});
		});

		describe('SCIM setup of the Okta provider', () => {
			async function openOktaForm(provider: AuthProvider, values?: Record<string, string>) {
				worker.use(
					http.post(`/api/auth-providers/${provider.id}/reveal`, () =>
						values ? HttpResponse.json(values) : HttpResponse.json(null, { status: 404 })
					)
				);
				await renderIdentityAccessPage({
					authProviders: [provider],
					groups: [Group.OWNER, Group.ADMIN]
				});
				const buttonName = provider.configured ? 'Modify' : 'Configure';
				await providerCard('Okta').getByRole('button', { name: buttonName, exact: true }).click();
				await expect.element(page.getByLabelText('Org URL', { exact: true })).toBeVisible();
			}

			it('explains the choice the directory credentials make', async () => {
				await openOktaForm(oktaProvider);

				await expect
					.element(
						page.getByText(/With these left empty, Okta provisions users and groups through SCIM/)
					)
					.toBeVisible();

				await page.getByLabelText('API Services Client ID', { exact: true }).fill('service-client');
				await expect
					.element(page.getByText(/With these provided, Obot fetches each user's groups from Okta/))
					.toBeVisible();
			});

			it('warns when the Org URL of a provider with a SCIM connection changes', async () => {
				await openOktaForm(
					{
						...oktaProvider,
						configured: true,
						scimState: 'connected',
						optionalConfigurationParameters: [],
						scim: {
							...oktaProvider.scim!,
							connectionIssuer: 'https://example.okta.com'
						}
					},
					{
						OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID: 'oidc-client',
						OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL: 'https://example.okta.com/'
					}
				);

				await expect
					.element(page.getByText(/belong to the Okta organization they were provisioned from/))
					.not.toBeInTheDocument();
				await page.getByLabelText('Org URL', { exact: true }).fill('https://login.example.com');
				await expect
					.element(page.getByText(/belong to the Okta organization they were provisioned from/))
					.toBeVisible();
			});

			describe('group data left from an earlier configuration', () => {
				let deconfigure = vi.fn<() => void>();

				beforeEach(async () => {
					deconfigure = vi.fn<() => void>();
					worker.use(
						http.post(`/api/auth-providers/${oktaProvider.id}/configure`, () =>
							HttpResponse.json(
								{ error: 'Okta still has group data from an earlier configuration' },
								{ status: 409 }
							)
						),
						http.get(`/api/auth-providers/${oktaProvider.id}/residual-group-data`, () =>
							HttpResponse.json({
								groups: [
									{
										id: 'okta/00g00000000000legacy',
										name: 'Legacy',
										references: [
											{
												kind: 'groupRoleAssignment',
												id: 'okta/00g00000000000legacy',
												detail: 'Admin'
											}
										]
									},
									{ id: 'okta/00g000000000000other', name: 'Other' },
									// A referenced group ID that no group has.
									{
										id: 'okta/00g0000000000missing',
										name: '',
										references: [
											{
												kind: 'groupRoleAssignment',
												id: 'okta/00g0000000000missing',
												detail: 'Basic'
											}
										]
									}
								],
								membershipCount: 2
							})
						),
						http.post(`/api/auth-providers/${oktaProvider.id}/deconfigure`, () => {
							deconfigure();
							return new HttpResponse(null, { status: 204 });
						})
					);
					await openOktaForm(oktaProvider);

					await page.getByLabelText('Client ID', { exact: true }).fill('oidc-client');
					await page.getByLabelText('Org URL', { exact: true }).fill('https://example.okta.com');
					await page.getByRole('button', { name: 'Confirm', exact: true }).click();
					await page.getByRole('button', { name: 'Remove leftover group data' }).click();
				});

				it('is removed once the admin confirms what the cleanup deletes', async () => {
					const confirm = page
						.getByRole('dialog')
						.filter({ hasText: "Remove Okta's leftover group data?" });
					await expect
						.element(confirm.getByText(/deletes 2 groups and 2 group memberships/))
						.toBeVisible();
					await expect
						.element(confirm.getByText('group role assignment (Admin)', { exact: false }))
						.toBeVisible();
					await expect
						.element(confirm.getByText('group role assignment (Basic)', { exact: false }))
						.toBeVisible();
					await expect.element(confirm.getByText('Other', { exact: true })).not.toBeInTheDocument();
					expect(deconfigure).not.toHaveBeenCalled();

					await confirm.getByRole('button', { name: 'Remove group data' }).click();
					await vi.waitFor(() => expect(deconfigure).toHaveBeenCalledOnce());
					await expect
						.element(page.getByText(/cleanup of Okta's leftover group data has started/))
						.toBeVisible();
				});

				it('is kept when the admin cancels', async () => {
					const confirm = page
						.getByRole('dialog')
						.filter({ hasText: "Remove Okta's leftover group data?" });
					await confirm.getByRole('button', { name: 'Cancel', exact: true }).click();

					await expect
						.element(page.getByText("Remove Okta's leftover group data?"))
						.not.toBeVisible();
					expect(deconfigure).not.toHaveBeenCalled();
					await expect
						.element(page.getByRole('button', { name: 'Remove leftover group data' }))
						.toBeVisible();
				});
			});

			it('asks to discard the staged switch before removing the group data of a staged provider', async () => {
				const localActive: AuthProvider = {
					...googleProvider,
					id: CommonAuthProviderIds.LOCAL,
					name: 'Local',
					configured: true,
					missingEntitlements: []
				};
				worker.use(
					http.post(`/api/auth-providers/${oktaProvider.id}/reveal`, () =>
						HttpResponse.json(null, { status: 404 })
					),
					http.post(`/api/auth-providers/${oktaProvider.id}/stage`, () =>
						HttpResponse.json(
							{ error: 'Okta still has group data from an earlier configuration' },
							{ status: 409 }
						)
					),
					http.get(`/api/auth-providers/${oktaProvider.id}/residual-group-data`, () =>
						HttpResponse.json({
							groups: [{ id: 'okta/00g000000000000other', name: 'Other' }],
							membershipCount: 1
						})
					)
				);
				await renderIdentityAccessPage({
					authProviders: [localActive, { ...oktaProvider, staged: true }],
					groups: [Group.ADMIN, Group.OWNER]
				});
				await page.getByRole('button', { name: 'Configuration', exact: true }).click();
				await page.getByLabelText('Client ID', { exact: true }).fill('oidc-client');
				await page.getByLabelText('Org URL', { exact: true }).fill('https://example.okta.com');
				await page.getByRole('button', { name: 'Continue', exact: true }).click();

				await expect
					.element(
						page.getByText(/Okta is staged as a replacement\. Discard the staged switch first/)
					)
					.toBeVisible();
				await expect
					.element(page.getByRole('button', { name: 'Remove leftover group data' }))
					.not.toBeInTheDocument();
			});
		});

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
					.element(page.getByRole('dialog').getByText('Next Step: Owner Setup', { exact: true }))
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
					.element(page.getByRole('dialog').filter({ hasText: 'Next Step: Owner Setup' }))
					.not.toBeInTheDocument();

				await expect
					.element(providerCard('Google').getByText('Configured', { exact: true }))
					.toBeVisible();
				await expect
					.element(providerCard('Google').getByRole('button', { name: 'Modify', exact: true }))
					.toBeVisible();
			});

			it('bootstrap local setup with no explicit owners shows owner setup dialog', async () => {
				const { initiateTempLogin } = mockLocalOnboarding();
				await renderIdentityAccessPage({
					authProviders: [localProvider],
					bootstrap: true,
					provider: CommonAuthProviderIds.LOCAL
				});

				await createInitialLocalUser();

				await vi.waitFor(() => {
					expect(initiateTempLogin).toHaveBeenCalledOnce();
				});
				await expect
					.element(page.getByRole('dialog').getByText('Next Step: Owner Setup', { exact: true }))
					.toBeVisible();
				await expect
					.element(
						page.getByRole('dialog').getByRole('link', { name: /Sign in as ada@example.com/ })
					)
					.toHaveAttribute('href', initiateTempLoginResponse.redirectUrl);
			});

			it('bootstrap local setup still prompts when explicit owners are preconfigured', async () => {
				const { initiateTempLogin } = mockLocalOnboarding({ owners: ['owner@example.com'] });
				await renderIdentityAccessPage({
					authProviders: [localProvider],
					bootstrap: true,
					provider: CommonAuthProviderIds.LOCAL
				});

				await createInitialLocalUser();

				await vi.waitFor(() => {
					expect(initiateTempLogin).toHaveBeenCalledOnce();
				});
				await expect
					.element(page.getByRole('dialog').getByText('Next Step: Owner Setup', { exact: true }))
					.toBeVisible();
				await expect
					.element(page.getByRole('dialog').getByText('ada@example.com', { exact: true }))
					.toBeVisible();
				await expect
					.element(
						page.getByRole('dialog').getByRole('link', { name: /Sign in as ada@example.com/ })
					)
					.toHaveAttribute('href', initiateTempLoginResponse.redirectUrl);
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

				const signup = page.getByRole('dialog').filter({ hasText: 'Get Access Now!' });
				await expect
					.element(signup.getByRole('heading', { name: 'Microsoft Entra', exact: true }))
					.toBeVisible();
				await expect
					.element(signup.getByRole('heading', { name: 'Get Access Now!', exact: true }))
					.toBeVisible();
				await expect
					.element(
						signup.getByText(
							/Register to get free access to all additional providers supported by Obot/,
							{
								exact: false
							}
						)
					)
					.toBeVisible();
				await expect.element(signup.getByLabelText('Name', { exact: true })).toBeVisible();
				await expect.element(signup.getByLabelText('Email', { exact: true })).toBeVisible();
				await expect.element(signup.getByLabelText('Company', { exact: false })).toBeVisible();
				await expect
					.element(signup.getByRole('button', { name: 'Register', exact: true }))
					.toBeVisible();
				await expect
					.element(page.getByText('Set Up Microsoft Entra', { exact: true }))
					.not.toBeInTheDocument();
			});
		});

		describe('staged provider switch', () => {
			// Entra requires a license, and that state takes over the card, so the provider being switched
			// to has to be one that does not.
			const localConfigured: AuthProvider = {
				...googleProvider,
				id: CommonAuthProviderIds.LOCAL,
				name: 'Local',
				configured: true,
				missingEntitlements: []
			};
			const stagedGoogle: AuthProvider = {
				...googleProvider,
				staged: true,
				missingEntitlements: []
			};
			const verifiedGoogle: AuthProvider = { ...stagedGoogle, verifiedEmail: 'owner@example.com' };

			// Switching is an owner operation, so every case below renders as one.
			const renderAsOwner = (authProviders: AuthProvider[], currentAuthProvider?: string) =>
				renderIdentityAccessPage({
					authProviders,
					groups: [Group.ADMIN, Group.OWNER],
					currentAuthProvider
				});

			// Deconfiguring the provider serving logins is refused by the server, since it would leave
			// nobody able to sign in, so the card must not offer it either.
			it('does not offer to deconfigure the provider that is serving logins', async () => {
				await renderAsOwner([localConfigured, stagedGoogle]);

				await expect
					.element(providerCard('Local').getByRole('button', { name: 'Modify', exact: true }))
					.toBeVisible();
				await expect.element(providerCard('Local').getByRole('button')).toHaveLength(1);
			});

			it('marks which provider is staged on its card and offers to resume the switch', async () => {
				await renderAsOwner([localConfigured, stagedGoogle]);

				await expect
					.element(providerCard('Google').getByText('Staged', { exact: true }))
					.toBeVisible();
				await expect
					.element(
						providerCard('Google').getByRole('button', { name: 'Resume switch', exact: true })
					)
					.toBeVisible();
			});

			it('asks for a sign-in before offering to complete the switch', async () => {
				await renderAsOwner([localConfigured, stagedGoogle]);

				await expect.element(page.getByText(/becomes the owner of Obot/)).toBeVisible();
				await expect
					.element(page.getByRole('button', { name: /^Sign in with/, exact: false }))
					.toBeVisible();
				await expect
					.element(page.getByRole('button', { name: 'Discard staged switch', exact: true }))
					.toBeVisible();
				// Completing the switch is not reachable until a sign-in has been recorded.
				await expect
					.element(page.getByRole('button', { name: /^Switch to/, exact: false }))
					.not.toBeInTheDocument();
			});

			it('returns to the configuration form from the sign-in step', async () => {
				await renderAsOwner([localConfigured, stagedGoogle]);

				await page.getByRole('button', { name: 'Configuration', exact: true }).click();

				await expect.element(page.getByText('Required Configuration')).toBeVisible();
			});

			it('opens on the switch step once the server reports a verified identity', async () => {
				// Verification is read back from the provider's status rather than from the URL, so the
				// dialog resumes on this step after the redirect, a refresh, or in another tab -- and opens
				// on its own, because the redirect lands here with the switch one click from done.
				await renderAsOwner([localConfigured, verifiedGoogle]);

				await expect.element(page.getByText('owner@example.com', { exact: true })).toBeVisible();
				await expect
					.element(page.getByRole('button', { name: /^Switch to/, exact: false }))
					.toBeVisible();
			});

			it('locks the configuration once an account is verified', async () => {
				await renderAsOwner([localConfigured, verifiedGoogle]);

				await expect
					.element(page.getByRole('button', { name: 'Configuration', exact: true }))
					.not.toBeInTheDocument();

				await page.getByRole('button', { name: 'Sign in', exact: true }).click();
				await expect.element(page.getByText(/becomes the owner of Obot/)).toBeVisible();
				await expect
					.element(page.getByRole('button', { name: 'Configuration', exact: true }))
					.not.toBeInTheDocument();
			});

			it('reloads after discarding from the verified account, which ends its session', async () => {
				worker.use(
					http.delete(`/api/auth-providers/${googleProvider.id}/stage`, () => HttpResponse.json({}))
				);
				await renderAsOwner([localConfigured, verifiedGoogle], verifiedGoogle.id);

				await page.getByRole('button', { name: 'Discard staged switch', exact: true }).click();
				await page.getByRole('button', { name: 'Discard switch', exact: true }).click();

				await vi.waitFor(() => expect(navigation.reloadPage).toHaveBeenCalledOnce());
			});

			it('does not reload after discarding from the outgoing provider', async () => {
				worker.use(
					http.delete(`/api/auth-providers/${googleProvider.id}/stage`, () =>
						HttpResponse.json({})
					),
					http.get('/api/auth-providers', () => HttpResponse.json({ items: [localConfigured] }))
				);
				const sharedEmail: AuthProvider = {
					...stagedGoogle,
					verifiedEmail: getProfileResponse.email
				};
				await renderAsOwner([localConfigured, sharedEmail], localConfigured.id);

				await page.getByRole('button', { name: 'Discard staged switch', exact: true }).click();
				await page.getByRole('button', { name: 'Discard switch', exact: true }).click();

				await expect
					.element(page.getByRole('button', { name: 'Discard staged switch', exact: true }))
					.not.toBeInTheDocument();
				expect(navigation.reloadPage).not.toHaveBeenCalled();
			});

			it('asks for confirmation before completing the switch', async () => {
				await renderAsOwner([localConfigured, verifiedGoogle]);

				await page.getByRole('button', { name: /^Switch to/, exact: false }).click();

				await expect.element(page.getByText(/Switch to Google\?/)).toBeVisible();
			});

			// Local manages its users in its own dialog, which stands in for the first step of a switch.
			// Resuming a staged Local has to reach the sign-in that proves it rather than reopening that
			// dialog, or switching back to Local stages settings and then stops with nowhere to go.
			it('resumes a staged Local provider at the sign-in step', async () => {
				const googleActive: AuthProvider = {
					...googleProvider,
					configured: true,
					missingEntitlements: []
				};
				const stagedLocal: AuthProvider = {
					...googleProvider,
					id: CommonAuthProviderIds.LOCAL,
					name: 'Local',
					configured: false,
					staged: true,
					missingEntitlements: []
				};

				await renderAsOwner([googleActive, stagedLocal]);

				await expect.element(page.getByText(/becomes the owner of Obot/)).toBeVisible();
				await expect
					.element(page.getByRole('button', { name: /^Sign in with/, exact: false }))
					.toBeVisible();
			});

			it('reopens a staged switch', async () => {
				await renderAsOwner([localConfigured, stagedGoogle]);

				await expect
					.element(providerCard('Google').getByText('Staged', { exact: true }))
					.toBeVisible();
				await expect.element(page.getByText(/becomes the owner of Obot/)).toBeVisible();
			});

			it('does not offer an administrator the switch', async () => {
				await renderIdentityAccessPage({ authProviders: [localConfigured, stagedGoogle] });

				await expect
					.element(
						providerCard('Google').getByRole('button', { name: 'Resume switch', exact: true })
					)
					.toBeDisabled();
				await expect.element(page.getByText(/becomes the owner of Obot/)).not.toBeInTheDocument();
			});

			it('warns that users will not transfer before completing the switch', async () => {
				await renderAsOwner([localConfigured, verifiedGoogle]);

				// The switch confirmation has to spell out both consequences, not just the sign-out.
				await expect.element(page.getByText(/sessions\s+end/)).toBeVisible();
				await expect.element(page.getByText(/will\s+not\s+transfer/)).toBeVisible();
			});

			it("says that switching deletes the outgoing provider's SCIM data, and how SCIM starts for the incoming one", async () => {
				await renderAsOwner([
					{ ...localConfigured, scimState: 'enforced' },
					{ ...verifiedGoogle, scimState: 'connected' }
				]);

				await page.getByRole('button', { name: /^Switch to/, exact: false }).click();

				await expect
					.element(page.getByText(/Switching deletes Local's SCIM connection/))
					.toBeVisible();
				await expect
					.element(
						page.getByText(
							/Users that SCIM disabled stay disabled until an administrator enables them/
						)
					)
					.toBeVisible();
				await expect
					.element(page.getByText(/Using SCIM with Local again starts over/))
					.toBeVisible();
				await expect
					.element(
						page.getByText(/After the switch, generate its SCIM token and enter it in Google/)
					)
					.toBeVisible();
			});

			it('says nothing about SCIM when neither provider provisions through it', async () => {
				await renderAsOwner([localConfigured, verifiedGoogle]);

				await page.getByRole('button', { name: /^Switch to/, exact: false }).click();

				await expect.element(page.getByText(/Switch to Google\?/)).toBeVisible();
				await expect
					.element(page.getByText(/SCIM connection|provisions users and groups through SCIM/))
					.not.toBeInTheDocument();
			});

			it('links to the SCIM tab once a switch to a provider that provisions through SCIM completes', async () => {
				const activate = vi.fn();
				worker.use(
					http.post(`/api/auth-providers/${googleProvider.id}/activate`, () => {
						activate();
						return new HttpResponse(null, { status: 204 });
					}),
					http.get('/api/auth-providers', () =>
						HttpResponse.json({
							items: [
								{ ...localConfigured, configured: false },
								{ ...googleConfigured, scimState: 'connected' }
							]
						})
					)
				);
				await renderAsOwner([localConfigured, { ...verifiedGoogle, scimState: 'connected' }]);

				await page.getByRole('button', { name: /^Switch to/, exact: false }).click();
				await page.getByRole('button', { name: 'Switch to Google', exact: true }).last().click();

				await vi.waitFor(() => expect(activate).toHaveBeenCalledOnce());
				await expect
					.element(
						page.getByText(
							/Google now serves sign-ins, and provisions users and groups through SCIM\. Finish setting it up on Auth Providers → SCIM: generate the token and enter it in Google\./
						)
					)
					.toBeVisible();
				const link = page.getByRole('link', { name: 'Go to SCIM', exact: true });
				await expect.element(link).toBeVisible();
				await expect
					.element(link)
					.toHaveAttribute('href', '/identity-access?view=auth-providers&subview=scim');
			});
		});
	});
});
