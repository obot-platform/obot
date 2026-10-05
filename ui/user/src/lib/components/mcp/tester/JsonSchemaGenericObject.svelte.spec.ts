import JsonSchemaForm from './JsonSchemaForm.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('generic object arguments', () => {
	it('accepts arbitrary nested arguments for a tool call and rejects invalid input', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					name: { type: 'string' },
					revision: { type: 'string' },
					arguments: { type: 'object' }
				},
				required: ['name', 'revision']
			},
			onvalidchange
		});

		await page.getByLabelText('name *').fill('server_tool');
		await page.getByLabelText('revision *').fill('abc123');
		const argumentsJSON = page.getByLabelText('arguments', { exact: true });
		await expect.element(argumentsJSON).toBeVisible();
		// The freeform editor sits directly in the main arguments group.
		expect(page.getByCSS('fieldset').all()).toHaveLength(1);
		await expect.element(page.getByText('arguments JSON', { exact: true })).not.toBeInTheDocument();
		await argumentsJSON.fill('{"query":"hello","limit":2,"options":{"exact":true}}');
		const expected = {
			name: 'server_tool',
			revision: 'abc123',
			arguments: { query: 'hello', limit: 2, options: { exact: true } }
		};
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(expected));

		await argumentsJSON.fill('{');
		await expect.element(page.getByText(/Invalid JSON:/)).toBeVisible();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));

		await argumentsJSON.fill('[]');
		await expect.element(page.getByText('arguments must be an object')).toBeVisible();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));

		await argumentsJSON.fill(JSON.stringify(expected.arguments));
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(expected));
	});

	it('supports an object schema without any named fields', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, { schema: { type: 'object' }, onvalidchange });

		await page.getByLabelText('Arguments *').fill('{"custom":42}');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ custom: 42 }));
	});
});
