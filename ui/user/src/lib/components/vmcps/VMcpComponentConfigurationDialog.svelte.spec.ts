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

		await page.getByRole('radio', { name: 'User-supplied' }).first().click();
		await page.getByRole('radio', { name: 'Fixed' }).nth(1).click();
		await page.getByRole('radio', { name: 'Prohibited' }).last().click();

		await page.getByCSS('#fixed-REGION').fill('us-east-1');
		await page.getByRole('button', { name: 'Next' }).click();

		await vi.waitFor(() => expect(onNext).toHaveBeenCalledOnce());
		expect(onNext).toHaveBeenCalledWith([
			{ key: 'API_TOKEN', policy: 'userAllowed' },
			{ key: 'REGION', policy: 'fixed', value: 'us-east-1' },
			{ key: 'X-Org', policy: 'prohibited' }
		]);
	});
});
