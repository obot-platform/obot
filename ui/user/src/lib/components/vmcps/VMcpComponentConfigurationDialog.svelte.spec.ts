import type { MCPCatalogEntry, MCPConfig } from '$lib/services';
import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import VMcpComponentConfigurationDialog from './VMcpComponentConfigurationDialog.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function field(overrides: Partial<MCPConfig> & Pick<MCPConfig, 'key' | 'name'>): MCPConfig {
	return {
		description: overrides.description ?? overrides.name,
		required: overrides.required ?? false,
		sensitive: overrides.sensitive ?? false,
		value: overrides.value ?? '',
		usage: overrides.usage ?? 'env',
		...overrides
	};
}

function configurableEntry(
	overrides: { runtime?: MCPCatalogEntry['manifest']['runtime']; config?: MCPConfig[] } = {}
): MCPCatalogEntry {
	return createMCPCatalogEntry({
		id: 'entry-configurable',
		name: 'Configurable server',
		runtime: overrides.runtime,
		manifest: {
			config: overrides.config ?? [
				field({
					key: 'API_TOKEN',
					name: 'API token',
					description: 'Secret token',
					required: true,
					sensitive: true
				}),
				field({
					key: 'REGION',
					name: 'Region',
					description: 'Deployment region',
					required: true
				}),
				field({
					key: 'X-Org',
					name: 'Org header',
					description: 'Organization header',
					usage: 'header'
				})
			]
		}
	});
}

describe('VMcpComponentConfigurationDialog.svelte', () => {
	it('requires values for fixed fields before Next', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry());

		await expect.element(page.getByText('Configure Configurable server')).toBeVisible();
		await page.getByRole('button', { name: 'Next' }).click();
		await expect
			.element(page.getByText('Please complete all fixed configuration fields with valid values.'))
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

		await expect
			.element(page.getByRole('combobox', { name: 'API token policy' }))
			.toHaveValue('userAllowed');
		await expect
			.element(page.getByRole('combobox', { name: 'Region policy' }))
			.toHaveValue('fixed');
		await expect.element(page.getByCSS('#fixed-REGION')).toHaveValue('us-west-2');
		await expect
			.element(page.getByRole('combobox', { name: 'Org header policy' }))
			.toHaveValue('prohibited');
		await expect.element(page.getByRole('button', { name: 'Save' })).toBeVisible();
	});
});
