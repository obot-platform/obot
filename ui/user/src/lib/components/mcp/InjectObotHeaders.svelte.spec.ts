import InjectObotHeaders from './InjectObotHeaders.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('InjectObotHeaders', () => {
	it('starts collapsed with every header disabled and allows independent selection', async () => {
		await render(InjectObotHeaders);
		await expect.element(page.getByCSS('details')).not.toHaveAttribute('open');
		await page.getByText('Inject Obot headers', { exact: true }).click();
		for (const name of [
			'X-Obot-Api-Key-Name',
			'X-Obot-Api-Key-Id',
			'X-Obot-User-Id',
			'X-Obot-User-Name',
			'X-Obot-User-Email',
			'X-Obot-IDP-User-Id',
			'X-Obot-IDP-Groups'
		]) {
			await expect.element(page.getByRole('checkbox', { name, exact: true })).not.toBeChecked();
		}
		await page.getByRole('checkbox', { name: 'X-Obot-Api-Key-Name', exact: true }).click();
		await page.getByRole('checkbox', { name: 'X-Obot-User-Id', exact: true }).click();
		await expect
			.element(page.getByRole('checkbox', { name: 'X-Obot-Api-Key-Name', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('checkbox', { name: 'X-Obot-User-Email', exact: true }))
			.not.toBeChecked();
		await page.getByRole('checkbox', { name: 'X-Obot-IDP-User-Id', exact: true }).click();
		await page.getByRole('checkbox', { name: 'X-Obot-IDP-Groups', exact: true }).click();
		await expect
			.element(page.getByRole('checkbox', { name: 'X-Obot-IDP-User-Id', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('checkbox', { name: 'X-Obot-IDP-Groups', exact: true }))
			.toBeChecked();
		await expect.element(page.getByText(/ambiguous across linked identities/)).toBeVisible();
		await expect.element(page.getByText(/including provider prefixes/)).toBeVisible();
		await expect.element(page.getByText(/not an autonomous agent/)).toBeVisible();
		await expect.element(page.getByText(/not guarantee the address is verified/)).toBeVisible();
	});

	it('shows saved selections without allowing changes in read-only views', async () => {
		await render(InjectObotHeaders, {
			selected: ['X-Obot-Api-Key-Name', 'X-Obot-IDP-User-Id', 'X-Obot-IDP-Groups'],
			readonly: true
		});
		await page.getByText('Inject Obot headers', { exact: true }).click();
		await expect
			.element(page.getByRole('checkbox', { name: 'X-Obot-Api-Key-Name', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('checkbox', { name: 'X-Obot-IDP-User-Id', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('checkbox', { name: 'X-Obot-IDP-Groups', exact: true }))
			.toBeChecked();
		for (let i = 0; i < 7; i++) {
			await expect.element(page.getByRole('checkbox').nth(i)).toBeDisabled();
		}
	});
});
