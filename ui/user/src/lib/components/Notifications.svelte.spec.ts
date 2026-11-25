import { notifications, profile } from '$lib/stores';
import { createMockProfile } from '../../tests/helpers/pageData';
import Notifications from './Notifications.svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

let dialog: HTMLDialogElement | undefined;

function mountNotifications() {
	profile.initialize(createMockProfile());
	notifications.items = [];
	return render(Notifications);
}

function openDialog(modal: boolean) {
	dialog = document.createElement('dialog');
	dialog.className = 'dialog';
	const backdrop = document.createElement('div');
	backdrop.className = 'dialog-backdrop';
	const label = document.createElement('p');
	label.textContent = 'Editing';
	dialog.append(backdrop, label);
	document.body.append(dialog);
	if (modal) dialog.showModal();
	else dialog.show();
}

afterEach(() => {
	notifications.items = [];
	if (dialog?.open) dialog.close();
	dialog?.remove();
	dialog = undefined;
});

describe('Notifications', () => {
	it('stays clickable when a modal dialog opens over it', async () => {
		await mountNotifications();
		notifications.add(new Error('Something failed'));

		const toast = page.getByRole('button', { name: 'Something failed' });
		await expect.element(toast).toBeVisible();
		await expect.element(page.getByCSS('.notification-host:popover-open')).toBeVisible();

		openDialog(true);
		await expect.element(page.getByRole('dialog')).toBeVisible();

		// Dismiss is intercepted when the dialog backdrop is the topmost hit target.
		await page.getByRole('button', { name: 'close' }).click();
		await expect.element(page.getByText('Something failed')).not.toBeInTheDocument();
	});

	it('opens above a modal dialog that is already showing', async () => {
		await mountNotifications();
		openDialog(true);
		await expect.element(page.getByRole('dialog')).toBeVisible();

		notifications.add(new Error('Something failed'));
		await expect.element(page.getByRole('button', { name: 'Something failed' })).toBeVisible();
		await page.getByRole('button', { name: 'close' }).click();
		await expect.element(page.getByText('Something failed')).not.toBeInTheDocument();
	});

	it('stays clickable above a non-modal dialog backdrop', async () => {
		await mountNotifications();
		notifications.add(new Error('Something failed'));
		await expect.element(page.getByRole('button', { name: 'Something failed' })).toBeVisible();

		openDialog(false);
		await expect.element(page.getByRole('dialog')).toBeVisible();
		await page.getByRole('button', { name: 'close' }).click();
		await expect.element(page.getByText('Something failed')).not.toBeInTheDocument();
	});

	it('keeps the toast after the modal closes', async () => {
		await mountNotifications();
		notifications.add(new Error('Something failed'));
		openDialog(true);
		await expect.element(page.getByRole('dialog')).toBeVisible();

		dialog?.close();
		await expect.element(page.getByCSS('.notification-host:popover-open')).toBeVisible();
		await page.getByRole('button', { name: 'close' }).click();
		await expect.element(page.getByText('Something failed')).not.toBeInTheDocument();
	});

	it('dismisses each error while the modal stays open', async () => {
		await mountNotifications();
		openDialog(true);
		notifications.add(new Error('First failed'));
		notifications.add(new Error('Second failed'));

		await expect.element(page.getByRole('button', { name: 'First failed' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Second failed' })).toBeVisible();
		await page.getByRole('button', { name: 'close' }).first().click();
		await expect
			.element(page.getByRole('button', { name: 'First failed' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'close' }).click();
		await expect.element(page.getByText('Second failed')).not.toBeInTheDocument();
		await expect.element(page.getByRole('dialog')).toBeVisible();
	});

	it('shows errors and successes in the order they were added', async () => {
		await mountNotifications();
		notifications.add(new Error('First failed'));
		notifications.add('Saved the profile', 'success', 60_000);
		notifications.add(new Error('Second failed'));

		await expect.element(page.getByRole('button', { name: 'First failed' })).toBeVisible();
		await expect.element(page.getByText('Saved the profile')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Second failed' })).toBeVisible();

		const text = (await page.getByCSS('.notification-host').element()).textContent ?? '';
		expect(text.indexOf('First failed')).toBeLessThan(text.indexOf('Saved the profile'));
		expect(text.indexOf('Saved the profile')).toBeLessThan(text.indexOf('Second failed'));
	});
});
