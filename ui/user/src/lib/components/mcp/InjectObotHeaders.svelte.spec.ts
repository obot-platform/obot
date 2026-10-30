import InjectObotHeaders from './InjectObotHeaders.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('InjectObotHeaders', () => {
	it('starts collapsed with every header disabled and allows independent selection', async () => {
		await render(InjectObotHeaders);
		const master = page.getByRole('switch', { name: /Obot header injection/ });
		await expect.element(master).not.toBeChecked();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-User-Id', exact: true }))
			.not.toBeInTheDocument();
		await master.click();
		await expect.element(master).toBeChecked();
		for (const name of [
			'X-Obot-Api-Key-Name',
			'X-Obot-Api-Key-Id',
			'X-Obot-User-Id',
			'X-Obot-User-Name',
			'X-Obot-User-Email',
			'X-Obot-IDP-User-Id',
			'X-Obot-IDP-Groups'
		]) {
			await expect.element(page.getByRole('switch', { name, exact: true })).not.toBeChecked();
		}
		await page.getByRole('switch', { name: 'X-Obot-Api-Key-Name', exact: true }).click();
		await page.getByRole('switch', { name: 'X-Obot-User-Id', exact: true }).click();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-Api-Key-Name', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-User-Email', exact: true }))
			.not.toBeChecked();
		await page.getByRole('switch', { name: 'X-Obot-IDP-User-Id', exact: true }).click();
		await page.getByRole('switch', { name: 'X-Obot-IDP-Groups', exact: true }).click();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-IDP-User-Id', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-IDP-Groups', exact: true }))
			.toBeChecked();
	});

	it('shows saved selections without allowing changes in read-only views', async () => {
		await render(InjectObotHeaders, {
			selected: [
				'X-Obot-Api-Key-Name',
				'X-Obot-User-Id',
				'X-Obot-IDP-User-Id',
				'X-Obot-IDP-Groups'
			],
			readonly: true
		});
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-Api-Key-Name', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-IDP-User-Id', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-IDP-Groups', exact: true }))
			.toBeChecked();
		await expect
			.element(page.getByRole('switch', { name: 'X-Obot-User-Id', exact: true }))
			.toBeChecked();
		for (let i = 0; i < 8; i++) {
			await expect.element(page.getByRole('switch').nth(i)).toBeDisabled();
		}
	});
});
