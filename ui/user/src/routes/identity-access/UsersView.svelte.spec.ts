import { Group, Role, type OrgUser } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import UsersView from './UsersView.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

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

async function renderUsersView(users: OrgUser[]) {
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
	return render(UsersView, { users });
}

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
});
