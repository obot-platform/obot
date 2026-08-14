import type { MCPSubField } from '$lib/services';
import CatalogConfigureForm from './CatalogConfigureForm.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const options = [
	{ name: 'United States', value: 'us', description: 'US endpoint' },
	{ name: 'Europe', value: 'eu', description: 'Europe endpoint' }
];

function field(key: string, name: string): MCPSubField & { value: string } {
	return {
		key,
		name,
		description: `${name} description`,
		value: 'stale',
		required: true,
		sensitive: false,
		options
	};
}

const scenarios = [
	{
		name: 'standalone environment',
		label: 'Service Region',
		id: 'SERVICE_REGION',
		form: { envs: [field('SERVICE_REGION', 'Service Region')] }
	},
	{
		name: 'standalone header',
		label: 'Request Region',
		id: 'REQUEST_REGION',
		form: { headers: [field('REQUEST_REGION', 'Request Region')] }
	},
	{
		name: 'composite environment',
		label: 'Component Region',
		id: 'component-COMPONENT_REGION',
		form: {
			componentConfigs: {
				component: {
					name: 'Component server',
					envs: [field('COMPONENT_REGION', 'Component Region')]
				}
			}
		}
	},
	{
		name: 'composite header',
		label: 'Component Header Region',
		id: 'component-COMPONENT_HEADER_REGION',
		form: {
			componentConfigs: {
				component: {
					name: 'Component server',
					headers: [field('COMPONENT_HEADER_REGION', 'Component Header Region')]
				}
			}
		}
	}
];

describe('CatalogConfigureForm.svelte configuration options', () => {
	it.each(scenarios)('handles $name options', async ({ form, id, label }) => {
		const onSave = vi.fn();
		const result = await render(CatalogConfigureForm, {
			form,
			name: 'Catalog server',
			onSave,
			animate: null
		});
		await result.component.open();

		const select = page.getByRole('combobox', { name: label });
		const fieldLabel = page.getByCSS(`#${id}-label`);

		await page.getByRole('button', { name: 'Save' }).click();

		expect(onSave).not.toHaveBeenCalled();
		await expect
			.element(page.getByText('Please complete all configuration fields with valid values.'))
			.toBeInTheDocument();
		await expect.element(fieldLabel).toHaveClass(/text-error/);
		await expect.element(select).toHaveClass(/border-error/);

		await select.click();
		await page.getByRole('button', { name: 'Europe', exact: true }).click();
		await result.rerender({ form: { ...form } });

		await expect.element(select).toHaveTextContent('Europe');
		await expect.element(page.getByText('Europe endpoint', { exact: true })).toBeInTheDocument();
		await expect.element(fieldLabel).not.toHaveClass(/text-error/);
		await expect.element(select).not.toHaveClass(/border-error/);

		// Select's single-value clear control has no accessible name.
		await page.getByCSS(`#${id} + button`).click();
		await result.rerender({ form: { ...form } });

		await expect.element(select).toHaveTextContent('Select a value');
		await expect
			.element(page.getByText('Europe endpoint', { exact: true }))
			.not.toBeInTheDocument();
		await expect.element(fieldLabel).toHaveClass(/text-error/);
		await expect.element(select).toHaveClass(/border-error/);
	});
});
