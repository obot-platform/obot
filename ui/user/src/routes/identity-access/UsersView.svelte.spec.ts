import { page as appPage } from '$app/state';
import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
import { Group, Role, type LocalAuthUser, type OrgUser } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import UsersView from './UsersView.svelte';
import { http, HttpResponse } from 'msw';
import { untrack } from 'svelte';
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

afterEach(() => {
	appPage.url.searchParams.delete('query');
});

function orgUser(overrides: Partial<OrgUser> & Pick<OrgUser, 'id' | 'email'>): OrgUser {
	return {
		created: '2026-01-01T00:00:00.000Z',
		username: overrides.email,
		explicitRole: false,
		role: Role.BASIC,
		effectiveRole: Role.BASIC,
		groups: [Group.USER],
		iconURL: '',
		displayName: overrides.email,
		...overrides
	};
}

const activeUser = orgUser({
	id: '1',
	email: 'active@example.com',
	status: 'active',
	managementSource: 'obot'
});

const deactivatedUser = orgUser({
	id: '2',
	email: 'deactivated@example.com',
	status: 'disabled',
	disabledAt: '2026-09-01T00:00:00.000Z',
	disabledReason: 'scim_inactive',
	managementSource: 'scim'
});

const unprovisionedUser = orgUser({
	id: '3',
	email: 'unprovisioned@example.com',
	status: 'disabled',
	disabledAt: '2026-09-01T00:00:00.000Z',
	disabledReason: 'scim_unprovisioned',
	managementSource: 'scim'
});

// A user that SCIM provisions, and the identity provider still has active.
const provisionedUser = orgUser({
	id: '5',
	email: 'provisioned@example.com',
	status: 'active',
	managementSource: 'scim'
});

// A server that predates lifecycle status sends none of its fields.
const legacyUser = orgUser({
	id: '4',
	email: 'legacy@example.com'
});

async function renderUsersView(
	users: OrgUser[],
	viewerGroups: string[] = [Group.ADMIN],
	options?: { localAuthProviderAvailable?: boolean; providerDiscoveryComplete?: boolean }
) {
	await preparePageData({ profile: createMockProfile(viewerGroups) });
	return render(UsersView, {
		users,
		localAuthProviderAvailable: options?.localAuthProviderAvailable,
		providerDiscoveryComplete: options?.providerDiscoveryComplete
	});
}

function localAuthUser(email: string, overrides: Partial<LocalAuthUser> = {}): LocalAuthUser {
	return {
		id: overrides.id ?? email,
		email,
		created: '2026-02-01T00:00:00.000Z',
		requirePasswordChange: false,
		...overrides
	};
}

const validPassword = 'a'.repeat(LOCAL_AUTH_MIN_PASSWORD_LENGTH);

function userRow(email: string) {
	return page.getByRole('row').filter({ hasText: email });
}

async function openRowActions(email: string) {
	await userRow(email).getByRole('button', { name: 'Row actions' }).click();
}

describe('UsersView', () => {
	it('shows a status column', async () => {
		await renderUsersView([activeUser]);

		await expect.element(page.getByText('Status', { exact: true }).first()).toBeVisible();
	});

	it('shows active users as active without a SCIM badge', async () => {
		await renderUsersView([activeUser, legacyUser]);

		for (const user of [activeUser, legacyUser]) {
			const row = userRow(user.email);
			await expect.element(row.getByText('Active', { exact: true })).toBeVisible();
			await expect.element(row.getByText('SCIM', { exact: true })).not.toBeInTheDocument();
		}
	});

	it('shows the status, reason, and management source of disabled users', async () => {
		await renderUsersView([activeUser, deactivatedUser, unprovisionedUser]);

		const deactivated = userRow(deactivatedUser.email);
		await expect.element(deactivated.getByText('Disabled', { exact: true })).toBeVisible();
		await expect
			.element(deactivated.getByText('Deactivated in identity provider', { exact: true }))
			.toBeVisible();
		await expect.element(deactivated.getByText('SCIM', { exact: true })).toBeVisible();

		const unprovisioned = userRow(unprovisionedUser.email);
		await expect.element(unprovisioned.getByText('Disabled', { exact: true })).toBeVisible();
		await expect
			.element(unprovisioned.getByText('Not provisioned by identity provider', { exact: true }))
			.toBeVisible();
		await expect.element(unprovisioned.getByText('SCIM', { exact: true })).toBeVisible();

		const active = userRow(activeUser.email);
		await expect.element(active.getByText('Disabled', { exact: true })).not.toBeInTheDocument();
	});

	it('shows Claude Tag users read-only', async () => {
		const claudeTagUser = {
			...orgUser({
				id: '7',
				email: '',
				displayName: 'Claude Tag — Sandbox',
				username: 'claude-tag:1'
			}),
			email: undefined,
			internal: true
		} as unknown as OrgUser;
		await renderUsersView([activeUser, claudeTagUser]);

		await openRowActions('Claude Tag — Sandbox');
		await expect.element(page.getByRole('button', { name: 'View Access Policies' })).toBeEnabled();
		await expect.element(page.getByRole('button', { name: 'Update Role' })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeDisabled();
	});

	it('offers to update the role of a user that Obot manages', async () => {
		await renderUsersView([activeUser]);

		await openRowActions(activeUser.email);
		await expect.element(page.getByRole('button', { name: 'Update Role' })).toBeEnabled();
	});

	it('searches users when some users have no email', async () => {
		// The server lists internal users even when they have no email.
		const noEmailUser = {
			...orgUser({ id: '6', email: '', displayName: 'Build Service', username: 'build-service' }),
			email: undefined,
			internal: true
		} as unknown as OrgUser;
		await renderUsersView([activeUser, noEmailUser]);

		await page.getByPlaceholder('Search by name or email...').fill('nomatch');
		await expect.element(userRow(activeUser.email)).not.toBeInTheDocument();
		await expect.element(userRow('Build Service')).not.toBeInTheDocument();

		await page.getByPlaceholder('Search by name or email...').fill('build');
		await expect.element(userRow('Build Service')).toBeVisible();
		await expect.element(userRow(activeUser.email)).not.toBeInTheDocument();
	});

	it('does not offer to delete a user whom the identity provider still provisions', async () => {
		await renderUsersView([provisionedUser, deactivatedUser, activeUser]);

		await openRowActions(provisionedUser.email);
		await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeDisabled();
	});

	it('offers to delete a user whom the identity provider deactivated', async () => {
		await renderUsersView([provisionedUser, deactivatedUser, activeUser]);

		await openRowActions(deactivatedUser.email);
		await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeEnabled();
	});

	it('offers to delete a user that Obot manages', async () => {
		await renderUsersView([provisionedUser, deactivatedUser, activeUser]);

		await openRowActions(activeUser.email);
		await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeEnabled();
	});

	it('enables a disabled user after confirmation', async () => {
		const enable = vi.fn();
		worker.use(
			http.post(`*/api/users/${deactivatedUser.id}/enable`, () => {
				enable();
				return HttpResponse.json({ ...deactivatedUser, status: 'active' });
			}),
			http.get('*/api/users', () =>
				HttpResponse.json({
					items: [
						{
							...deactivatedUser,
							status: 'active',
							disabledAt: undefined,
							disabledReason: undefined
						}
					]
				})
			)
		);
		await renderUsersView([deactivatedUser]);

		await openRowActions(deactivatedUser.email);
		await page.getByRole('button', { name: 'Enable User' }).click();
		const confirm = page.getByRole('dialog').filter({ hasText: 'Enable user' });
		await expect.element(confirm.getByText('Confirm Enable', { exact: true })).toBeVisible();
		await expect.element(confirm.getByText('Confirm Delete')).not.toBeInTheDocument();
		await expect.element(page.getByText(/including their API keys and agents/)).toBeVisible();
		await page.getByRole('button', { name: 'Enable', exact: true }).click();

		await vi.waitFor(() => expect(enable).toHaveBeenCalledOnce());
		await expect
			.element(userRow(deactivatedUser.email).getByText('Active', { exact: true }))
			.toBeVisible();
	});

	it('does not offer to enable a user who is active', async () => {
		await renderUsersView([activeUser]);

		await openRowActions(activeUser.email);
		await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Enable User' })).not.toBeInTheDocument();
	});

	describe('enabling a user with a role that only an Owner can restore', () => {
		const privileged = [
			{ name: 'an Owner', role: Role.OWNER, effectiveRole: Role.OWNER },
			{
				name: 'an auditor',
				role: Role.BASIC | Role.AUDITOR,
				effectiveRole: Role.BASIC | Role.AUDITOR
			},
			{
				name: 'a user whose group grants the Owner role',
				role: Role.BASIC,
				effectiveRole: Role.OWNER
			}
		];

		for (const target of privileged) {
			const user = orgUser({
				id: '6',
				email: 'privileged@example.com',
				status: 'disabled',
				disabledAt: '2026-09-01T00:00:00.000Z',
				disabledReason: 'scim_inactive',
				managementSource: 'scim',
				role: target.role,
				effectiveRole: target.effectiveRole
			});

			it(`is not offered to an administrator who is not an Owner, for ${target.name}`, async () => {
				await renderUsersView([user]);

				await openRowActions(user.email);
				await expect.element(page.getByRole('button', { name: 'Enable User' })).toBeDisabled();
			});

			it(`is offered to an Owner, for ${target.name}`, async () => {
				await renderUsersView([user], [Group.OWNER, Group.ADMIN]);

				await openRowActions(user.email);
				await expect.element(page.getByRole('button', { name: 'Enable User' })).toBeEnabled();
			});
		}
	});

	it('closes the confirmation, and leaves the other actions usable, when a deletion is refused', async () => {
		worker.use(
			http.delete(`*/api/users/${activeUser.id}`, () =>
				HttpResponse.json({ error: 'refused' }, { status: 409 })
			)
		);
		await renderUsersView([activeUser]);

		await openRowActions(activeUser.email);
		await page.getByRole('button', { name: 'Delete User' }).click();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();
		await expect
			.element(page.getByText(`Delete user ${activeUser.email}?`))
			.not.toBeInTheDocument();

		await openRowActions(activeUser.email);
		await page.getByRole('button', { name: 'Update Role' }).click();
		await expect.element(page.getByRole('button', { name: 'Update', exact: true })).toBeEnabled();
	});

	describe('local auth provider', () => {
		const pending = localAuthUser('pending@example.com', { id: '10', requirePasswordChange: true });
		const linked = localAuthUser(activeUser.email, { id: '11', orgUserId: activeUser.id });

		function mockLocalAccounts(accounts: LocalAuthUser[]) {
			let items = accounts;
			worker.use(
				http.get('*/api/local-auth/users', () => HttpResponse.json({ items })),
				http.get('*/api/users', () => HttpResponse.json({ items: [activeUser] }))
			);
			return {
				setItems(next: LocalAuthUser[]) {
					items = next;
				}
			};
		}

		it('blocks deletion until provider discovery and local membership lookup succeed', async () => {
			let finishLookup!: (response: Response) => void;
			worker.use(
				http.get(
					'*/api/local-auth/users',
					() =>
						new Promise<Response>((resolve) => {
							finishLookup = resolve;
						})
				)
			);
			const view = await renderUsersView([activeUser], [Group.ADMIN], {
				providerDiscoveryComplete: false
			});
			await openRowActions(activeUser.email);
			await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeDisabled();
			await view.rerender({ providerDiscoveryComplete: true, localAuthProviderAvailable: true });
			await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeDisabled();
			await vi.waitFor(() => expect(finishLookup).toBeTypeOf('function'));
			finishLookup(HttpResponse.json({ items: [linked] }));
			await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeEnabled();
		});

		it.each([false, true])(
			'rechecks membership on confirmation (lookup fails: %s)',
			async (fails) => {
				const accounts = mockLocalAccounts([]);
				const removeOrg = vi.fn();
				const removeLocal = vi.fn();
				worker.use(
					http.delete('*/api/users/*', () => {
						removeOrg();
						return new HttpResponse(null, { status: 204 });
					}),
					http.delete('*/api/local-auth/users/11', () => {
						removeLocal();
						return new HttpResponse(null, { status: 204 });
					})
				);
				await renderUsersView([activeUser], [Group.ADMIN], { localAuthProviderAvailable: true });
				await openRowActions(activeUser.email);
				await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeEnabled();
				await page.getByRole('button', { name: 'Delete User' }).click();
				accounts.setItems([linked]);
				if (fails)
					worker.use(
						http.get('*/api/local-auth/users', () =>
							HttpResponse.json({ message: 'lookup failed' }, { status: 500 })
						)
					);
				await page.getByRole('button', { name: "Yes, I'm sure" }).click();
				await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
				expect(removeOrg).not.toHaveBeenCalled();
				expect(removeLocal).toHaveBeenCalledTimes(fails ? 0 : 1);
			}
		);

		it('lists a local account that has not signed in, with org actions disabled', async () => {
			mockLocalAccounts([pending, linked]);
			await renderUsersView([activeUser], [Group.ADMIN], { localAuthProviderAvailable: true });

			const pendingRow = userRow(pending.email);
			await expect.element(pendingRow).toBeVisible();
			await expect.element(pendingRow.getByText('Active', { exact: true })).not.toBeInTheDocument();
			await expect
				.element(userRow(activeUser.email).getByText('Active', { exact: true }))
				.toBeVisible();

			await openRowActions(pending.email);
			await expect
				.element(page.getByRole('button', { name: 'View Access Policies' }))
				.toBeDisabled();
			await expect.element(page.getByRole('button', { name: 'Update Role' })).toBeDisabled();
			await expect
				.element(page.getByRole('button', { name: 'Enable User' }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Reset password' })).toBeEnabled();
			await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeEnabled();
		});

		it('keeps the org user actions when the local account has signed in', async () => {
			mockLocalAccounts([linked]);
			await renderUsersView([activeUser], [Group.ADMIN], { localAuthProviderAvailable: true });

			await openRowActions(activeUser.email);
			await expect
				.element(page.getByRole('button', { name: 'View Access Policies' }))
				.toBeEnabled();
			await expect.element(page.getByRole('button', { name: 'Update Role' })).toBeEnabled();
			await expect.element(page.getByRole('button', { name: 'Reset password' })).toBeEnabled();
			await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeEnabled();
		});

		const ownerUser = orgUser({
			id: '20',
			email: 'owner@example.com',
			role: Role.OWNER,
			effectiveRole: Role.OWNER,
			groups: [Group.OWNER, Group.ADMIN]
		});
		const ownerAccount = localAuthUser(ownerUser.email, { id: '21', orgUserId: ownerUser.id });

		for (const target of [
			{ name: 'an auditor', role: Role.BASIC | Role.AUDITOR },
			{ name: 'an admin with auditor access', role: Role.ADMIN | Role.AUDITOR }
		]) {
			it.each([
				{ name: 'an admin', groups: [Group.ADMIN], allowed: false },
				{
					name: 'an admin with auditor access',
					groups: [Group.ADMIN, Group.AUDITOR],
					allowed: false
				},
				{ name: 'an Owner', groups: [Group.OWNER, Group.ADMIN], allowed: true }
			])(
				`only lets an Owner reset ${target.name}'s password: $name`,
				async ({ groups, allowed }) => {
					const targetUser = orgUser({
						id: '30',
						email: 'auditor@example.com',
						role: target.role,
						effectiveRole: target.role,
						groups: [Group.AUDITOR]
					});
					const setPassword = vi.fn();
					mockLocalAccounts([
						localAuthUser(targetUser.email, { id: '31', orgUserId: targetUser.id })
					]);
					worker.use(
						http.post('*/api/local-auth/users/31/password', async ({ request }) => {
							setPassword(await request.json());
							return new HttpResponse(null, { status: 204 });
						})
					);
					await renderUsersView([targetUser], groups, { localAuthProviderAvailable: true });
					await openRowActions(targetUser.email);
					const reset = page.getByRole('button', { name: 'Reset password' });
					if (!allowed) {
						await expect.element(reset).toBeDisabled();
						expect(setPassword).not.toHaveBeenCalled();
						return;
					}

					await expect.element(reset).toBeEnabled();
					await reset.click();
					await page.getByCSS('#users-reset-password').fill(validPassword);
					await page.getByRole('dialog').getByRole('button', { name: 'Save', exact: true }).click();
					await vi.waitFor(() =>
						expect(setPassword).toHaveBeenCalledWith({
							password: validPassword,
							requirePasswordChange: true
						})
					);
					await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
				}
			);
		}

		it.each([
			{ name: 'an admin', groups: [Group.ADMIN] },
			{ name: 'an admin with auditor access', groups: [Group.ADMIN, Group.AUDITOR] }
		])('blocks $name from resetting or deleting a linked Owner account', async ({ groups }) => {
			mockLocalAccounts([ownerAccount]);
			await renderUsersView([ownerUser], groups, { localAuthProviderAvailable: true });

			await openRowActions(ownerUser.email);
			await expect.element(page.getByRole('button', { name: 'Reset password' })).toBeDisabled();
			await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeDisabled();
		});

		it('hides reset and delete actions from a read-only auditor for a linked Owner account', async () => {
			mockLocalAccounts([ownerAccount]);
			await renderUsersView([ownerUser], [Group.AUDITOR], { localAuthProviderAvailable: true });

			await openRowActions(ownerUser.email);
			await expect
				.element(page.getByRole('button', { name: 'View Access Policies' }))
				.toBeEnabled();
			await expect
				.element(page.getByRole('button', { name: 'Reset password' }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Delete User' }))
				.not.toBeInTheDocument();
		});

		it.each(['reset', 'delete'] as const)(
			'lets an Owner %s a linked Owner account',
			async (action) => {
				const act = vi.fn();
				mockLocalAccounts([ownerAccount]);
				worker.use(
					http.post('*/api/local-auth/users/21/password', async ({ request }) => {
						act(await request.json());
						return new HttpResponse(null, { status: 204 });
					}),
					http.delete('*/api/local-auth/users/21', () => {
						act();
						return new HttpResponse(null, { status: 204 });
					})
				);
				await renderUsersView([ownerUser], [Group.OWNER, Group.ADMIN], {
					localAuthProviderAvailable: true
				});

				await openRowActions(ownerUser.email);
				await expect.element(page.getByRole('button', { name: 'Reset password' })).toBeEnabled();
				await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeEnabled();
				if (action === 'reset') {
					await page.getByRole('button', { name: 'Reset password' }).click();
					await page.getByCSS('#users-reset-password').fill(validPassword);
					await page.getByRole('dialog').getByRole('button', { name: 'Save', exact: true }).click();
					await vi.waitFor(() =>
						expect(act).toHaveBeenCalledWith({
							password: validPassword,
							requirePasswordChange: true
						})
					);
				} else {
					await page.getByRole('button', { name: 'Delete User' }).click();
					await page.getByRole('button', { name: "Yes, I'm sure" }).click();
					await vi.waitFor(() => expect(act).toHaveBeenCalledOnce());
				}
				await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
			}
		);

		it('blocks deletion while local account lookup fails and retries the lookup', async () => {
			mockLocalAccounts([linked]);
			let attempts = 0;
			const removeOrg = vi.fn();
			const removeLocal = vi.fn();
			worker.use(
				http.get('*/api/local-auth/users', () => {
					attempts += 1;
					return attempts === 1
						? HttpResponse.json({ message: 'temporary failure' }, { status: 500 })
						: HttpResponse.json({ items: [linked] });
				}),
				http.delete('*/api/users/*', () => {
					removeOrg();
					return new HttpResponse(null, { status: 204 });
				}),
				http.delete('*/api/local-auth/users/11', () => {
					removeLocal();
					return new HttpResponse(null, { status: 204 });
				})
			);
			await renderUsersView([activeUser], [Group.ADMIN], { localAuthProviderAvailable: true });

			await expect.element(page.getByRole('alert')).toBeVisible();
			await openRowActions(activeUser.email);
			await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeDisabled();
			await page.getByCSS('#click-catch').click();
			await page.getByRole('button', { name: 'Retry' }).click();
			await vi.waitFor(() => expect(attempts).toBe(2));
			await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
			await openRowActions(activeUser.email);
			await expect.element(page.getByRole('button', { name: 'Delete User' })).toBeEnabled();
			await page.getByRole('button', { name: 'Delete User' }).click();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();
			await vi.waitFor(() => expect(removeLocal).toHaveBeenCalledOnce());
			expect(removeOrg).not.toHaveBeenCalled();
		});

		it('keeps an OAuth user separate from a local account with the same email when deleting', async () => {
			const removeOrg = vi.fn();
			const removeLocal = vi.fn();
			const accounts = mockLocalAccounts([localAuthUser(activeUser.email, { id: '10' })]);
			worker.use(
				http.delete('*/api/users/*', () => {
					removeOrg();
					return new HttpResponse(null, { status: 204 });
				}),
				http.delete('*/api/local-auth/users/10', () => {
					removeLocal();
					accounts.setItems([]);
					return new HttpResponse(null, { status: 204 });
				})
			);
			await renderUsersView([activeUser], [Group.ADMIN], { localAuthProviderAvailable: true });

			const localRow = userRow(activeUser.email).filter({ hasText: 'Created' });
			await expect.element(localRow).toBeVisible();
			await expect
				.element(userRow(activeUser.email).getByText('Active', { exact: true }))
				.toBeVisible();
			await localRow.getByRole('button', { name: 'Row actions' }).click();
			await expect.element(page.getByRole('button', { name: 'Update Role' })).toBeDisabled();
			await page.getByRole('button', { name: 'Delete User' }).click();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();

			await vi.waitFor(() => expect(removeLocal).toHaveBeenCalledOnce());
			expect(removeOrg).not.toHaveBeenCalled();
			await expect.element(userRow(activeUser.email)).toBeVisible();
		});

		it('keeps org users that have no local auth account', async () => {
			const claudeTagUser = {
				...orgUser({
					id: '7',
					email: '',
					displayName: 'Claude Tag — Sandbox',
					username: 'claude-tag:1'
				}),
				email: undefined,
				internal: true
			} as unknown as OrgUser;
			mockLocalAccounts([linked]);
			await renderUsersView([activeUser, claudeTagUser], [Group.ADMIN], {
				localAuthProviderAvailable: true
			});

			await expect.element(userRow('Claude Tag — Sandbox')).toBeVisible();
		});

		it('deletes local auth user', async () => {
			const removeOrg = vi.fn();
			const removeLocal = vi.fn();
			mockLocalAccounts([linked]);
			worker.use(
				http.delete(`*/api/users/${activeUser.id}`, () => {
					removeOrg();
					return new HttpResponse(null, { status: 204 });
				}),
				http.delete('*/api/local-auth/users/11', () => {
					removeLocal();
					return new HttpResponse(null, { status: 204 });
				})
			);
			await renderUsersView([activeUser], [Group.ADMIN], { localAuthProviderAvailable: true });

			await openRowActions(activeUser.email);
			await page.getByRole('button', { name: 'Delete User' }).click();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();

			await vi.waitFor(() => {
				expect(removeLocal).toHaveBeenCalledOnce();
			});
			expect(removeOrg).not.toHaveBeenCalled();
		});

		it('deletes only the local auth user when there is no org user', async () => {
			const removeOrg = vi.fn();
			const removeLocal = vi.fn();
			const accounts = mockLocalAccounts([pending]);
			worker.use(
				http.delete('*/api/users/*', () => {
					removeOrg();
					return new HttpResponse(null, { status: 204 });
				}),
				http.delete('*/api/local-auth/users/10', () => {
					removeLocal();
					accounts.setItems([]);
					return new HttpResponse(null, { status: 204 });
				}),
				http.get('*/api/users', () => HttpResponse.json({ items: [] }))
			);
			await renderUsersView([], [Group.ADMIN], { localAuthProviderAvailable: true });

			await openRowActions(pending.email);
			await page.getByRole('button', { name: 'Delete User' }).click();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();

			await vi.waitFor(() => expect(removeLocal).toHaveBeenCalledOnce());
			expect(removeOrg).not.toHaveBeenCalled();
			await expect.element(userRow(pending.email)).not.toBeInTheDocument();
		});

		it('resets the password immediately', async () => {
			const setPassword = vi.fn();
			mockLocalAccounts([linked]);
			worker.use(
				http.post('*/api/local-auth/users/11/password', async ({ request }) => {
					setPassword(await request.json());
					return new HttpResponse(null, { status: 204 });
				})
			);
			await renderUsersView([activeUser], [Group.ADMIN], { localAuthProviderAvailable: true });

			await openRowActions(activeUser.email);
			await page.getByRole('button', { name: 'Reset password' }).click();
			const dialog = page.getByRole('dialog');
			await expect.element(dialog.getByText(activeUser.email)).toBeVisible();
			await page.getByCSS('#users-reset-password').fill(validPassword);
			await dialog.getByRole('button', { name: 'Save', exact: true }).click();

			await vi.waitFor(() =>
				expect(setPassword).toHaveBeenCalledWith({
					password: validPassword,
					requirePasswordChange: true
				})
			);
			await expect.element(dialog).not.toBeInTheDocument();
		});

		it('creates a local auth user from the dialog', async () => {
			const created = vi.fn();
			const accounts = mockLocalAccounts([]);
			worker.use(
				http.post('*/api/local-auth/users', async ({ request }) => {
					const body = (await request.json()) as {
						email: string;
						password: string;
						requirePasswordChange: boolean;
					};
					created(body);
					const user = localAuthUser(body.email, {
						id: '20',
						requirePasswordChange: body.requirePasswordChange
					});
					accounts.setItems([user]);
					return HttpResponse.json(user);
				}),
				http.get('*/api/users', () => HttpResponse.json({ items: [] }))
			);
			const view = await renderUsersView([], [Group.ADMIN], { localAuthProviderAvailable: true });
			untrack(() => view.component.openCreateUser());

			const dialog = page.getByRole('dialog');
			await dialog.getByLabelText('Email', { exact: true }).fill('ada@example.com');
			await page.getByCSS('#users-local-auth-password').fill(validPassword);
			await dialog.getByRole('button', { name: 'Save', exact: true }).click();

			await vi.waitFor(() =>
				expect(created).toHaveBeenCalledWith({
					email: 'ada@example.com',
					password: validPassword,
					requirePasswordChange: true
				})
			);
			await expect.element(userRow('ada@example.com')).toBeVisible();
			await expect.element(dialog).not.toBeInTheDocument();
		});
	});
});
