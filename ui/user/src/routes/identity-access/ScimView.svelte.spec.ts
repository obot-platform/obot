import { AdminService, Group } from '$lib/services';
import type {
	SCIMConnection,
	SCIMConnectionReview,
	SCIMEnablePreview,
	SCIMSetupGroup,
	SCIMSetupUser
} from '$lib/services/admin/types';
import errors from '$lib/stores/errors.svelte';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import ScimView from './ScimView.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
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
		baseURL: `https://obot.example.com/scim/v2/${connectionID}`,
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

function enablePreview(overrides: Partial<SCIMEnablePreview> = {}): SCIMEnablePreview {
	return {
		authProviderNamespace: 'default',
		authProviderName: 'okta-auth-provider',
		authProviderDisplayName: 'Okta',
		blockers: [],
		duplicateGroupNames: [],
		warnings: [],
		unboundReferencedGroups: {
			items: [
				group('okta/00g00000000000000eng', 'Engineering', {
					consoleURL: 'https://example-admin.okta.com/admin/group/00g00000000000000eng',
					references: [{ kind: 'accessControlRule', id: 'acr1', displayName: 'Servers' }]
				})
			],
			total: 1
		},
		unreferencedGroups: { items: [group('okta/00g000000000000stale', 'Stale')], total: 1 },
		baseURLPrefix: 'https://obot.example.com/scim/v2/',
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

	it('shows the setup checklist of a SCIM-first connection', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({
				connection: connection({ hasToken: true, tokenIssuedAt: '2026-09-02T00:00:00.000Z' }),
				provisionedUsers: { items: [user('1', { scimID: 'scim-1', active: true })], total: 1 },
				activity: {
					lastRequestAt: '2026-09-03T00:00:00.000Z',
					recentFailures: { items: [], total: 0 }
				}
			})
		});

		const checklist = page.getByRole('list').filter({ hasText: 'Generate the token' });
		await expect.element(checklist.getByLabelText('Done')).toHaveLength(3);
		await expect.element(checklist.getByLabelText('Not done')).toHaveLength(2);
		await expect.element(page.getByText('1 provisioned, 1 not provisioned yet.')).toBeVisible();
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
		await expect.element(page.getByText('It is shown only once', { exact: false })).toBeVisible();
	});

	it('lets the bootstrap user manage the token but not enforce', async () => {
		// The server tells the bootstrap user why they cannot enforce.
		const blocker =
			'Only an Owner who signed in through Okta can enforce SCIM. The bootstrap user cannot.';
		await renderScimView(
			[Group.OWNER, Group.ADMIN],
			{ review: review({ enforceBlockers: [blocker] }) },
			true
		);

		await expect.element(page.getByRole('button', { name: 'Generate token' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
		await expect.element(page.getByText(blocker)).toBeVisible();
	});

	it('does not let the bootstrap user enforce, even without a blocker from the server', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], { review: review() }, true);

		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
	});

	it('shows administrators the connection without token management', async () => {
		await renderScimView([Group.ADMIN], {
			review: review({ connection: connection({ hasToken: true }) })
		});

		await expect
			.element(page.getByRole('button', { name: 'Rotate token' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
	});

	it('lists what blocks enforcing', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({
				unboundReferencedGroups: {
					items: [
						group('okta/00g00000000000legacy', 'Legacy', {
							consoleURL: 'https://example-admin.okta.com/admin/group/00g00000000000legacy',
							references: [{ kind: 'modelAccessPolicy', id: 'map1', displayName: 'Models' }]
						})
					],
					total: 1
				},
				enforceBlockers: ['The referenced group "Legacy" has not been pushed from Okta.']
			})
		});

		await expect.element(page.getByText('SCIM cannot be enforced yet')).toBeVisible();
		await expect
			.element(page.getByText('The referenced group "Legacy" has not been pushed from Okta.'))
			.toBeVisible();
		await expect
			.element(page.getByText('Referenced by model access policy “Models”'))
			.toBeVisible();
		await expect.element(page.getByRole('link', { name: 'Open in Okta' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enforce SCIM' })).toBeDisabled();
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
				HttpResponse.json(review({ connection: connection({ state: 'enforced' }) }))
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { review: review() });

		await page.getByRole('button', { name: 'Enforce SCIM' }).click();
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
			review: review({
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
				review: review({
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
			review: review({
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
		await renderScimView([Group.USER, Group.AUDITOR], {
			review: review({ connection: connection({ hasToken: true }) })
		});

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
		const blocker = 'The referenced group "Legacy" has not been pushed from Okta.';
		worker.use(
			http.post(`*/api/scim-connections/${connectionID}/enforce`, () =>
				HttpResponse.json({ error: `SCIM cannot be enforced:\n- ${blocker}` }, { status: 400 })
			),
			http.get(`*/api/scim-connections/${connectionID}/review`, () =>
				HttpResponse.json(review({ enforceBlockers: [blocker] }))
			)
		);
		await renderScimView([Group.OWNER, Group.ADMIN], { review: review() });

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
				blockers: ['GitHub does not support SCIM provisioning.'],
				unboundReferencedGroups: { items: [], total: 0 },
				unreferencedGroups: { items: [], total: 0 }
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

	it('previews enabling SCIM for a provider that synchronizes its directory', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			enablePreview: enablePreview({
				warnings: [
					{
						type: 'everyoneGroup',
						message: 'The group "Everyone" cannot be pushed from Okta.',
						groupID: 'okta/00g000000000everyone',
						groupName: 'Everyone'
					}
				]
			})
		});

		await expect.element(page.getByRole('heading', { name: 'Move Okta to SCIM' })).toBeVisible();
		await expect.element(page.getByText('Referenced groups not pushed yet (1)')).toBeVisible();
		await expect.element(page.getByText('Engineering', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Referenced by access control rule “Servers”'))
			.toBeVisible();
		await expect.element(page.getByRole('link', { name: 'Open in Okta' })).toBeVisible();
		await expect.element(page.getByText('Unreferenced groups (1)')).toBeVisible();
		await expect.element(page.getByText('Stale', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('The group "Everyone" cannot be pushed from Okta.'))
			.toBeVisible();
		await expect
			.element(page.getByText('https://obot.example.com/scim/v2/<connection ID>'))
			.toBeVisible();
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
		await expect
			.element(page.getByText('deletes 1 unreferenced group.', { exact: false }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Enable SCIM' }).last().click();

		await vi.waitFor(() => expect(enable).toHaveBeenCalledOnce());
		await expect.element(page.getByText('obot_scim_enabled')).toBeVisible();
		await expect
			.element(page.getByText('SCIM is enabled for Okta. Deleted 1 unreferenced group.'))
			.toBeVisible();
		await page.getByRole('button', { name: 'Done' }).click();
		await expect.element(page.getByText('obot_scim_enabled')).not.toBeInTheDocument();

		await expect
			.element(page.getByRole('heading', { name: 'Finish moving to SCIM' }))
			.toBeVisible();
		const checklist = page.getByRole('list').filter({ hasText: 'Push the referenced groups' });
		await expect.element(checklist.getByText('1 not pushed yet', { exact: false })).toBeVisible();
		await expect
			.element(page.getByText('Referenced groups not pushed yet (1)', { exact: true }))
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
						// A group became unreferenced after the deletion, so only the retry's success clears
						// the report of the failed deletion.
						unreferencedGroups:
							deleteGroups.mock.calls.length === 0
								? { items: [group('okta/00g000000000000stale', 'Stale')], total: 1 }
								: { items: [group('okta/00g000000000000later', 'Later')], total: 1 }
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
		await expect.element(page.getByText('Later', { exact: true })).toBeVisible();
	});

	it('does not let administrators who are not Owners delete unreferenced groups', async () => {
		await renderScimView([Group.ADMIN], {
			review: review({
				unreferencedGroups: { items: [group('okta/00g000000000000stale', 'Stale')], total: 1 }
			})
		});

		await expect.element(page.getByText('Unreferenced groups (1)')).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Delete unreferenced groups' }))
			.not.toBeInTheDocument();
	});

	it('asks to remove the directory credentials that SCIM made unused', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({
				connection: connection({ origin: 'migrated', state: 'enforced', hasToken: true }),
				unusedDirectoryParameters: [
					'OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID',
					'OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY'
				]
			})
		});

		await expect
			.element(page.getByText('still holds the credentials that Obot used', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Auth Providers' }))
			.toHaveAttribute('href', '/identity-access?view=auth-providers');
	});

	it('pages through the groups of the preview', async () => {
		const requested = vi.fn();
		worker.use(
			http.get('*/api/scim-connections/enable-preview/groups', ({ request }) => {
				const url = new URL(request.url);
				requested(url.searchParams.get('list'), url.searchParams.get('offset'));
				return HttpResponse.json({
					items: [group('okta/g3', 'Group 3'), group('okta/g4', 'Group 4')],
					total: 4
				});
			})
		);
		await renderScimView([Group.OWNER, Group.ADMIN], {
			pageSize: 2,
			enablePreview: enablePreview({
				unreferencedGroups: {
					items: [group('okta/g1', 'Group 1'), group('okta/g2', 'Group 2')],
					total: 4
				}
			})
		});

		await expect.element(page.getByText('1 of 2', { exact: true })).toBeVisible();
		await page.getByRole('button', { name: 'Next page of unreferenced groups' }).click();

		await vi.waitFor(() => expect(requested).toHaveBeenCalledWith('unreferenced', '2'));
		await expect.element(page.getByText('Group 3')).toBeVisible();
		await expect.element(page.getByText('2 of 2', { exact: true })).toBeVisible();
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
						? review({
								connection: { ...migrated, state: 'enforced' },
								unreferencedGroups: {
									items: [group('okta/00g000000000000later', 'Later')],
									total: 1
								}
							})
						: review({
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

		await expect.element(page.getByText('SCIM is enforced.', { exact: false })).toBeVisible();
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

		const checklist = page.getByRole('list').filter({ hasText: 'Push the referenced groups' });
		await expect.element(checklist.getByText('1. Generate the token')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Generate token' })).toBeVisible();
	});

	it('does not ask a migrated connection that has its token for another', async () => {
		await renderScimView([Group.OWNER, Group.ADMIN], {
			review: review({ connection: connection({ origin: 'migrated', hasToken: true }) })
		});

		const checklist = page.getByRole('list').filter({ hasText: 'Push the referenced groups' });
		await expect.element(checklist.getByText('1. Create the SCIM app in Okta')).toBeVisible();
		await expect.element(checklist.getByText('Generate the token')).not.toBeInTheDocument();
	});
});
