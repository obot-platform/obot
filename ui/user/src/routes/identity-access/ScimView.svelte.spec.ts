import { AdminService, Group } from '$lib/services';
import type {
	SCIMConnection,
	SCIMConnectionReview,
	SCIMEnablePreview,
	SCIMPage,
	SCIMRequestFailure,
	SCIMSetupGroup,
	SCIMSetupUser,
	SCIMSetupWarning
} from '$lib/services/admin/types';
import errors from '$lib/stores/errors.svelte';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import ScimView from './ScimView.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { describe, expect, it, onTestFinished, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const connectionID = '0b6bd0a4-7e44-4c3c-9d0b-8a3d1f1b8f6e';

function connection(overrides: Partial<SCIMConnection> = {}): SCIMConnection {
	return {
		id: connectionID,
		adapterType: 'okta',
		origin: 'scim_first',
		authProviderNamespace: 'default',
		authProviderName: 'okta-auth-provider',
		authProviderDisplayName: 'Okta',
		state: 'connected',
		baseURL: 'https://obot.example.com/scim/v2',
		issuer: 'https://example.okta.com',
		enabledAt: '2026-09-01T00:00:00.000Z',
		hasToken: false,
		previousTokenAccepted: false,
		authProviderConfigured: true,
		...overrides
	};
}

function user(id: string, overrides: Partial<SCIMSetupUser> = {}): SCIMSetupUser {
	return {
		id,
		email: `user${id}@example.com`,
		displayName: `User ${id}`,
		status: 'active',
		...overrides
	};
}

function group(id: string, name: string, overrides: Partial<SCIMSetupGroup> = {}): SCIMSetupGroup {
	return {
		id,
		name,
		...overrides
	};
}

function failure(resource: string, detail = 'unavailable'): SCIMRequestFailure {
	return {
		time: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
		method: 'PATCH',
		resource,
		status: 400,
		detail
	};
}

function review(overrides: Partial<SCIMConnectionReview> = {}): SCIMConnectionReview {
	return {
		connection: connection(),
		provisionedUsers: { items: [], total: 0 },
		unprovisionedUsers: { items: [user('2', { signedIn: true })], total: 1 },
		boundGroups: { items: [], total: 0 },
		unboundReferencedGroups: { items: [], total: 0 },
		unreferencedGroups: { items: [], total: 0 },
		warnings: [],
		enforceBlockers: [],
		activity: {
			recentFailures: { items: [], total: 0 }
		},
		...overrides
	};
}

// A review of a connection whose setup has reached its last step, Enforce.
function reviewAtEnforce(overrides: Partial<SCIMConnectionReview> = {}): SCIMConnectionReview {
	return review({
		connection: connection({ hasToken: true }),
		provisionedUsers: { items: [user('1', { scimID: 'scim-1', active: true })], total: 1 },
		activity: {
			lastRequestAt: '2026-09-03T00:00:00.000Z',
			recentFailures: { items: [], total: 0 }
		},
		...overrides
	});
}

// The review of an enforced connection, which lists its users and groups.
function enforcedReview(overrides: Partial<SCIMConnectionReview> = {}): SCIMConnectionReview {
	return reviewAtEnforce({
		connection: connection({ state: 'enforced', hasToken: true }),
		...overrides
	});
}

function enablePreview(overrides: Partial<SCIMEnablePreview> = {}): SCIMEnablePreview {
	return {
		authProviderNamespace: 'default',
		authProviderName: 'okta-auth-provider',
		authProviderDisplayName: 'Okta',
		blockers: [],
		duplicateGroupNames: [],
		...overrides
	};
}

async function renderScimView(
	groups: string[],
	props: {
		review?: SCIMConnectionReview;
		enablePreview?: SCIMEnablePreview;
		pageSize?: number;
	} = {},
	bootstrap = false
) {
	const profile = createMockProfile(groups);
	profile.isBootstrapUser = () => bootstrap;
	await preparePageData({ profile });
	return render(ScimView, props);
}

// Enables SCIM, confirming it.
async function confirmEnableSCIM() {
	await page.getByRole('button', { name: 'Enable SCIM' }).click();
	await page.getByRole('button', { name: 'Enable SCIM' }).last().click();
}

describe('ScimView', () => {
	it('explains how to set up SCIM when there is no connection', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN]);

		await expect
			.element(page.getByRole('heading', { name: 'SCIM provisioning is not set up' }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Enforce SCIM' }))
			.not.toBeInTheDocument();
	});

	it('shows the first step of setup that is not done, and the steps done before it', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: reviewAtEnforce({
				provisionedUsers: { items: [], total: 0 },
				unprovisionedUsers: { items: [user('2', { signedIn: true })], total: 1 }
			})
		});

		await expect.element(page.getByText('Not finished', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'Set up provisioning' })).toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'Assign users' })).toBeVisible();
		await expect.element(page.getByText('0 provisioned, 1 not provisioned yet.')).toBeVisible();
		await expect.element(page.getByText('User 2')).toBeVisible();

		const steps = page.getByRole('list', { name: 'Setup steps' });
		await expect
			.element(steps.getByRole('listitem').filter({ hasText: 'Users' }))
			.toHaveAttribute('aria-current', 'step');
		await expect.element(steps.getByRole('button', { name: 'Token, done' })).toBeEnabled();
		await expect.element(steps.getByRole('button', { name: 'SCIM app, done' })).toBeEnabled();
		// The groups need nothing, but the steps ahead of them are not done, so they cannot be shown.
		await expect.element(steps.getByRole('button', { name: 'Groups', exact: true })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Next' })).toBeDisabled();

		await page.getByRole('button', { name: 'Back' }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Create the SCIM app in Okta' }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Next' }).click();
		await expect.element(page.getByRole('heading', { name: 'Assign users' })).toBeVisible();
		await steps.getByRole('button', { name: 'Token, done' }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Generate the bearer token' }))
			.toBeVisible();
	});

	it('refreshes without moving on, and moves on with Next once the step is done', async () => {
		worker.use(
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(reviewAtEnforce({ provisionedUsers: { items: [], total: 0 } }))
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ hasToken: true }) })
		});

		await expect.element(page.getByText('Waiting for Okta to send a request.')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Next' })).toBeDisabled();
		await page.getByRole('button', { name: 'Refresh' }).click();

		await expect
			.element(page.getByText('Okta sent its last request', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Create the SCIM app in Okta' }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Next' }).click();
		await expect.element(page.getByRole('heading', { name: 'Assign users' })).toBeVisible();
	});

	it('goes back to a step that is no longer done, and stays there once it is done again', async () => {
		const migrated = connection({ origin: 'migrated', hasToken: true });
		const unpushed = {
			items: [group('okta/00g00000000000legacy', 'Legacy')],
			total: 1
		};
		let provisioned = false;
		worker.use(
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(
					reviewAtEnforce({
						connection: migrated,
						unboundReferencedGroups: unpushed,
						provisionedUsers: provisioned
							? { items: [user('1', { scimID: 'scim-1', active: true })], total: 1 }
							: { items: [], total: 0 }
					})
				)
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: reviewAtEnforce({ connection: migrated, unboundReferencedGroups: unpushed })
		});
		await expect
			.element(page.getByRole('heading', { name: 'Push the referenced groups' }))
			.toBeVisible();

		// The provisioned user was deprovisioned, so assigning users is no longer done.
		await page.getByRole('button', { name: 'Refresh' }).click();
		await expect.element(page.getByRole('heading', { name: 'Assign users' })).toBeVisible();

		provisioned = true;
		await page.getByRole('button', { name: 'Refresh' }).click();
		await expect.element(page.getByText('1 provisioned, 1 not provisioned yet.')).toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'Assign users' })).toBeVisible();
		await page.getByRole('button', { name: 'Next' }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Push the referenced groups' }))
			.toBeVisible();
	});

	it('stays on a step that a page of its list shows done', async () => {
		// Every referenced group was pushed since the review loaded.
		worker.use(
			http.get(`*/api/scim-connections/${connectionID}/groups`, () =>
				HttpResponse.json({ items: [], total: 0 })
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			pageSize: 2,
			review: reviewAtEnforce({
				connection: connection({ origin: 'migrated', hasToken: true }),
				unboundReferencedGroups: {
					items: [group('okta/g1', 'Group 1'), group('okta/g2', 'Group 2')],
					total: 3
				}
			})
		});

		await page
			.getByRole('button', { name: 'Next page of referenced groups not pushed yet' })
			.click();

		await expect.element(page.getByText('Every referenced group has been pushed.')).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Push the referenced groups' }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Next' })).toBeEnabled();
	});

	// Okta groups bind by name, so a migrated connection asks for the referenced groups under their names.
	it('asks to push the referenced groups of a migrated Okta connection under their names', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: reviewAtEnforce({
				connection: connection({ origin: 'migrated', hasToken: true }),
				unboundReferencedGroups: {
					items: [group('okta/g1', 'Group 1')],
					total: 1
				}
			})
		});

		await expect.element(page.getByText(/under exactly the name shown/)).toBeVisible();
		await expect.element(page.getByText(/doesn't need to match/)).not.toBeInTheDocument();
	});

	it('asks to push the referenced groups of an enforced Okta connection under their names', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: enforcedReview({
				connection: connection({ origin: 'migrated', state: 'enforced', hasToken: true }),
				unboundReferencedGroups: {
					items: [group('okta/g1', 'Group 1')],
					total: 1
				}
			})
		});

		await expect
			.element(page.getByText(/renaming it there first if its name differs/))
			.toBeVisible();
		await expect.element(page.getByText(/doesn't need to match/)).not.toBeInTheDocument();
	});

	it('shows a failed refresh inline, and keeps what it showed', async () => {
		worker.use(
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json({ error: 'the review could not be loaded' }, { status: 500 })
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ hasToken: true }) })
		});

		await page.getByRole('button', { name: 'Refresh' }).click();

		await expect
			.element(page.getByRole('alert').filter({ hasText: /could not be loaded/ }))
			.toBeVisible();
		await expect.element(page.getByText('Waiting for Okta to send a request.')).toBeVisible();
	});

	it('shows the base URL with a trailing slash and a button that copies it', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], { review: review() });

		await expect
			.element(page.getByText('https://obot.example.com/scim/v2/', { exact: true }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Copy base URL' })).toBeVisible();
	});

	it('lets an Owner generate the first token, and shows it once', async () => {
		const rotate = vi.fn();
		worker.use(
			http.post(`*/api/scim-connections/${connectionID}/rotate-token`, () => {
				rotate();
				return HttpResponse.json(connection({ hasToken: true, token: 'obot_scim_secret' }));
			}),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(review({ connection: connection({ hasToken: true }) }))
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { review: review() });

		await page.getByRole('button', { name: 'Generate token' }).click();
		await page.getByRole('button', { name: 'Generate token' }).last().click();

		await vi.waitFor(() => expect(rotate).toHaveBeenCalledOnce());
		await expect.element(page.getByText('obot_scim_secret')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Copy token' })).toBeVisible();
		await expect.element(page.getByText('It is shown only once', { exact: false })).toBeVisible();
	});

	it('lets the bootstrap user generate the first token', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], { review: review() }, true);

		await expect
			.element(page.getByRole('heading', { name: 'Generate the bearer token' }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Generate token' })).toBeVisible();
	});

	it('tells administrators who are not Owners that an Owner generates the token', async () => {
		await renderScimView([Group.ADMIN], { review: review() });

		await expect.element(page.getByText('An Owner generates the token.')).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Generate token' }))
			.not.toBeInTheDocument();
	});

	it('offers no token while its provider is not the configured auth provider yet', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ authProviderConfigured: false }) })
		});

		await expect
			.element(page.getByRole('alert').getByText(/not the configured auth provider yet/))
			.toBeVisible();
		await expect
			.element(
				page.getByText('The token can be generated once Okta is the configured auth provider.')
			)
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Generate token' }))
			.not.toBeInTheDocument();
	});

	it('does not offer to replace the token of a connection whose provider is not configured', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: reviewAtEnforce({
				connection: connection({
					hasToken: true,
					previousTokenAccepted: true,
					authProviderConfigured: false
				})
			})
		});

		await expect
			.element(
				page.getByRole('alert').getByText(/its\s+token\s+cannot\s+be\s+rotated\s+or\s+replaced/)
			)
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Revoke previous token' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Rotate token' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Revoke current token' }))
			.not.toBeInTheDocument();
	});

	it('lets the bootstrap user manage the token but not enforce', async () => {
		// The server tells the bootstrap user why they cannot enforce.
		const blocker =
			'Only an Owner who signed in through Okta can enforce SCIM. The bootstrap user cannot.';
		await renderScimView(
			[Group.OWNER, Group.ADMIN],
			{ review: reviewAtEnforce({ enforceBlockers: [blocker] }) },
			true
		);

		await expect.element(page.getByRole('button', { name: 'Rotate token' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
		await expect.element(page.getByText(blocker)).toBeVisible();
	});

	it('does not let the bootstrap user enforce, even without a blocker from the server', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], { review: reviewAtEnforce() }, true);

		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
	});

	it('shows administrators the connection without token management', async () => {
		await renderScimView([Group.ADMIN], { review: reviewAtEnforce() });

		await expect
			.element(page.getByRole('button', { name: 'Rotate token' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
	});

	describe('the bearer token expiry', () => {
		const day = 24 * 60 * 60 * 1000;
		const reviewWithTokenExpiringIn = (ms: number) =>
			enforcedReview({
				connection: connection({
					state: 'enforced',
					hasToken: true,
					tokenIssuedAt: new Date(Date.now() + ms - 365 * day).toISOString(),
					tokenExpiresAt: new Date(Date.now() + ms).toISOString()
				})
			});

		it('is shown without a warning while far off', async () => {
			await renderScimView([Group.OWNER], { review: reviewWithTokenExpiringIn(200 * day) });

			await expect.element(page.getByText(/^Expires /)).toBeVisible();
			await expect.element(page.getByText(/The bearer token expires on/)).not.toBeInTheDocument();
			await expect.element(page.getByText(/The bearer token expired on/)).not.toBeInTheDocument();
		});

		it('warns within 30 days of it', async () => {
			await renderScimView([Group.OWNER], { review: reviewWithTokenExpiringIn(10 * day) });

			await expect
				.element(page.getByRole('status').filter({ hasText: /The bearer token expires on/ }))
				.toBeVisible();
			await expect.element(page.getByText(/The bearer token expired on/)).not.toBeInTheDocument();
		});

		it('says that requests fail once it has passed', async () => {
			await renderScimView([Group.OWNER], { review: reviewWithTokenExpiringIn(-day) });

			await expect
				.element(page.getByRole('alert').filter({ hasText: /The bearer token expired on/ }))
				.toBeVisible();
			await expect.element(page.getByText(/^Expired /)).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Rotate token' })).toBeVisible();
		});

		it('does not ask to rotate an expired token while the provider is not configured', async () => {
			const review = reviewWithTokenExpiringIn(-day);
			await renderScimView([Group.OWNER], {
				review: { ...review, connection: { ...review.connection, authProviderConfigured: false } }
			});

			await expect
				.element(page.getByRole('alert').getByText(/its\s+token\s+cannot\s+be\s+rotated/))
				.toBeVisible();
			await expect.element(page.getByText(/Rotate the token/)).not.toBeInTheDocument();
		});
	});

	it('asks for the referenced groups to be pushed before enforcing', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: reviewAtEnforce({
				connection: connection({ origin: 'migrated', hasToken: true }),
				unboundReferencedGroups: {
					items: [
						group('okta/00g00000000000legacy', 'Legacy', {
							consoleURL: 'https://example-admin.okta.com/admin/group/00g00000000000legacy',
							references: [{ kind: 'modelAccessPolicy', id: 'map1', displayName: 'Models' }]
						})
					],
					total: 1
				},
				boundGroups: { items: [group('okta/00g000000000support', 'Support')], total: 1 }
			})
		});

		await expect
			.element(page.getByRole('heading', { name: 'Finish moving to SCIM' }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Push the referenced groups' }))
			.toBeVisible();
		await expect.element(page.getByText('1 referenced group not pushed yet.')).toBeVisible();
		await expect.element(page.getByText('Legacy', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Referenced by LLM gateway profile “Models”'))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Open in Okta' }))
			.toHaveAttribute('href', 'https://example-admin.okta.com/admin/group/00g00000000000legacy');
		await expect.element(page.getByText('Pushed groups (1)')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Next' })).toBeDisabled();
		await expect
			.element(page.getByRole('button', { name: 'Enforce SCIM' }))
			.not.toBeInTheDocument();
	});

	describe('a OneLogin connection', () => {
		function oneLoginConnection(overrides: Partial<SCIMConnection> = {}): SCIMConnection {
			return connection({
				adapterType: 'onelogin',
				authProviderName: 'onelogin-auth-provider',
				authProviderDisplayName: 'OneLogin',
				issuer: 'https://acme.onelogin.com/oidc/2',
				...overrides
			});
		}

		it('says how to set up the SCIM app before anyone is given access to it', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: review({ connection: oneLoginConnection({ hasToken: true }) })
			});

			await expect
				.element(page.getByRole('heading', { name: 'Create the SCIM app in OneLogin' }))
				.toBeVisible();
			await expect
				.element(page.getByText(/add a "SCIM Provisioner with SAML \(SCIM v2 Core\)" app/))
				.toBeVisible();
			await expect.element(page.getByText(/before you give anyone access to it/)).toBeVisible();
			await expect
				.element(page.getByText('https://obot.example.com/scim-provisioning', { exact: true }))
				.toBeVisible();
			await expect
				.element(
					page.getByText(
						'Configuration tab: add "externalId": "{$user.id}" to the SCIM JSON Template, after userName, and leave the "userName": "{$parameters.scimusername}" line as it is.',
						{ exact: false }
					)
				)
				.toBeVisible();
			await expect.element(page.getByText(/Info tab: turn off Visible in portal/)).toBeVisible();
			await expect
				.element(page.getByText(/Parameters tab: set SCIM Username to OneLogin ID\./))
				.toBeVisible();
			await expect
				.element(
					page.getByText(
						/"When users are deleted in OneLogin, or the user's app access is removed" and "When user accounts are suspended in OneLogin" to Suspend\./
					)
				)
				.toBeVisible();
			await expect.element(page.getByText('Waiting for OneLogin to send a request.')).toBeVisible();
			await expect
				.element(page.getByText(/create a SCIM 2\.0 application/))
				.not.toBeInTheDocument();
		});

		it('says how OneLogin pushes roles as groups', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewAtEnforce({
					connection: oneLoginConnection({ origin: 'migrated', hasToken: true }),
					unboundReferencedGroups: {
						items: [
							group('onelogin/1113742', 'obot-scim-test', {
								nativeID: '1113742',
								consoleURL: 'https://acme.onelogin.com/roles/1113742/edit',
								references: [{ kind: 'modelAccessPolicy', id: 'map1', displayName: 'Models' }]
							})
						],
						total: 1
					}
				})
			});

			await expect
				.element(page.getByRole('heading', { name: 'Push the referenced groups' }))
				.toBeVisible();
			await expect.element(page.getByText(/OneLogin pushes roles as groups\./)).toBeVisible();
			await expect
				.element(page.getByText(/with a pattern that has no capture group/))
				.toBeVisible();
			await expect
				.element(page.getByRole('link', { name: 'Open in OneLogin' }))
				.toHaveAttribute('href', 'https://acme.onelogin.com/roles/1113742/edit');
		});

		it("leaves Okta's help as it is", async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: review({ connection: connection({ hasToken: true }) })
			});

			await expect.element(page.getByText(/In Okta, create a SCIM 2\.0 application/)).toBeVisible();
			await expect.element(page.getByText(/SCIM Provisioner with SAML/)).not.toBeInTheDocument();
		});
	});

	describe('a Microsoft Entra connection', () => {
		const objectID = '00000000-0000-4000-8000-000000000011';

		function entraConnection(overrides: Partial<SCIMConnection> = {}): SCIMConnection {
			return connection({
				adapterType: 'entra',
				authProviderName: 'entra-auth-provider',
				authProviderDisplayName: 'Microsoft Entra',
				issuer: '00000000-0000-4000-8000-0000000000aa',
				...overrides
			});
		}

		// A time the given number of minutes ago.
		function minutesAgo(minutes: number) {
			return new Date(Date.now() - minutes * 60 * 1000).toISOString();
		}

		it('says how to set up provisioning on the enterprise app, with the objectId mapping', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: review({ connection: entraConnection({ hasToken: true }) })
			});

			await expect
				.element(page.getByText(/open the enterprise app that Obot signs in with/))
				.toBeVisible();
			await expect
				.element(page.getByText(/An app created under App registrations can't be provisioned to/))
				.toBeVisible();
			await expect
				.element(page.getByText(/Finish every step below before you select Start provisioning/))
				.toBeVisible();
			await expect
				.element(page.getByText(/select Test Connection, then select Create\./))
				.toBeVisible();
			await expect
				.element(page.getByText(/change the source of externalId from mailNickname to objectId/))
				.toBeVisible();
			await expect
				.element(page.getByText(/clear Delete under Target object actions/))
				.toBeVisible();
			await expect.element(page.getByText(/Sync only assigned users and groups/)).toBeVisible();
			// The note on quarantine is shown once, with the activity.
			await expect
				.element(page.getByText(/Entra may have quarantined the provisioning job/))
				.toBeVisible();
			await expect
				.element(page.getByText('Waiting for Microsoft Entra to send a request.'))
				.toBeVisible();
			await expect
				.element(page.getByText(/create a SCIM 2\.0 application/))
				.not.toBeInTheDocument();
			await expect.element(page.getByText(/SCIM Provisioner with SAML/)).not.toBeInTheDocument();
		});

		it('says how Entra pushes groups, with a link to each in the admin center', async () => {
			const consoleURL = `https://entra.microsoft.com/#view/Microsoft_AAD_IAM/GroupDetailsMenuBlade/~/Overview/groupId/${objectID}`;
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewAtEnforce({
					connection: entraConnection({ origin: 'migrated', hasToken: true }),
					unboundReferencedGroups: {
						items: [
							group(`entra/${objectID}`, 'Engineering', {
								nativeID: objectID,
								consoleURL,
								references: [{ kind: 'modelAccessPolicy', id: 'map1', displayName: 'Models' }]
							})
						],
						total: 1
					}
				})
			});

			await expect
				.element(page.getByText(/Assign the groups listed below to the enterprise app/))
				.toBeVisible();
			await expect
				.element(page.getByText(/Provision on demand pushes one group immediately/))
				.toBeVisible();
			await expect
				.element(page.getByRole('link', { name: 'Open in Microsoft Entra' }))
				.toHaveAttribute('href', consoleURL);
			await expect
				.element(page.getByText(/OneLogin pushes roles as groups/))
				.not.toBeInTheDocument();
		});

		// Entra groups bind by object ID, so a migrated connection does not ask for renames.
		it('does not ask to rename the referenced groups of a migrated Entra connection', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewAtEnforce({
					connection: entraConnection({ origin: 'migrated', hasToken: true }),
					unboundReferencedGroups: {
						items: [group(`entra/${objectID}`, 'Engineering', { nativeID: objectID })],
						total: 1
					}
				})
			});

			await expect
				.element(page.getByText(/its name in Microsoft Entra doesn't need to match/))
				.toBeVisible();
			await expect.element(page.getByText(/under exactly the name shown/)).not.toBeInTheDocument();
		});

		it('asks a new Entra connection for the groups to grant roles and policies to', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewAtEnforce({
					connection: entraConnection({ hasToken: true }),
					unboundReferencedGroups: {
						items: [group(`entra/${objectID}`, 'Engineering', { nativeID: objectID })],
						total: 1
					}
				})
			});

			await expect.element(page.getByText(/should be granted to/)).toBeVisible();
			await expect.element(page.getByText(/doesn't need to match/)).not.toBeInTheDocument();
		});

		it('does not ask to rename the referenced groups of an enforced Entra connection', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: enforcedReview({
					connection: entraConnection({ origin: 'migrated', state: 'enforced', hasToken: true }),
					unboundReferencedGroups: {
						items: [group(`entra/${objectID}`, 'Engineering', { nativeID: objectID })],
						total: 1
					}
				})
			});

			await expect
				.element(page.getByText(/Push each group from Microsoft Entra, or remove its references/))
				.toBeVisible();
			await expect.element(page.getByText(/renaming it there first/)).not.toBeInTheDocument();
		});

		it('warns once Entra has sent no request for longer than a running job goes without one', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: enforcedReview({
					connection: entraConnection({ state: 'enforced', hasToken: true }),
					activity: {
						lastRequestAt: minutesAgo(120),
						recentFailures: { items: [], total: 0 }
					}
				})
			});

			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.toBeVisible();
		});

		// Test Connection sends a request before the provisioning job has been started.
		it('does not warn about silence before Entra has provisioned anything', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: review({
					connection: entraConnection({ hasToken: true }),
					activity: {
						lastRequestAt: minutesAgo(120),
						recentFailures: { items: [], total: 0 }
					}
				})
			});

			await expect.element(page.getByRole('heading', { name: 'Activity' })).toBeVisible();
			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.not.toBeInTheDocument();
		});

		it('does not warn while Entra keeps sending requests', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: enforcedReview({
					connection: entraConnection({ state: 'enforced', hasToken: true }),
					activity: {
						lastRequestAt: minutesAgo(30),
						recentFailures: { items: [], total: 0 }
					}
				})
			});

			await expect.element(page.getByRole('heading', { name: 'Activity' })).toBeVisible();
			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.not.toBeInTheDocument();
			// The note on quarantine stays with the activity, which a quarantined job stops.
			await expect.element(page.getByText(/Entra may have quarantined/)).toBeVisible();
		});

		// Renders an enforced Entra connection whose last request was 80 minutes ago, with the clock and
		// the page's timer under the test's control, and the review loaded again with the given last
		// request. The failures are those at each offset, and the first page of them is shown first.
		async function renderEntraLeftOpen(
			reloadedLastRequestAt: () => string,
			{
				failures = () => ({ items: [], total: 0 }),
				pageSize
			}: {
				failures?: (offset: number) => SCIMPage<SCIMRequestFailure>;
				pageSize?: number;
			} = {}
		) {
			vi.useFakeTimers({
				toFake: ['setInterval', 'clearInterval', 'Date']
			});
			onTestFinished(() => {
				vi.useRealTimers();
			});
			worker.use(
				http.get(`*/api/scim-connections/${connectionID}/review`, () =>
					HttpResponse.json(
						enforcedReview({
							connection: entraConnection({ state: 'enforced', hasToken: true }),
							activity: {
								lastRequestAt: reloadedLastRequestAt(),
								recentFailures: failures(0)
							}
						})
					)
				),
				http.get(`*/api/scim-connections/${connectionID}/failures`, ({ request }) =>
					HttpResponse.json(failures(Number(new URL(request.url).searchParams.get('offset'))))
				)
			);
			const lastRequestAt = minutesAgo(80);
			await renderScimView([Group.OWNER, Group.ADMIN], {
				pageSize,
				review: enforcedReview({
					connection: entraConnection({ state: 'enforced', hasToken: true }),
					activity: {
						lastRequestAt,
						recentFailures: failures(0)
					}
				})
			});
			await expect.element(page.getByRole('heading', { name: 'Activity' })).toBeVisible();
			return lastRequestAt;
		}

		it('loads the activity again before warning on a page left open', async () => {
			const reload = vi.spyOn(AdminService, 'getSCIMConnectionReview');
			onTestFinished(() => reload.mockRestore());
			await renderEntraLeftOpen(() => minutesAgo(5));

			await vi.advanceTimersByTimeAsync(10 * 60 * 1000);
			expect(reload).not.toHaveBeenCalled();

			// Entra kept sending requests while the page was open, so it is not silent.
			await vi.advanceTimersByTimeAsync(60 * 1000);
			await expect.element(page.getByText('5 minutes ago', { exact: true })).toBeVisible();
			expect(reload).toHaveBeenCalledOnce();
			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.not.toBeInTheDocument();
		});

		it('warns on a page left open once Entra has still sent nothing', async () => {
			let lastRequestAt = '';
			lastRequestAt = await renderEntraLeftOpen(() => lastRequestAt);

			await vi.advanceTimersByTimeAsync(11 * 60 * 1000);
			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.toBeVisible();
		});

		it('clears the warning on a page left open once Entra sends a request again', async () => {
			let lastRequestAt = '';
			lastRequestAt = await renderEntraLeftOpen(() => lastRequestAt);
			await vi.advanceTimersByTimeAsync(11 * 60 * 1000);
			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.toBeVisible();

			// The admin restarts provisioning, and Entra sends a request.
			lastRequestAt = new Date().toISOString();
			await vi.advanceTimersByTimeAsync(5 * 60 * 1000);
			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.not.toBeInTheDocument();
		});

		it('shows the failures loaded again with the activity on a page left open', async () => {
			let failed = false;
			await renderEntraLeftOpen(() => minutesAgo(5), {
				failures: () =>
					failed
						? {
								items: [failure('/Users/u-1', 'externalId identifies the user and cannot change')],
								total: 1
							}
						: { items: [], total: 0 }
			});
			await expect.element(page.getByText('No recent failures.')).toBeVisible();
			failed = true;

			// Entra's requests since the page was opened include a failure.
			await vi.advanceTimersByTimeAsync(11 * 60 * 1000);
			await expect.element(page.getByText('PATCH /Users/u-1')).toBeVisible();
			await expect
				.element(page.getByText('externalId identifies the user and cannot change'))
				.toBeVisible();
			await expect.element(page.getByText('No recent failures.')).not.toBeInTheDocument();
		});

		it('shows the last page of failures on a page left open once the page shown aged out', async () => {
			let agedOut = false;
			const all = [failure('/Users/u-1'), failure('/Users/u-2'), failure('/Users/u-3')];
			await renderEntraLeftOpen(() => minutesAgo(5), {
				pageSize: 2,
				failures: (offset) => {
					const shown = agedOut ? all.slice(0, 2) : all;
					return { items: shown.slice(offset, offset + 2), total: shown.length };
				}
			});
			await page.getByRole('button', { name: 'Next page of recent failures' }).click();
			await expect.element(page.getByText('PATCH /Users/u-3')).toBeVisible();

			// The third failure ages out, so the second page is empty.
			agedOut = true;
			await vi.advanceTimersByTimeAsync(11 * 60 * 1000);
			await expect.element(page.getByText('PATCH /Users/u-1')).toBeVisible();
			await expect.element(page.getByText('PATCH /Users/u-3')).not.toBeInTheDocument();
		});

		it('keeps a page of failures asked for just before the activity is loaded again', async () => {
			const all = [failure('/Users/u-1'), failure('/Users/u-2'), failure('/Users/u-3')];
			const failuresAt = (offset: number) => ({ items: all.slice(offset, offset + 2), total: 3 });
			await renderEntraLeftOpen(() => minutesAgo(5), {
				pageSize: 2,
				failures: failuresAt
			});
			let releasePage!: () => void;
			const pageHeld = new Promise<void>((resolve) => (releasePage = resolve));
			let releaseReview!: () => void;
			const reviewHeld = new Promise<void>((resolve) => (releaseReview = resolve));
			worker.use(
				http.get(`*/api/scim-connections/${connectionID}/failures`, async ({ request }) => {
					const offset = Number(new URL(request.url).searchParams.get('offset'));
					if (offset === 2) await pageHeld;
					return HttpResponse.json(failuresAt(offset));
				}),
				http.get(`*/api/scim-connections/${connectionID}/review`, async () => {
					await reviewHeld;
					return HttpResponse.json(
						enforcedReview({
							connection: entraConnection({ state: 'enforced', hasToken: true }),
							activity: {
								lastRequestAt: minutesAgo(5),
								recentFailures: failuresAt(0)
							}
						})
					);
				})
			);

			// The admin asks for the second page, and the activity is loaded again before it arrives.
			await page.getByRole('button', { name: 'Next page of recent failures' }).click();
			await vi.advanceTimersByTimeAsync(11 * 60 * 1000);
			releasePage();
			await expect.element(page.getByText('PATCH /Users/u-3')).toBeVisible();

			// The activity arrives last, and the second page stays.
			releaseReview();
			await expect.element(page.getByText('5 minutes ago', { exact: true })).toBeVisible();
			await tick();
			await expect.element(page.getByText('PATCH /Users/u-3')).toBeVisible();
			await expect.element(page.getByText('PATCH /Users/u-1')).not.toBeInTheDocument();
		});

		it('warns on a page left open when Entra has sent nothing, even if its failures fail to load', async () => {
			let lastRequestAt = '';
			lastRequestAt = await renderEntraLeftOpen(() => lastRequestAt);
			worker.use(
				http.get(`*/api/scim-connections/${connectionID}/failures`, () =>
					HttpResponse.json({ error: 'unavailable' }, { status: 500 })
				)
			);

			await vi.advanceTimersByTimeAsync(11 * 60 * 1000);
			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.toBeVisible();
		});

		it('does not warn about silence from another identity provider', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: enforcedReview({
					activity: {
						lastRequestAt: minutesAgo(120),
						recentFailures: { items: [], total: 0 }
					}
				})
			});

			await expect.element(page.getByRole('heading', { name: 'Activity' })).toBeVisible();
			await expect
				.element(page.getByText(/a running provisioning job sends one every 20 to 40 minutes/))
				.not.toBeInTheDocument();
			await expect.element(page.getByText(/may have quarantined/)).not.toBeInTheDocument();
		});

		it('warns before enabling that pushed groups lose members until Entra adds them', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				enablePreview: enablePreview({
					authProviderName: 'entra-auth-provider',
					authProviderDisplayName: 'Microsoft Entra'
				})
			});

			await expect
				.element(page.getByRole('heading', { name: 'Move Microsoft Entra to SCIM' }))
				.toBeVisible();
			await expect
				.element(page.getByText(/Groups with many members will take longer to push/))
				.toBeVisible();
			await expect
				.element(page.getByText(/Provision on demand does not shorten this/))
				.toBeVisible();
			await expect
				.element(page.getByText(/Access that came through a nested group ends/))
				.toBeVisible();
		});

		it('does not warn about Entra before enabling SCIM for another provider', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], { enablePreview: enablePreview() });

			await expect.element(page.getByRole('heading', { name: 'Move Okta to SCIM' })).toBeVisible();
			await expect
				.element(page.getByText(/Groups with many members will take longer to push/))
				.not.toBeInTheDocument();
		});
	});

	describe('offering to delete the unreferenced groups that keep a push from binding', () => {
		// The groups step of a migrated connection with a warning of the given type, and that many
		// unreferenced groups.
		function reviewWithWarning(type: SCIMSetupWarning['type'], unreferenced: number) {
			return reviewAtEnforce({
				connection: connection({ origin: 'migrated', hasToken: true }),
				unboundReferencedGroups: {
					items: [group('okta/00g000000engineering', 'Engineering')],
					total: 1
				},
				unreferencedGroups: {
					items: Array.from({ length: unreferenced }, (_, i) =>
						group(`okta/00g00000000000000${i}`, 'engineering')
					),
					total: unreferenced
				},
				warnings: [
					{
						type,
						message: 'A group pushed under the name "engineering" binds to no group.',
						groupID: 'okta/00g000000engineering',
						groupName: 'engineering'
					}
				]
			});
		}
		const deleteButton = () => page.getByRole('button', { name: 'Delete unreferenced groups' });

		it('offers it to an Owner when there is a group to delete', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewWithWarning('unreferencedNamesake', 1)
			});

			await expect
				.element(page.getByText('A group pushed under the name "engineering" binds to no group.'))
				.toBeVisible();
			await expect.element(deleteButton()).toBeVisible();
		});

		it('does not offer it when there is no group to delete', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewWithWarning('duplicateName', 0)
			});

			await expect
				.element(page.getByText('A group pushed under the name "engineering" binds to no group.'))
				.toBeVisible();
			await expect.element(deleteButton()).not.toBeInTheDocument();
		});

		it('does not offer it to administrators who are not Owners', async () => {
			await renderScimView([Group.ADMIN], { review: reviewWithWarning('unreferencedNamesake', 1) });

			await expect
				.element(page.getByText('A group pushed under the name "engineering" binds to no group.'))
				.toBeVisible();
			await expect.element(deleteButton()).not.toBeInTheDocument();
		});

		it('offers it for referenced groups that share a name, when there is a group to delete', async () => {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewWithWarning('duplicateName', 1)
			});

			await expect.element(deleteButton()).toBeVisible();
		});

		// Entra binds groups by object ID, so deleting unreferenced groups does not resolve duplicate names.
		it('does not offer it for referenced Entra groups that share a name', async () => {
			const message =
				'2 referenced groups are named "Engineering", and only one group with that name can be pushed from Microsoft Entra.';
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewAtEnforce({
					connection: connection({
						origin: 'migrated',
						hasToken: true,
						adapterType: 'entra',
						authProviderName: 'entra-auth-provider',
						authProviderDisplayName: 'Microsoft Entra'
					}),
					unboundReferencedGroups: {
						items: [
							group('entra/00000000-0000-4000-8000-000000000001', 'Engineering'),
							group('entra/00000000-0000-4000-8000-000000000002', 'Engineering')
						],
						total: 2
					},
					unreferencedGroups: {
						items: [group('entra/00000000-0000-4000-8000-000000000003', 'Sales')],
						total: 1
					},
					warnings: [
						{
							type: 'duplicateName',
							message,
							groupID: 'entra/00000000-0000-4000-8000-000000000001',
							groupName: 'Engineering'
						}
					]
				})
			});

			await expect.element(page.getByText(message)).toBeVisible();
			await expect.element(deleteButton()).not.toBeInTheDocument();
		});

		it.each(['everyoneGroup', 'missingGroup'] as const)(
			'does not offer it for a %s warning, which deleting does not resolve',
			async (type) => {
				await renderScimView([Group.OWNER, Group.ADMIN], { review: reviewWithWarning(type, 1) });

				await expect
					.element(page.getByText('A group pushed under the name "engineering" binds to no group.'))
					.toBeVisible();
				await expect.element(deleteButton()).not.toBeInTheDocument();
			}
		);

		it('deletes them, after which the warning is gone', async () => {
			const deleteGroups = vi.fn();
			worker.use(
				http.post(`*/api/scim-connections/${connectionID}/delete-unreferenced-groups`, () => {
					deleteGroups();
					return HttpResponse.json({ deletedGroupCount: 1 });
				}),
				http.get(`*/api/scim-connections/${connectionID}/review`, () =>
					// The namesake is gone, so a push of the referenced group can bind to it.
					HttpResponse.json({ ...reviewWithWarning('unreferencedNamesake', 0), warnings: [] })
				)
			);
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: reviewWithWarning('unreferencedNamesake', 1)
			});

			await deleteButton().click();
			await expect
				.element(page.getByText('Delete 1 unreferenced group?', { exact: true }))
				.toBeVisible();
			await page.getByRole('button', { name: 'Delete groups' }).click();

			await vi.waitFor(() => expect(deleteGroups).toHaveBeenCalledOnce());
			await expect.element(page.getByText('Deleted 1 unreferenced group.')).toBeVisible();
			await expect
				.element(page.getByText('A group pushed under the name "engineering" binds to no group.'))
				.not.toBeInTheDocument();
			await expect.element(deleteButton()).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('heading', { name: 'Push the referenced groups' }))
				.toBeVisible();
		});

		it('offers it once, in the report of a failed deletion, while there is one', async () => {
			const deletionError =
				'SCIM is enabled, but the groups that nothing references could not be deleted.';
			const enabled = reviewWithWarning('unreferencedNamesake', 1);
			worker.use(
				http.post('*/api/scim-connections', () =>
					HttpResponse.json({
						connection: { ...enabled.connection, token: 'obot_scim_enabled' },
						deletedGroupCount: 0,
						deletionError
					})
				),
				http.get(`*/api/scim-connections/${connectionID}/review`, () => HttpResponse.json(enabled))
			);
			await renderScimView([Group.OWNER, Group.ADMIN], { enablePreview: enablePreview() });

			await confirmEnableSCIM();
			await page.getByRole('button', { name: 'Done' }).click();

			await expect
				.element(page.getByText('A group pushed under the name "engineering" binds to no group.'))
				.toBeVisible();
			await expect
				.element(
					page
						.getByRole('alert')
						.filter({ hasText: deletionError })
						.getByRole('button', { name: 'Delete unreferenced groups' })
				)
				.toBeVisible();
			expect(deleteButton().elements()).toHaveLength(1);
		});
	});

	it('does not mention disabling users when every user is provisioned', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: reviewAtEnforce({ unprovisionedUsers: { items: [], total: 0 } })
		});

		await expect
			.element(
				page.getByText('Once SCIM is enforced, only accounts that Okta provisioned can sign in.', {
					exact: false
				})
			)
			.toBeVisible();
		await page.getByRole('button', { name: 'Enforce SCIM' }).click();
		await expect
			.element(
				page.getByText(
					'This cannot be undone. Only provisioned users can sign in with Okta afterwards.'
				)
			)
			.toBeVisible();
		await expect
			.element(page.getByRole('group', { name: 'Enforce SCIM' }))
			.not.toHaveTextContent('disable');
	});

	it('enforces SCIM after confirmation', async () => {
		const enforce = vi.fn();
		worker.use(
			http.post(`*/api/scim-connections/${connectionID}/enforce`, () => {
				enforce();
				return HttpResponse.json({
					connection: connection({ state: 'enforced' }),
					disabledUserCount: 1,
					deletedGroupCount: 2
				});
			}),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(enforcedReview())
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { review: reviewAtEnforce() });

		await expect.element(page.getByText('Ready to enforce SCIM.')).toBeVisible();
		await expect
			.element(
				page.getByText('Enforcing disables the 1 user that it has not provisioned.', {
					exact: false
				})
			)
			.toBeVisible();
		await page.getByRole('button', { name: 'Enforce SCIM' }).click();
		await expect
			.element(
				page.getByText('Enforcing disables 1 user that Okta has not provisioned', { exact: false })
			)
			.toBeVisible();
		await page.getByRole('button', { name: 'Enforce SCIM' }).last().click();

		await vi.waitFor(() => expect(enforce).toHaveBeenCalledOnce());
		await expect
			.element(page.getByText('Disabled 1 unprovisioned user', { exact: false }))
			.toBeVisible();
		await expect.element(page.getByText('Enforced', { exact: true }).first()).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Enforce SCIM' }))
			.not.toBeInTheDocument();
	});

	it('leaves out the users not provisioned once SCIM is enforced, when there are none', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: enforcedReview({ unprovisionedUsers: { items: [], total: 0 } })
		});

		await expect.element(page.getByText('Provisioned (1)')).toBeVisible();
		await expect
			.element(page.getByText('Not provisioned', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('lists the pushed groups without their references once SCIM is enforced', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: enforcedReview({
				boundGroups: {
					items: [
						group('okta/00g000000000support', 'Support', {
							references: [{ kind: 'modelAccessPolicy', id: 'map1', displayName: 'Models' }]
						})
					],
					total: 1
				}
			})
		});

		await expect.element(page.getByText('Support', { exact: true })).toBeVisible();
		await expect.element(page.getByText('okta/00g000000000support')).toBeVisible();
		await expect.element(page.getByText(/^Referenced by/)).not.toBeInTheDocument();
	});

	it('lists the references of a removed group only in its warning', async () => {
		const message =
			'The group with ID okta/00g0000000000missing was removed but is still referenced by the following vMCP profiles: sales (Gmail)';
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: enforcedReview({
				warnings: [
					{
						type: 'missingGroup',
						message,
						groupID: 'okta/00g0000000000missing',
						references: [
							{ kind: 'vmcpProfile', id: 'vmcp1', displayName: 'Gmail', detail: 'profile sales' }
						]
					}
				]
			})
		});

		await expect.element(page.getByText(message)).toBeVisible();
		await expect.element(page.getByText(/^Referenced by/)).not.toBeInTheDocument();
	});

	it('pages through long lists', async () => {
		const requested = vi.fn();
		worker.use(
			http.get(`*/api/scim-connections/${connectionID}/users`, ({ request }) => {
				const url = new URL(request.url);
				requested(url.searchParams.get('provisioned'), url.searchParams.get('offset'));
				return HttpResponse.json({ items: [user('3'), user('4')], total: 4 });
			})
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			pageSize: 2,
			review: enforcedReview({
				unprovisionedUsers: { items: [user('1'), user('2')], total: 4 }
			})
		});

		await expect.element(page.getByText('1 of 2', { exact: true })).toBeVisible();
		await page.getByRole('button', { name: 'Next page of unprovisioned users' }).click();

		await vi.waitFor(() => expect(requested).toHaveBeenCalledWith('false', '2'));
		await expect.element(page.getByText('User 3')).toBeVisible();
		await expect.element(page.getByText('2 of 2', { exact: true })).toBeVisible();
	});

	it('keeps the page asked for last when an earlier request answers late', async () => {
		let releaseFirst!: () => void;
		const firstHeld = new Promise<void>((resolve) => (releaseFirst = resolve));
		let requests = 0;
		worker.use(
			http.get(`*/api/scim-connections/${connectionID}/users`, async () => {
				requests++;
				if (requests === 1) {
					// The first request answers only once the test lets it, after the second one.
					await firstHeld;
					return HttpResponse.json({ items: [user('late')], total: 4 });
				}
				return HttpResponse.json({ items: [user('3'), user('4')], total: 4 });
			})
		);
		const listUsers = vi.spyOn(AdminService, 'listSCIMUsers');
		try {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				pageSize: 2,
				review: enforcedReview({
					unprovisionedUsers: { items: [user('1'), user('2')], total: 4 }
				})
			});

			const next = page.getByRole('button', { name: 'Next page of unprovisioned users' });
			await next.click();
			await next.click();
			await expect.element(page.getByText('User 3')).toBeVisible();

			// The view handles the late answer before this test does, so once it resolves here, it was dropped.
			releaseFirst();
			await listUsers.mock.results[0].value;
			await tick();
			await expect.element(page.getByText('User late')).not.toBeInTheDocument();
			await expect.element(page.getByText('User 3')).toBeVisible();
		} finally {
			listUsers.mockRestore();
		}
	});

	it('shows a failed token action inline rather than as a notification', async () => {
		errors.items = [];
		worker.use(
			http.post(`*/api/scim-connections/${connectionID}/rotate-token`, () =>
				HttpResponse.json({ error: 'the token could not be issued' }, { status: 500 })
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { review: review() });

		await page.getByRole('button', { name: 'Generate token' }).click();
		await page.getByRole('button', { name: 'Generate token' }).last().click();

		await expect
			.element(page.getByRole('alert').filter({ hasText: /could not be issued/ }))
			.toBeVisible();
		expect(errors.items).toHaveLength(0);
	});

	it('returns to the last page when the page shown has emptied', async () => {
		const requested = vi.fn();
		worker.use(
			http.get(`*/api/scim-connections/${connectionID}/users`, ({ request }) => {
				const offset = new URL(request.url).searchParams.get('offset');
				requested(offset);
				// Two users were provisioned since the review loaded, so only the first page is left.
				return HttpResponse.json(
					offset === '0' ? { items: [user('1'), user('2')], total: 2 } : { items: [], total: 2 }
				);
			})
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			pageSize: 2,
			review: enforcedReview({
				provisionedUsers: { items: [], total: 0 },
				unprovisionedUsers: { items: [user('1'), user('2')], total: 4 }
			})
		});

		await page.getByRole('button', { name: 'Next page of unprovisioned users' }).click();

		await vi.waitFor(() => expect(requested).toHaveBeenCalledWith('0'));
		await expect.element(page.getByText('User 1')).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Next page of unprovisioned users' }))
			.not.toBeInTheDocument();
	});

	it('shows a page that keeps coming back empty, rather than asking for it again forever', async () => {
		const requested = vi.fn();
		worker.use(
			http.get(`*/api/scim-connections/${connectionID}/users`, ({ request }) => {
				requested(new URL(request.url).searchParams.get('offset'));
				// The count says there is a second page, but the page itself is empty.
				return HttpResponse.json({ items: [], total: 4 });
			})
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			pageSize: 2,
			review: enforcedReview({
				provisionedUsers: { items: [], total: 0 },
				unprovisionedUsers: { items: [user('1'), user('2')], total: 4 }
			})
		});

		await page.getByRole('button', { name: 'Next page of unprovisioned users' }).click();

		await expect.element(page.getByText('2 of 2', { exact: true })).toBeVisible();
		expect(requested).toHaveBeenCalledExactlyOnceWith('2');
	});

	it('rotates the token, shows the new one once, and can revoke the previous one', async () => {
		const revokePrevious = vi.fn();
		let rotated = false;
		worker.use(
			http.post(`*/api/scim-connections/${connectionID}/rotate-token`, () => {
				rotated = true;
				return HttpResponse.json(
					connection({ hasToken: true, previousTokenAccepted: true, token: 'obot_scim_rotated' })
				);
			}),
			http.post(`*/api/scim-connections/${connectionID}/revoke-previous-token`, () => {
				revokePrevious();
				return HttpResponse.json(connection({ hasToken: true }));
			}),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(
					review({
						connection: connection({
							hasToken: true,
							previousTokenAccepted: rotated && revokePrevious.mock.calls.length === 0,
							previousTokenExpiresAt: '2026-09-30T00:00:00.000Z'
						})
					})
				)
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ hasToken: true }) })
		});

		await page.getByRole('button', { name: 'Rotate token' }).click();
		await page.getByRole('button', { name: 'Rotate token' }).last().click();
		await expect.element(page.getByText('obot_scim_rotated')).toBeVisible();
		await page.getByRole('button', { name: 'Done' }).click();
		await expect.element(page.getByText('obot_scim_rotated')).not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Revoke previous token' }).click();
		await page.getByRole('button', { name: 'Revoke', exact: true }).click();
		await vi.waitFor(() => expect(revokePrevious).toHaveBeenCalledOnce());
		await expect
			.element(page.getByRole('button', { name: 'Revoke previous token' }))
			.not.toBeInTheDocument();
	});

	it("refreshes the layout's token expiry warning after issuing a token", async () => {
		const listed = vi.fn();
		worker.use(
			http.get('*/api/auth-providers', () => {
				listed();
				return HttpResponse.json({ items: [] });
			}),
			http.post(`*/api/scim-connections/${connectionID}/rotate-token`, () =>
				HttpResponse.json(connection({ hasToken: true, token: 'obot_scim_rotated' }))
			),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(review({ connection: connection({ hasToken: true }) }))
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ hasToken: true }) })
		});
		listed.mockClear();

		await page.getByRole('button', { name: 'Rotate token' }).click();
		await page.getByRole('button', { name: 'Rotate token' }).last().click();
		await expect.element(page.getByText('obot_scim_rotated')).toBeVisible();
		await vi.waitFor(() => expect(listed).toHaveBeenCalledOnce());
	});

	it('closes the confirmation before showing the token it issued', async () => {
		let releaseReview!: () => void;
		const reviewHeld = new Promise<void>((resolve) => (releaseReview = resolve));
		worker.use(
			http.post(`*/api/scim-connections/${connectionID}/rotate-token`, () =>
				HttpResponse.json(connection({ hasToken: true, token: 'obot_scim_new' }))
			),
			http.get(`*/api/scim-connections/${connectionID}/review`, async () => {
				// The review loaded after the rotation answers only once the test lets it.
				await reviewHeld;
				return HttpResponse.json(review({ connection: connection({ hasToken: true }) }));
			})
		);
		try {
			await renderScimView([Group.OWNER, Group.ADMIN], {
				review: review({ connection: connection({ hasToken: true }) })
			});

			await page.getByRole('button', { name: 'Rotate token' }).click();
			await page.getByRole('button', { name: 'Rotate token' }).last().click();

			await expect.element(page.getByText('obot_scim_new')).toBeVisible();
			await expect
				.element(page.getByText('Issue a new SCIM bearer token?'))
				.not.toBeInTheDocument();
		} finally {
			releaseReview();
		}
	});

	it('replaces a leaked token', async () => {
		const revokeCurrent = vi.fn();
		worker.use(
			http.post(`*/api/scim-connections/${connectionID}/revoke-current-token`, () => {
				revokeCurrent();
				return HttpResponse.json(connection({ hasToken: true, token: 'obot_scim_replacement' }));
			}),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(review({ connection: connection({ hasToken: true }) }))
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ hasToken: true }) })
		});

		await page.getByRole('button', { name: 'Revoke current token' }).click();
		await page.getByRole('button', { name: 'Revoke and replace' }).click();

		await vi.waitFor(() => expect(revokeCurrent).toHaveBeenCalledOnce());
		await expect.element(page.getByText('obot_scim_replacement')).toBeVisible();
	});

	it('shows auditors the connection without any action', async () => {
		await renderScimView([Group.USER, Group.AUDITOR], { review: reviewAtEnforce() });

		await expect.element(page.getByRole('heading', { name: 'SCIM provisioning' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Rotate token' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Revoke current token' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
	});

	it('shows what refused Enforce once it fails', async () => {
		const blocker =
			'Your account has not been provisioned through SCIM. Assign yourself to the SCIM application in Okta.';
		worker.use(
			http.post(`*/api/scim-connections/${connectionID}/enforce`, () =>
				HttpResponse.json({ error: `SCIM cannot be enforced:\n- ${blocker}` }, { status: 400 })
			),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(reviewAtEnforce({ enforceBlockers: [blocker] }))
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { review: reviewAtEnforce() });

		await page.getByRole('button', { name: 'Enforce SCIM' }).click();
		await page.getByRole('button', { name: 'Enforce SCIM' }).last().click();

		await expect.element(page.getByText('SCIM cannot be enforced yet')).toBeVisible();
		await expect.element(page.getByRole('listitem').filter({ hasText: blocker })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
	});

	it('explains why SCIM cannot be set up for the configured provider', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			enablePreview: enablePreview({
				authProviderNamespace: undefined,
				authProviderName: undefined,
				authProviderDisplayName: undefined,
				blockers: ['GitHub does not support SCIM provisioning.']
			})
		});

		await expect
			.element(page.getByRole('heading', { name: 'SCIM provisioning is not set up' }))
			.toBeVisible();
		await expect
			.element(page.getByText('GitHub does not support SCIM provisioning.'))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enable SCIM' })).not.toBeInTheDocument();
	});

	it('offers to enable SCIM for a provider that synchronizes its directory', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], { enablePreview: enablePreview() });

		await expect.element(page.getByRole('heading', { name: 'Move Okta to SCIM' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enable SCIM' })).toBeEnabled();
	});

	it('lets only Owners enable SCIM', async () => {
		await renderScimView([Group.ADMIN], { enablePreview: enablePreview() });

		await expect.element(page.getByRole('button', { name: 'Enable SCIM' })).toBeDisabled();
		await expect.element(page.getByText('Only an owner can enable SCIM.')).toBeVisible();
	});

	it('lists what blocks enabling, with the referenced groups that share a name', async () => {
		const blocker = '2 referenced groups are named "Engineering".';
		await renderScimView([Group.OWNER, Group.ADMIN], {
			enablePreview: enablePreview({
				blockers: [blocker],
				duplicateGroupNames: [
					{
						name: 'Engineering',
						groups: [
							group('okta/00g00000000000000eng', 'Engineering', {
								references: [{ kind: 'modelAccessPolicy', id: 'map1', displayName: 'Models' }]
							}),
							group('okta/00g00000engineering', 'engineering ', {
								references: [{ kind: 'groupRoleAssignment', id: 'okta/00g00000engineering' }]
							})
						]
					}
				]
			})
		});

		await expect.element(page.getByText('SCIM cannot be enabled yet')).toBeVisible();
		await expect.element(page.getByText(blocker)).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Referenced groups named "Engineering" (2)' }))
			.toBeVisible();
		await expect.element(page.getByText('okta/00g00000engineering', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Referenced by group role assignment')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enable SCIM' })).toBeDisabled();
	});

	it('enables SCIM after confirmation, shows the token once, and then the migration review', async () => {
		const enable = vi.fn();
		const migrated = connection({ origin: 'migrated', hasToken: true });
		worker.use(
			http.post('*/api/scim-connections', () => {
				enable();
				return HttpResponse.json({
					connection: { ...migrated, token: 'obot_scim_enabled' },
					deletedGroupCount: 1
				});
			}),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(
					review({
						connection: migrated,
						unboundReferencedGroups: {
							items: [group('okta/00g00000000000000eng', 'Engineering')],
							total: 1
						}
					})
				)
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { enablePreview: enablePreview() });

		await page.getByRole('button', { name: 'Enable SCIM' }).click();
		const note = page.getByText('Obot stops fetching groups from Okta at sign-in, so until', {
			exact: false
		});
		await expect.element(note).toBeVisible();
		await expect.element(note).not.toHaveTextContent('unreferenced');
		await page.getByRole('button', { name: 'Enable SCIM' }).last().click();

		await vi.waitFor(() => expect(enable).toHaveBeenCalledOnce());
		await expect.element(page.getByText('obot_scim_enabled')).toBeVisible();
		await expect
			.element(page.getByText('SCIM is enabled for Okta.', { exact: true }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Done' }).click();
		await expect.element(page.getByText('obot_scim_enabled')).not.toBeInTheDocument();

		await expect
			.element(page.getByRole('heading', { name: 'Finish moving to SCIM' }))
			.toBeVisible();
		// Enabling issued the token, so setup continues with the SCIM app.
		await expect
			.element(page.getByRole('heading', { name: 'Create the SCIM app in Okta' }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enable SCIM' })).not.toBeInTheDocument();
	});

	it('shows why enabling failed, and the connection if it was created anyway', async () => {
		worker.use(
			http.post('*/api/scim-connections', () =>
				HttpResponse.json({ error: 'failed to wait for the change' }, { status: 500 })
			),
			http.get('*/api/scim-connections', () =>
				HttpResponse.json({ items: [connection({ origin: 'migrated' })] })
			),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(review({ connection: connection({ origin: 'migrated' }) }))
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { enablePreview: enablePreview() });

		await confirmEnableSCIM();

		await expect
			.element(page.getByRole('alert').filter({ hasText: /failed to wait for the change/ }))
			.toBeVisible();
		// The connection has no token yet, which an Owner generates here.
		await expect.element(page.getByRole('button', { name: 'Generate token' })).toBeVisible();
	});

	it('reports a failed deletion of unreferenced groups, and lets an Owner retry it', async () => {
		const deleteGroups = vi.fn();
		const migrated = connection({ origin: 'migrated', hasToken: true });
		const deletionError =
			'SCIM is enabled, but the groups that nothing references could not be deleted.';
		worker.use(
			http.post('*/api/scim-connections', () =>
				HttpResponse.json({
					connection: { ...migrated, token: 'obot_scim_enabled' },
					deletedGroupCount: 0,
					deletionError
				})
			),
			http.post(`*/api/scim-connections/${connectionID}/delete-unreferenced-groups`, () => {
				deleteGroups();
				return HttpResponse.json({ deletedGroupCount: 1 });
			}),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(
					review({
						connection: migrated,
						// A group is still unreferenced after the retry, so the report clears only because the
						// retry succeeded.
						unreferencedGroups: { items: [group('okta/00g000000000000stale', 'Stale')], total: 1 }
					})
				)
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { enablePreview: enablePreview() });

		await confirmEnableSCIM();
		await page.getByRole('button', { name: 'Done' }).click();
		await expect.element(page.getByText(deletionError)).toBeVisible();

		await page.getByRole('button', { name: 'Delete unreferenced groups' }).click();
		await page.getByRole('button', { name: 'Delete groups' }).click();

		await vi.waitFor(() => expect(deleteGroups).toHaveBeenCalledOnce());
		await expect.element(page.getByText('Deleted 1 unreferenced group.')).toBeVisible();
		await expect.element(page.getByText(deletionError)).not.toBeInTheDocument();
	});

	it('does not let administrators who are not Owners delete unreferenced groups', async () => {
		await renderScimView([Group.ADMIN], {
			review: enforcedReview({
				unreferencedGroups: { items: [group('okta/00g000000000000stale', 'Stale')], total: 1 }
			})
		});

		await expect.element(page.getByText('Unreferenced groups (1)')).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Delete unreferenced groups' }))
			.not.toBeInTheDocument();
	});

	it('stops reporting a failed deletion once enforcing deleted the groups', async () => {
		const migrated = connection({ origin: 'migrated', hasToken: true });
		const deletionError =
			'SCIM is enabled, but the groups that nothing references could not be deleted.';
		let enforced = false;
		worker.use(
			http.post('*/api/scim-connections', () =>
				HttpResponse.json({
					connection: { ...migrated, token: 'obot_scim_enabled' },
					deletedGroupCount: 0,
					deletionError
				})
			),
			http.post(`*/api/scim-connections/${connectionID}/enforce`, () => {
				enforced = true;
				return HttpResponse.json({
					connection: { ...migrated, state: 'enforced' },
					disabledUserCount: 0,
					deletedGroupCount: 1
				});
			}),
			// A group became unreferenced after Enforce, so only Enforce's success clears the report.
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(
					enforced
						? enforcedReview({
								connection: { ...migrated, state: 'enforced' },
								unreferencedGroups: {
									items: [group('okta/00g000000000000later', 'Later')],
									total: 1
								}
							})
						: reviewAtEnforce({
								connection: migrated,
								unreferencedGroups: {
									items: [group('okta/00g000000000000stale', 'Stale')],
									total: 1
								}
							})
				)
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { enablePreview: enablePreview() });

		await confirmEnableSCIM();
		await page.getByRole('button', { name: 'Done' }).click();
		await expect.element(page.getByText(deletionError)).toBeVisible();

		await page.getByRole('button', { name: 'Enforce SCIM' }).click();
		await page.getByRole('button', { name: 'Enforce SCIM' }).last().click();

		// Enforce disabled no one, so the notice does not mention disabling.
		await expect.element(page.getByText('SCIM is enforced.', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Later', { exact: true })).toBeVisible();
		await expect.element(page.getByText(deletionError)).not.toBeInTheDocument();
	});

	it('never offers to enable SCIM again once it is enabled, even when the review fails to load', async () => {
		worker.use(
			http.post('*/api/scim-connections', () =>
				HttpResponse.json({
					connection: connection({
						origin: 'migrated',
						hasToken: true,
						token: 'obot_scim_enabled'
					}),
					deletedGroupCount: 0
				})
			),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json({ error: 'the review could not be loaded' }, { status: 500 })
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { enablePreview: enablePreview() });

		await confirmEnableSCIM();
		await expect.element(page.getByText('obot_scim_enabled')).toBeVisible();
		await page.getByRole('button', { name: 'Done' }).click();

		await expect
			.element(page.getByRole('heading', { name: 'SCIM provisioning is enabled' }))
			.toBeVisible();
		await expect
			.element(page.getByRole('alert').filter({ hasText: /could not be loaded/ }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enable SCIM' })).not.toBeInTheDocument();
	});

	it('asks for a token when enabling SCIM was interrupted before issuing one', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ origin: 'migrated' }) })
		});

		await expect
			.element(page.getByRole('heading', { name: 'Generate the bearer token' }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Generate token' })).toBeVisible();
	});

	it('does not ask a migrated connection that has its token for another', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ origin: 'migrated', hasToken: true }) })
		});

		await expect
			.element(page.getByRole('heading', { name: 'Create the SCIM app in Okta' }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Generate token' }))
			.not.toBeInTheDocument();
	});
});
