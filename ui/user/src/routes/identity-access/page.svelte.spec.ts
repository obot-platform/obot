import { replaceState } from '$app/navigation';
import { page as appPage } from '$app/state';
import { CommonAuthProviderIds, LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
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

vi.mock('$app/navigation', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/navigation')>();
	return {
		...actual,
		replaceState: vi.fn()
	};
});

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
	provider,
	groups,
	apiKeys = [],
	users = []
}: {
	authProviders?: AuthProvider[];
	authEnabled?: boolean;
	bootstrap?: boolean;
	view?: string;
	provider?: string;
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

afterEach(() => {
	appPage.url.searchParams.delete('view');
	appPage.url.searchParams.delete('provider');
	if (window.location.hash) {
		window.history.replaceState(null, '', window.location.pathname + window.location.search);
	}
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
			const signIn = page.getByRole('link', { name: 'Sign in as owner@example.com' });
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
				.element(page.getByRole('link', { name: 'Sign in as owner@example.com' }))
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
			await expect.element(page.getByRole('link', { name: 'Continue with Local' })).toBeVisible();
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
							/Register to unlock all remaining providers and to subscribe to the free Obot Community Newsletter/,
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
			const renderAsOwner = (authProviders: AuthProvider[]) =>
				renderIdentityAccessPage({ authProviders, groups: [Group.ADMIN, Group.OWNER] });

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
					.element(page.getByRole('button', { name: 'Unstage', exact: true }))
					.toBeVisible();
				// Completing the switch is not reachable until a sign-in has been recorded.
				await expect
					.element(page.getByRole('button', { name: /^Switch to/, exact: false }))
					.not.toBeInTheDocument();
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
		});
	});
});
