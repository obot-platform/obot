import type { MCPCatalogEntry, MCPConfig } from '$lib/services';
import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
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
	it('lets non-administrator editors select the override', async () => {
		await preparePageData({ profile: createMockProfile([]) });
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry({ config: [] }));
		await page.getByRole('checkbox', { name: 'Force single-user' }).click();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([], true);
	});
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

		await page
			.getByRole('combobox', { name: 'API token policy' })
			.selectOptions('Provided at connection');
		await page.getByRole('combobox', { name: 'Region policy' }).selectOptions('Preconfigured');
		await page.getByRole('combobox', { name: 'Org header policy' }).selectOptions('Ignore');

		await page.getByCSS('#fixed-REGION').fill('us-east-1');
		await page.getByRole('button', { name: 'Next' }).click();

		await vi.waitFor(() => expect(onNext).toHaveBeenCalledOnce());
		expect(onNext).toHaveBeenCalledWith(
			[
				{ key: 'API_TOKEN', policy: 'userAllowed' },
				{ key: 'REGION', policy: 'fixed', value: 'us-east-1' },
				{ key: 'X-Org', policy: 'prohibited' }
			],
			false
		);
	});

	it('defaults to unchecked and saves the single-user choice without configuration fields', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const onClose = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext, onClose });
		result.component.open(configurableEntry({ config: [] }));
		const checkbox = page.getByRole('checkbox', { name: 'Force single-user' });
		await expect.element(checkbox).not.toBeChecked();
		await checkbox.click();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([], true);
		await vi.waitFor(() => expect(onClose).toHaveBeenCalledOnce());
		result.component.open(configurableEntry({ config: [] }));
		await expect.element(checkbox).not.toBeChecked();
	});

	it('clears and hides the choice when a non-header policy becomes user-allowed', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry({ config: [field({ key: 'TOKEN', name: 'Token' })] }), {
			forceSingleUser: true
		});
		const checkbox = page.getByRole('checkbox', { name: 'Force single-user' });
		await expect.element(checkbox).toBeChecked();
		const policy = page.getByRole('combobox', { name: 'Token policy' });
		await policy.selectOptions('Provided at connection');
		await expect.element(checkbox).not.toBeInTheDocument();
		await policy.selectOptions('Ignore');
		await expect.element(checkbox).not.toBeChecked();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([{ key: 'TOKEN', policy: 'prohibited' }], false);
	});

	it('preserves and saves the override with user-allowed headers', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const onClose = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext, onClose });
		const entry = configurableEntry({
			config: [field({ key: 'HEADER', name: 'Header', usage: 'header' })]
		});
		result.component.open(entry, { forceSingleUser: true });
		const checkbox = page.getByRole('checkbox', { name: 'Force single-user' });
		await page
			.getByRole('combobox', { name: 'Header policy' })
			.selectOptions('Provided at connection');
		await expect.element(checkbox).toBeChecked();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([{ key: 'HEADER', policy: 'userAllowed' }], true);
		await vi.waitFor(() => expect(onClose).toHaveBeenCalledOnce());

		result.component.open(entry, {
			configuration: [{ key: 'HEADER', policy: 'userAllowed' }],
			forceSingleUser: true
		});
		await expect.element(checkbox).toBeChecked();
		await checkbox.click();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenLastCalledWith([{ key: 'HEADER', policy: 'userAllowed' }], false);
	});

	it('ignores a saved override when existing configuration is user-allowed', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry({ config: [field({ key: 'TOKEN', name: 'Token' })] }), {
			configuration: [{ key: 'TOKEN', policy: 'userAllowed' }],
			forceSingleUser: true
		});
		await expect
			.element(page.getByRole('checkbox', { name: 'Force single-user' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([{ key: 'TOKEN', policy: 'userAllowed' }], false);
	});

	it('omits Ignore from required field policies', async () => {
		await preparePageData();
		const result = await render(VMcpComponentConfigurationDialog);
		result.component.open(configurableEntry());

		const requiredPolicy = page.getByRole('combobox', { name: 'API token policy' });
		const optionalPolicy = page.getByRole('combobox', { name: 'Org header policy' });
		await expect.element(requiredPolicy).toBeVisible();
		expect(
			Array.from((requiredPolicy.element() as HTMLSelectElement).options).map(
				(option) => option.value
			)
		).toEqual(['fixed', 'userAllowed']);
		expect(
			Array.from((optionalPolicy.element() as HTMLSelectElement).options).map(
				(option) => option.value
			)
		).toEqual(['fixed', 'userAllowed', 'prohibited']);
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
