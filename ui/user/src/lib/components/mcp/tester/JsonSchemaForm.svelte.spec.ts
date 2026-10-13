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
	it('generates the fetch_content form and preserves or edits its nullable backend', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					backend: {
						anyOf: [{ type: 'string' }, { type: 'null' }],
						default: null,
						title: 'Backend'
					},
					max_length: { default: 8000, title: 'Max Length', type: 'integer' },
					start_index: { default: 0, title: 'Start Index', type: 'integer' },
					url: { title: 'Url', type: 'string' }
				},
				required: ['url'],
				title: 'fetch_contentArguments'
			},
			onvalidchange
		});

		await expect.element(page.getByRole('button', { name: 'Generated form' })).toBeVisible();
		await expect.element(page.getByLabelText('Max Length')).toHaveValue(8000);
		await expect.element(page.getByLabelText('Start Index')).toHaveValue(0);
		const backend = page.getByLabelText('Backend', { exact: true });
		const useNull = page.getByRole('checkbox', { name: 'Use null for Backend' });
		await expect.element(useNull).toBeChecked();
		await expect.element(backend).toBeDisabled();
		await page.getByLabelText('Url *').fill('https://example.com');
		const defaults = { url: 'https://example.com', max_length: 8000, start_index: 0 };
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ ...defaults, backend: null })
		);

		await useNull.click();
		await backend.fill('requests');
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ ...defaults, backend: 'requests' })
		);
		await backend.fill('');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(defaults));
		await useNull.click();
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ ...defaults, backend: null })
		);

		await page.getByRole('button', { name: 'Raw JSON' }).click();
		const raw = page.getByLabelText('Arguments JSON');
		expect(JSON.parse((raw.element() as HTMLTextAreaElement).value)).toEqual({
			...defaults,
			backend: null
		});
		await raw.fill(JSON.stringify({ ...defaults, backend: 42 }));
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));
		await raw.fill(JSON.stringify({ ...defaults, backend: 'requests' }));
		await page.getByRole('button', { name: 'Generated form' }).click();
		await expect.element(useNull).not.toBeChecked();
		await expect.element(backend).toHaveValue('requests');
	});

	it('allows required nullable type arrays to switch between null and a constrained value', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				required: ['count'],
				properties: { count: { type: ['integer', 'null'], minimum: 2, default: null } }
			},
			onvalidchange
		});
		const useNull = page.getByRole('checkbox', { name: 'Use null for count' });
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ count: null }));
		await useNull.click();
		await expect.element(page.getByLabelText('count *')).toHaveValue(2);
		await page.getByLabelText('count *').fill('1');
		await expect.element(page.getByText('count must be at least 2')).toBeVisible();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));
		await useNull.click();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ count: null }));
	});

	it('restores the non-null branch default when toggling off a null union default', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					count: { anyOf: [{ type: 'integer', default: 5 }, { type: 'null' }], default: null }
				}
			},
			onvalidchange
		});
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ count: null }));
		await page.getByRole('checkbox', { name: 'Use null for count' }).click();
		await expect.element(page.getByLabelText('count', { exact: true })).toHaveValue(5);
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ count: 5 }));
	});

	it('renders the first union member and keeps the others reachable in Raw JSON', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				required: ['value'],
				properties: { value: { anyOf: [{ type: 'string' }, { type: 'integer' }] } }
			},
			onvalidchange
		});
		await expect.element(page.getByRole('button', { name: 'Generated form' })).toBeVisible();
		await expect
			.element(page.getByText('Also accepts integer, which can be entered in Raw JSON.'))
			.toBeVisible();
		await page.getByLabelText('value *').fill('abc');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ value: 'abc' }));

		await page.getByRole('button', { name: 'Raw JSON' }).click();
		await page.getByLabelText('Arguments JSON').fill('{"value":7}');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ value: 7 }));
		await page.getByRole('button', { name: 'Generated form' }).click();
		await expect.element(page.getByLabelText('value *')).toHaveValue(7);
		await expect
			.element(page.getByText('Also accepts string, which can be entered in Raw JSON.'))
			.toBeVisible();
	});

	it('keeps a union member control while its input is cleared or holds an unmatched value', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					value: { anyOf: [{ type: 'string' }, { type: 'integer' }] },
					count: { anyOf: [{ type: 'string' }, { type: 'integer' }], default: 5 }
				}
			},
			onvalidchange
		});
		await page.getByRole('button', { name: 'Raw JSON' }).click();
		await page.getByLabelText('Arguments JSON').fill('{"value":7}');
		await page.getByRole('button', { name: 'Generated form' }).click();

		const value = page.getByLabelText('value', { exact: true });
		await expect.element(value).toHaveValue(7);
		await value.clear();
		await expect.element(value).toHaveAttribute('type', 'number');
		await value.fill('8');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ value: 8 }));

		const count = page.getByLabelText('count', { exact: true });
		await expect.element(count).toHaveAttribute('type', 'number');
		await count.fill('2.5');
		await expect.element(count).toHaveAttribute('type', 'number');
		await expect.element(count).toHaveValue(2.5);
	});

	it('describes a union control with its alternatives hint', async () => {
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					value: { anyOf: [{ type: 'string' }, { type: 'integer' }], description: 'A value' }
				}
			}
		});
		await expect
			.element(page.getByLabelText('value', { exact: true }))
			.toHaveAccessibleDescription(
				'A value Also accepts integer, which can be entered in Raw JSON.'
			);
	});

	it('keeps Raw JSON when its value cannot be shown in the generated form', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					value: { anyOf: [{ type: 'string' }, { type: 'object', properties: { nested: {} } }] }
				}
			},
			onvalidchange
		});
		const generated = page.getByRole('button', { name: 'Generated form' });
		await page.getByRole('button', { name: 'Raw JSON' }).click();
		await page.getByLabelText('Arguments JSON').fill('{"value":{"nested":1}}');
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ value: { nested: 1 } })
		);
		await expect.element(generated).toBeDisabled();
		await expect
			.element(generated)
			.toHaveAccessibleDescription('This value can only be edited in Raw JSON.');

		await page.getByLabelText('Arguments JSON').fill('{"value":"text"}');
		await expect.element(generated).toBeEnabled();
		await expect
			.element(page.getByText('This value can only be edited in Raw JSON.'))
			.not.toBeInTheDocument();
		await generated.click();
		await expect.element(page.getByLabelText('value', { exact: true })).toHaveValue('text');
	});

	it('renders the same-typed union member that a Raw JSON value satisfies', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					value: {
						anyOf: [
							{ type: 'object', properties: { a: { type: 'string' } }, required: ['a'] },
							{ type: 'object', properties: { b: { type: 'string' } }, required: ['b'] }
						]
					}
				}
			},
			onvalidchange
		});
		await page.getByRole('button', { name: 'Raw JSON' }).click();
		await page.getByLabelText('Arguments JSON').fill('{"value":{"b":"x"}}');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ value: { b: 'x' } }));
		await page.getByRole('button', { name: 'Generated form' }).click();
		await expect.element(page.getByLabelText('b *')).toHaveValue('x');
		await expect.element(page.getByLabelText('a *')).not.toBeInTheDocument();
	});

	it('keeps a boolean member after toggling null when the union default is a string', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					mode: {
						anyOf: [{ type: 'boolean' }, { type: 'string' }, { type: 'null' }],
						default: 'auto'
					}
				}
			},
			onvalidchange
		});
		await page.getByRole('button', { name: 'Raw JSON' }).click();
		await page.getByLabelText('Arguments JSON').fill('{"mode":true}');
		await page.getByRole('button', { name: 'Generated form' }).click();
		const useNull = page.getByRole('checkbox', { name: 'Use null for mode' });
		await useNull.click();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ mode: null }));
		await useNull.click();
		await expect
			.element(page.getByLabelText('mode', { exact: true }))
			.toHaveAttribute('type', 'checkbox');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ mode: false }));
	});

	it('offers a boolean toggle and a null switch for boolean, string, or null unions', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				properties: {
					reply_all: {
						anyOf: [{ type: 'boolean' }, { type: 'string' }, { type: 'null' }],
						default: false
					}
				}
			},
			onvalidchange
		});
		const toggle = page.getByLabelText('reply_all', { exact: true });
		await expect.element(toggle).not.toBeChecked();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ reply_all: false }));
		await toggle.click();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ reply_all: true }));
		await page.getByRole('checkbox', { name: 'Use null for reply_all' }).click();
		await expect.element(toggle).toBeDisabled();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ reply_all: null }));
	});

	it('enforces anyOf sibling constraints in generated and raw input modes', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				required: ['name'],
				properties: {
					name: { anyOf: [{ type: 'string' }, { type: 'null' }], minLength: 3 }
				}
			},
			onvalidchange
		});
		await page.getByLabelText('name *').fill('x');
		await expect.element(page.getByText('name must contain at least 3 characters')).toBeVisible();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));
		await page.getByLabelText('name *').fill('abc');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ name: 'abc' }));
		await page.getByRole('button', { name: 'Raw JSON' }).click();
		await page.getByLabelText('Arguments JSON').fill('{"name":"x"}');
		await expect.element(page.getByText('name must contain at least 3 characters')).toBeVisible();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));
		await page.getByLabelText('Arguments JSON').fill('{"name":null}');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ name: null }));
	});

	it.each([{ enum: [null] }, { const: null }])(
		'uses Raw JSON when only null is allowed: %j',
		async (constraint) => {
			const onvalidchange = vi.fn();
			render(JsonSchemaForm, {
				schema: {
					type: 'object',
					required: ['name'],
					properties: {
						name: { anyOf: [{ type: 'string' }, { type: 'null' }], ...constraint }
					}
				},
				onvalidchange
			});
			await expect
				.element(page.getByRole('button', { name: 'Generated form' }))
				.not.toBeInTheDocument();
			await expect.element(page.getByLabelText('Arguments JSON')).toBeVisible();
			await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ name: null }));
		}
	);

	it('uses Raw JSON for overlapping nullable object constraints and requires both sets of fields', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				required: ['settings'],
				properties: {
					settings: {
						anyOf: [
							{ type: 'object', properties: { first: { type: 'string' } }, required: ['first'] },
							{ type: 'null' }
						],
						properties: { second: { type: 'string' } },
						required: ['second'],
						default: null
					}
				}
			},
			onvalidchange
		});
		await expect
			.element(page.getByRole('button', { name: 'Generated form' }))
			.not.toBeInTheDocument();
		const raw = page.getByLabelText('Arguments JSON');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ settings: null }));
		await raw.fill('{"settings":{"second":"two"}}');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));
		await raw.fill('{"settings":{"first":"one"}}');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));
		await expect.element(page.getByText('settings.second is required')).toBeVisible();
		await raw.fill('{"settings":{"first":"one","second":"two"}}');
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ settings: { first: 'one', second: 'two' } })
		);
	});

	it.each([{ enum: ['allowed'] }, { const: 'allowed' }])(
		'uses Raw JSON when union constraints prohibit null: %j',
		async (constraint) => {
			const onvalidchange = vi.fn();
			render(JsonSchemaForm, {
				schema: {
					type: 'object',
					required: ['name'],
					properties: { name: { anyOf: [{ type: 'string' }, { type: 'null' }], ...constraint } }
				},
				onvalidchange
			});
			await expect
				.element(page.getByRole('button', { name: 'Generated form' }))
				.not.toBeInTheDocument();
			await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ name: 'allowed' }));
			await page.getByLabelText('Arguments JSON').fill('{"name":null}');
			await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));
		}
	);

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

	it('returns a cleared optional numeric field to its unset state', async () => {
		const optional: JSONSchema = {
			type: 'object',
			required: ['name'],
			properties: {
				name: { type: 'string', minLength: 3 },
				retries: { type: 'integer', minimum: 1 }
			}
		};
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, { schema: optional, onvalidchange });

		await page.getByLabelText('name *').fill('valid');
		await page.getByLabelText('retries', { exact: true }).fill('3');
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ name: 'valid', retries: 3 })
		);

		// Clearing it must drop the property rather than submit NaN or hold the form invalid.
		await page.getByLabelText('retries', { exact: true }).fill('');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ name: 'valid' }));
		await expect.element(page.getByText('retries must be a number')).not.toBeInTheDocument();
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

	it('clears a stale raw parse error when returning to the generated form', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, { schema, onvalidchange });

		await page.getByRole('button', { name: 'Raw JSON' }).click();
		const raw = page.getByLabelText('Arguments JSON');
		await raw.fill('{"name":"valid","count":1}');
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ name: 'valid', count: 1 })
		);

		await raw.fill('{"name":"valid","count":1,');
		await expect.element(page.getByText(/Invalid JSON:/)).toBeVisible();
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith(undefined));

		await page.getByRole('button', { name: 'Generated form' }).click();
		await expect.element(page.getByText(/Invalid JSON:/)).not.toBeInTheDocument();
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ name: 'valid', count: 1 })
		);
	});

	it('leaves an optional enum unset instead of displaying a value it will not send', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				required: ['count'],
				properties: {
					count: { type: 'integer', minimum: 1 },
					mode: { type: 'string', enum: ['safe', 'fast'] }
				}
			},
			onvalidchange
		});

		const mode = page.getByLabelText('mode', { exact: true });
		await expect.element(mode).toBeVisible();
		const select = mode.element() as HTMLSelectElement;
		expect(select.selectedOptions[0]?.textContent?.trim()).toBe('Not set');
		await vi.waitFor(() => expect(onvalidchange).toHaveBeenLastCalledWith({ count: 1 }));

		await mode.selectOptions(page.getByRole('option', { name: 'safe' }));
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith(expect.objectContaining({ mode: 'safe' }))
		);
	});

	it('shows a server pattern as a hint without handing it to constraint validation', async () => {
		const onvalidchange = vi.fn();
		render(JsonSchemaForm, {
			schema: {
				type: 'object',
				required: ['slug'],
				properties: { slug: { type: 'string', pattern: '^(a+)+$' } }
			},
			onvalidchange
		});

		const hint = page.getByText(/^Must match/);
		await expect.element(hint).toBeVisible();
		expect(hint.element().textContent).toContain('^(a+)+$');

		const slug = page.getByLabelText('slug *');
		expect(slug.element()).not.toHaveAttribute('pattern');
		await slug.fill(`${'a'.repeat(40)}b`);
		await vi.waitFor(() =>
			expect(onvalidchange).toHaveBeenLastCalledWith({ slug: `${'a'.repeat(40)}b` })
		);
	});
});
