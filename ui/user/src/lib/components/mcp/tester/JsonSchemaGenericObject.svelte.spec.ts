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
		await expect
			.element(argumentsJSON)
			.toHaveValue('{"query":"hello","limit":2,"options":{"exact":true}}');
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

		await argumentsJSON.fill('{"query":"","limit":2}');
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({
				name: 'server_tool',
				revision: 'abc123',
				arguments: { query: '', limit: 2 }
			})
		);

		await argumentsJSON.fill('');
		await expect.element(page.getByText(/Invalid JSON:/)).toBeVisible();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));
	});

	it('supports an object schema without any named fields', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, { schema: { type: 'object' }, onvalidchange });

		await page.getByLabelText('Arguments *').fill('{"custom":42}');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ custom: 42 }));
	});

	it('updates a reused editor after removing an earlier array item', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: { items: { type: 'array', items: { type: 'object' } } }
			},
			onvalidchange
		});

		await page.getByRole('button', { name: 'Add item' }).click();
		await page.getByLabelText('items item 1 *').fill('{"removed":1}');
		await page.getByRole('button', { name: 'Add item' }).click();
		await page.getByLabelText('items item 2 *').fill('{"remaining":2}');
		await page.getByRole('button', { name: 'Remove items item 1' }).click();

		await expect
			.element(page.getByLabelText('items item 1 *'))
			.toHaveValue('{\n  "remaining": 2\n}');
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ items: [{ remaining: 2 }] })
		);
	});

	it('resets a nullable freeform object editor when switched back from null', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: { data: { type: ['object', 'null'] } }
			},
			onvalidchange
		});

		const editor = page.getByLabelText('data', { exact: true });
		await editor.fill('{"old":true}');
		const useNull = page.getByRole('checkbox', { name: 'Use null for data' });
		await useNull.click();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ data: null }));
		await useNull.click();

		await expect.element(editor).toHaveValue('{}');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ data: {} }));
	});
});
