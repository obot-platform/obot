import type { MCPCatalogEntry } from '$lib/services';
import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import VMcpComponentConfigurationDialog from './VMcpComponentConfigurationDialog.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function configurableEntry(): MCPCatalogEntry {
	return createMCPCatalogEntry({
		id: 'entry-configurable',
		name: 'Configurable server',
		manifest: {
			config: [
				{
					key: 'API_TOKEN',
					name: 'API token',
					description: 'Secret token',
					required: true,
					sensitive: true,
					value: '',
					usage: 'env'
				},
				{
					key: 'REGION',
					name: 'Region',
					description: 'Deployment region',
					required: true,
					sensitive: false,
					value: '',
					usage: 'env'
				},
				{
					key: 'X-Org',
					name: 'Org header',
					description: 'Organization header',
					required: false,
					sensitive: false,
					value: '',
					usage: 'header'
				}
			]
		}
	});
}

describe('VMcpComponentConfigurationDialog.svelte', () => {
	it('requires a policy for every field before Next', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry());

		await expect.element(page.getByText('Set configuration policy')).toBeVisible();
		await page.getByRole('button', { name: 'Next' }).click();
		await expect
			.element(page.getByText('Select a policy for each configuration field.'))
			.toBeVisible();
		expect(onNext).not.toHaveBeenCalled();
	});

	it('shows a value field when Fixed is selected and submits policies on Next', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry());

		await page.getByRole('combobox', { name: 'API token policy' }).selectOptions('User-Supplied');
		await page.getByRole('combobox', { name: 'Region policy' }).selectOptions('Fixed');
		await page.getByRole('combobox', { name: 'Org header policy' }).selectOptions('Prohibited');

		await page.getByCSS('#fixed-REGION').fill('us-east-1');
		await page.getByRole('button', { name: 'Next' }).click();

		await vi.waitFor(() => expect(onNext).toHaveBeenCalledOnce());
		expect(onNext).toHaveBeenCalledWith([
			{ key: 'API_TOKEN', policy: 'userAllowed' },
			{ key: 'REGION', policy: 'fixed', value: 'us-east-1' },
			{ key: 'X-Org', policy: 'prohibited' }
		]);
	});

	it('prefills existing policies when editing configuration', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry(), {
			configuration: [
				{ key: 'API_TOKEN', policy: 'userAllowed' },
				{ key: 'REGION', policy: 'fixed', value: 'us-west-2' },
				{ key: 'X-Org', policy: 'prohibited' }
			],
			submitLabel: 'Save'
		});

		await expect.element(page.getByRole('combobox', { name: 'API token policy' })).toHaveValue(
			'userAllowed'
		);
		await expect.element(page.getByRole('combobox', { name: 'Region policy' })).toHaveValue('fixed');
		await expect.element(page.getByCSS('#fixed-REGION')).toHaveValue('us-west-2');
		await expect.element(page.getByRole('combobox', { name: 'Org header policy' })).toHaveValue(
			'prohibited'
		);
		await expect.element(page.getByRole('button', { name: 'Save' })).toBeVisible();
	});
});
