import type { MCPCatalogEntryFieldManifest } from '$lib/services';
import CustomConfigurationForm from './CustomConfigurationForm.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('CustomConfigurationForm.svelte', () => {
	it('separates configured static fields from user-supplied configuration', async () => {
		const config: MCPCatalogEntryFieldManifest[] = [
			{
				key: 'GREETING',
				name: 'Greeting',
				description: 'Greeting description',
				value: '',
				required: true,
				sensitive: false
			},
			{
				key: 'SECRET_KEY',
				name: 'Static Secret',
				description: 'Static secret description',
				value: '',
				valueConfigured: true,
				required: false,
				sensitive: true
			}
		];

		await render(CustomConfigurationForm, {
			config,
			readonly: true,
			serverUserType: 'singleUser'
		});

		const userSection = page.getByRole('heading', { name: 'User Supplied Configuration' });
		const staticSection = page.getByRole('heading', { name: 'Static Configuration' });
		await expect.element(userSection).toBeVisible();
		await expect.element(staticSection).toBeVisible();
		await expect
			.element(page.getByCSS('#catalog-server-configuration #env-name-catalog-server-env-0'))
			.toHaveValue('Greeting');
		await expect
			.element(page.getByCSS('#catalog-server-configuration-static #env-name-catalog-server-env-1'))
			.toHaveValue('Static Secret');
	});
});
