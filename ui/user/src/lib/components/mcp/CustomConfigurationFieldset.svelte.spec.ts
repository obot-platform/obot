import type { MCPSubField } from '$lib/services';
import CustomConfigurationFieldset from './CustomConfigurationFieldset.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function field(overrides: Partial<MCPSubField> = {}): MCPSubField {
	return {
		key: '',
		name: '',
		description: '',
		value: '',
		required: false,
		sensitive: false,
		...overrides
	};
}

async function renderFieldset(
	overrides: Partial<{
		data: MCPSubField;
		serverUserType: 'singleUser' | 'multiUser';
		readonly: boolean;
		showRequired: boolean;
		showInvalid: boolean;
	}> = {}
) {
	await render(CustomConfigurationFieldset, {
		id: 'test',
		data: field(),
		serverUserType: 'multiUser',
		...overrides
	});
}

describe('CustomConfigurationFieldset.svelte', () => {
	it('marks Key and Name as required before validation for user-supplied fields', async () => {
		await renderFieldset();

		await expect
			.element(page.getByRole('textbox', { name: 'Key (required)' }))
			.toHaveAttribute('aria-required', 'true');
		await expect
			.element(page.getByRole('textbox', { name: 'Name (required)' }))
			.toHaveAttribute('aria-required', 'true');
		await expect
			.element(page.getByRole('textbox', { name: 'Key (required)' }))
			.not.toHaveAttribute('aria-invalid', 'true');
		await expect
			.element(page.getByRole('textbox', { name: 'Name (required)' }))
			.not.toHaveAttribute('aria-invalid', 'true');
	});

	it('keeps Key and static Value required after they are filled', async () => {
		await renderFieldset({
			data: field({ key: 'API_KEY', value: 'secret', required: true }),
			showRequired: true
		});

		await expect
			.element(page.getByRole('textbox', { name: 'Key (required)' }))
			.toHaveAttribute('aria-required', 'true');
		await expect
			.element(page.getByLabelText('Static Value'))
			.toHaveAttribute('aria-required', 'true');
		await expect
			.element(page.getByRole('textbox', { name: 'Key (required)' }))
			.not.toHaveAttribute('aria-invalid', 'true');
		await expect
			.element(page.getByLabelText('Static Value'))
			.not.toHaveAttribute('aria-invalid', 'true');
	});

	it('marks an empty static Value invalid only after validation', async () => {
		await renderFieldset({ showRequired: true });

		await page.getByCSS('#env-value-type-test').click();
		await page.getByRole('button', { name: 'Static', exact: true }).click();

		await expect
			.element(page.getByLabelText('Static Value'))
			.toHaveAttribute('aria-required', 'true');
		await expect
			.element(page.getByLabelText('Static Value'))
			.toHaveAttribute('aria-invalid', 'true');
	});

	it('clears a secret binding when switching to options', async () => {
		const data: MCPSubField & { secretBindingSource?: string } = field({
			key: 'API_KEY',
			secretBinding: { name: 'api-credentials', key: 'api-key' }
		});
		data.secretBindingSource = 'secret';
		await renderFieldset({ data });

		await page.getByCSS('#env-value-type-test').click();
		await page.getByRole('button', { name: 'Options', exact: true }).click();

		expect(data.secretBinding).toBeUndefined();
		expect(data.secretBindingSource).toBe('value');
		expect(data.options).toEqual([{ name: '', value: '', description: '' }]);
	});

	it('shows an error on duplicate option values after invalid validation', async () => {
		await renderFieldset({
			data: field({
				key: 'REGION',
				options: [
					{ name: 'United States', value: 'us' },
					{ name: 'US fallback', value: 'us' }
				]
			}),
			showInvalid: true
		});

		await expect
			.element(page.getByCSS('#env-option-value-test-0'))
			.not.toHaveAttribute('aria-invalid', 'true');
		await expect
			.element(page.getByCSS('#env-option-value-test-1'))
			.toHaveAttribute('aria-invalid', 'true');
		await expect
			.element(page.getByCSS('#env-option-value-test-1-error'))
			.toHaveTextContent('Option values must be unique.');
	});

	it('omits aria-required when the fieldset is readonly', async () => {
		await renderFieldset({
			data: field({ key: 'API_KEY', value: 'secret', required: true }),
			readonly: true
		});

		await expect
			.element(page.getByRole('textbox', { name: 'Key' }))
			.not.toHaveAttribute('aria-required');
		await expect.element(page.getByLabelText('Static Value')).not.toHaveAttribute('aria-required');
	});
});
