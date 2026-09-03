import JsonSchemaForm from './JsonSchemaForm.svelte';
import type { JSONSchema } from './json-schema';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const schema: JSONSchema = {
	type: 'object',
	required: ['name', 'count'],
	properties: {
		name: { type: 'string', minLength: 3 },
		count: { type: 'integer', minimum: 1 },
		mode: { type: 'string', enum: ['safe', 'fast'] },
		tags: { type: 'array', items: { type: 'string' } },
		options: {
			type: 'object',
			properties: { enabled: { type: 'boolean' } }
		}
	}
};

describe('JsonSchemaForm', () => {
	it('renders nested generated controls and reports only valid arguments', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, { schema, onvalidchange });

		await expect.element(page.getByLabelText('name *')).toBeVisible();
		await expect.element(page.getByLabelText('count *')).toBeVisible();
		await expect.element(page.getByLabelText('mode', { exact: true })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add item' })).toBeVisible();

		await page.getByLabelText('name *').fill('valid');
		await page.getByLabelText('count *').fill('2');
		await page.getByRole('button', { name: 'Add item' }).click();
		await page.getByLabelText('tags item 1 *').fill('first');

		await vi.waitFor(() => {
			expect(onvalidchange).toHaveBeenLastCalledWith(
				expect.objectContaining({ name: 'valid', count: 2, tags: ['first'] })
			);
		});
	});

	it('validates raw JSON syntax and schema constraints', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, { schema, onvalidchange });
		await page.getByRole('button', { name: 'Raw JSON' }).click();

		const raw = page.getByLabelText('Arguments JSON');
		await raw.fill('{');
		await expect.element(page.getByText(/Invalid JSON:/)).toBeVisible();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));

		await raw.fill('{"name":"ok","count":0}');
		await expect.element(page.getByText('name must contain at least 3 characters')).toBeVisible();
		await expect.element(page.getByText('count must be at least 1')).toBeVisible();

		await raw.fill('{"name":"valid","count":1}');
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ name: 'valid', count: 1 })
		);
	});
});
