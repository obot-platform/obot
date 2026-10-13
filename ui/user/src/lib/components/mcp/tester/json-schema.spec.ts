import {
	defaultJSONSchemaValue,
	generatedFormRepresents,
	jsonValuesEqual,
	resolveUnionSchema,
	pruneClearedProperties,
	supportsGeneratedForm,
	validateJSONSchema,
	type JSONSchema
} from './json-schema';
import { describe, expect, it } from 'vitest';

const schema: JSONSchema = {
	type: 'object',
	required: ['name', 'settings'],
	properties: {
		name: { type: 'string', minLength: 3, pattern: '^[a-z]+$' },
		count: { type: 'integer', minimum: 1, maximum: 5 },
		tags: { type: 'array', minItems: 1, items: { type: 'string' } },
		settings: {
			type: 'object',
			required: ['enabled'],
			properties: { enabled: { type: 'boolean', default: true } }
		}
	}
};

describe('MCP tester JSON Schema support', () => {
	it('builds nested defaults for required and explicitly defaulted properties', () => {
		expect(defaultJSONSchemaValue(schema)).toEqual({
			name: '',
			settings: { enabled: true }
		});
		expect(supportsGeneratedForm(schema)).toBe(true);
	});

	it('copies object and array defaults held in proxies such as Svelte $state', () => {
		// structuredClone rejects proxies, which the tester's tool cache hands out.
		const domains = new Proxy<string[]>([], {});
		const filter = new Proxy({ site: 'example.com' }, {});
		const proxied: JSONSchema = {
			type: 'object',
			properties: {
				include_domains: { type: 'array', items: { type: 'string' }, default: domains },
				filter: { type: 'object', const: filter },
				options: { type: 'object', enum: [filter] }
			},
			required: ['filter', 'options']
		};

		const value = defaultJSONSchemaValue(proxied) as Record<string, unknown>;
		expect(value).toEqual({
			include_domains: [],
			filter: { site: 'example.com' },
			options: { site: 'example.com' }
		});
		expect(value.include_domains).not.toBe(domains);
		expect(value.filter).not.toBe(filter);
	});

	it('validates required, nested, array, string, and numeric constraints', () => {
		expect(
			validateJSONSchema(schema, {
				name: 'A',
				count: 8,
				tags: [],
				settings: {}
			})
		).toEqual(
			expect.arrayContaining([
				'name must contain at least 3 characters',
				'count must be at most 5',
				'tags must contain at least 1 items',
				'settings.enabled is required'
			])
		);
		expect(
			validateJSONSchema(schema, {
				name: 'valid',
				count: 2,
				tags: ['one'],
				settings: { enabled: true }
			})
		).toEqual([]);
	});

	it('never runs a server-supplied pattern, so a catastrophic one cannot hang the tab', () => {
		expect(validateJSONSchema(schema, { name: 'ABC', settings: { enabled: true } })).toEqual([]);

		const catastrophic: JSONSchema = { type: 'string', pattern: '^(a+)+$' };
		const started = performance.now();
		expect(validateJSONSchema(catastrophic, `${'a'.repeat(40)}b`)).toEqual([]);
		expect(performance.now() - started).toBeLessThan(100);
	});

	it('accepts every member of a union type and renders the first non-null type', () => {
		const nullable: JSONSchema = {
			type: 'object',
			required: ['label'],
			properties: {
				label: { type: ['string', 'null'], minLength: 2 },
				amount: { type: ['integer', 'string'] }
			}
		};

		expect(validateJSONSchema(nullable, { label: null, amount: 3 })).toEqual([]);
		expect(validateJSONSchema(nullable, { label: 'ok', amount: 'three' })).toEqual([]);
		expect(validateJSONSchema(nullable, { label: 'a' })).toEqual([
			'label must contain at least 2 characters'
		]);
		expect(validateJSONSchema(nullable, { label: null, amount: null })).toEqual([
			'amount must not be null'
		]);
		expect(validateJSONSchema(nullable, { label: null, amount: true })).toEqual([
			'amount must be one of these types: integer, string'
		]);
		expect(supportsGeneratedForm(nullable)).toBe(true);
		expect(resolveUnionSchema(nullable.properties!.amount)).toEqual({
			schema: { type: 'integer' },
			nullable: false,
			alternatives: ['string'],
			index: 0
		});
	});

	it.each([
		{ anyOf: [{ type: 'string', minLength: 2 }, { type: 'null' }] },
		{ anyOf: [{ type: 'null' }, { type: 'string', minLength: 2 }] },
		{ type: ['string', 'null'], minLength: 2 }
	])('supports nullable fields and validates both members: %j', (property) => {
		const nullable: JSONSchema = {
			type: 'object',
			required: ['label'],
			properties: { label: { ...property, default: null } }
		};
		expect(supportsGeneratedForm(nullable)).toBe(true);
		expect(defaultJSONSchemaValue(nullable)).toEqual({ label: null });
		expect(validateJSONSchema(nullable, { label: null })).toEqual([]);
		expect(validateJSONSchema(nullable, { label: 'ok' })).toEqual([]);
		expect(validateJSONSchema(nullable, { label: 'a' })).not.toEqual([]);
		expect(validateJSONSchema(nullable, { label: 42 })).not.toEqual([]);
		expect(validateJSONSchema(nullable, {})).toEqual(['label is required']);
	});

	it('uses nullable branch defaults and prunes nested optional fields', () => {
		const nullable: JSONSchema = {
			anyOf: [
				{
					type: 'object',
					required: ['count'],
					properties: {
						count: { type: 'integer', default: 3 },
						label: { type: 'string' }
					}
				},
				{ type: 'null' }
			]
		};
		expect(supportsGeneratedForm(nullable)).toBe(true);
		expect(defaultJSONSchemaValue(nullable)).toEqual({ count: 3 });
		expect(pruneClearedProperties(nullable, { count: 3, label: '' })).toEqual({ count: 3 });
		expect(pruneClearedProperties(nullable, null)).toBeNull();
		expect(
			defaultJSONSchemaValue({ anyOf: [{ type: 'integer', default: 5 }, { type: 'null' }] })
		).toBe(5);
	});

	it('preserves the branch default when the nullable union defaults to null', () => {
		const nullable: JSONSchema = {
			anyOf: [{ type: 'integer', default: 5 }, { type: 'null' }],
			default: null
		};
		expect(defaultJSONSchemaValue(nullable)).toBeNull();
		expect(defaultJSONSchemaValue(resolveUnionSchema(nullable)!.schema)).toBe(5);
		expect(defaultJSONSchemaValue(resolveUnionSchema({ ...nullable, default: 0 })!.schema)).toBe(0);
		expect(nullable.default).toBeNull();
	});

	it('still applies enum and const constraints to null values', () => {
		expect(validateJSONSchema({ type: ['string', 'null'], enum: ['allowed'] }, null)).toEqual([
			'Value must be one of the allowed values'
		]);
		expect(validateJSONSchema({ type: ['string', 'null'], const: 'allowed' }, null)).toEqual([
			'Value must equal "allowed"'
		]);
	});

	it('renders the first non-null anyOf member and validates sibling constraints', () => {
		expect(supportsGeneratedForm({ anyOf: [{ type: 'string' }, { type: 'integer' }] })).toBe(true);
		expect(
			supportsGeneratedForm({
				anyOf: [{ type: 'string' }, { type: 'integer' }, { type: 'null' }]
			})
		).toBe(true);
		expect(
			validateJSONSchema(
				{
					anyOf: [{ type: 'string' }, { type: 'null' }],
					minLength: 3,
					enum: ['allowed', null]
				},
				'other'
			)
		).toEqual(['Value must be one of the allowed values']);
	});

	it('prefers the member matching the current value, then the schema default, then the first', () => {
		const union: JSONSchema = {
			anyOf: [{ type: 'boolean' }, { type: 'string' }],
			default: false,
			description: 'Whether to reply to all'
		};
		const boolean = {
			schema: { type: 'boolean', default: false, description: 'Whether to reply to all' },
			nullable: false,
			alternatives: ['string'],
			index: 0
		};
		expect(resolveUnionSchema(union)).toEqual(boolean);
		expect(resolveUnionSchema(union, true)).toEqual(boolean);
		expect(resolveUnionSchema(union, 'yes')).toEqual({
			schema: { type: 'string', description: 'Whether to reply to all' },
			nullable: false,
			alternatives: ['boolean'],
			index: 1
		});
		expect(supportsGeneratedForm(union)).toBe(true);
		expect(defaultJSONSchemaValue(union)).toBe(false);

		const numeric: JSONSchema = { anyOf: [{ type: 'string' }, { type: 'integer' }], default: 5 };
		expect(resolveUnionSchema(numeric)?.schema.type).toBe('integer');
		expect(resolveUnionSchema(numeric, 2.5)?.schema.type).toBe('integer');
		expect(resolveUnionSchema({ anyOf: numeric.anyOf }, 2.5)?.schema.type).toBe('string');
		expect(resolveUnionSchema({ type: ['string', 'integer'] }, 7)).toEqual({
			schema: { type: 'integer' },
			nullable: false,
			alternatives: ['string'],
			index: 1
		});
	});

	it('only gives a member the union default when the default fits it', () => {
		const union: JSONSchema = {
			anyOf: [{ type: 'boolean', default: true }, { type: 'string' }, { type: 'null' }],
			default: 'auto'
		};
		expect(resolveUnionSchema(union, true)?.schema).toEqual({ type: 'boolean', default: true });
		expect(defaultJSONSchemaValue(resolveUnionSchema(union, true)!.schema)).toBe(true);
		expect(resolveUnionSchema(union)?.schema.default).toBe('auto');
		expect(resolveUnionSchema({ type: ['integer', 'string'], default: 'x' }, 3)?.schema).toEqual({
			type: 'integer'
		});
	});

	it('keeps the previously rendered member while the value matches no member', () => {
		const union: JSONSchema = { anyOf: [{ type: 'string' }, { type: 'integer' }] };
		expect(resolveUnionSchema(union, undefined, 1)?.schema.type).toBe('integer');
		expect(resolveUnionSchema(union, 2.5, 1)?.schema.type).toBe('integer');
		// A value matching a member still selects that member.
		expect(resolveUnionSchema(union, 'text', 1)?.schema.type).toBe('string');
		// The previous member beats the default, and an out-of-range one is ignored.
		expect(resolveUnionSchema({ ...union, default: 'a' }, undefined, 1)?.schema.type).toBe(
			'integer'
		);
		expect(resolveUnionSchema(union, undefined, 5)?.schema.type).toBe('string');
	});

	it('keeps the null toggle for unions with several non-null members', () => {
		for (const union of [
			{ anyOf: [{ type: 'boolean' }, { type: 'string' }, { type: 'null' }] },
			{ type: ['boolean', 'string', 'null'] }
		] as JSONSchema[]) {
			expect(resolveUnionSchema(union)).toEqual({
				schema: { type: 'boolean' },
				nullable: true,
				alternatives: ['string'],
				index: 0
			});
			expect(supportsGeneratedForm(union)).toBe(true);
			expect(defaultJSONSchemaValue(union)).toBe(false);
			expect(validateJSONSchema(union, null)).toEqual([]);
			expect(validateJSONSchema(union, 'text')).toEqual([]);
		}
	});

	it('prunes cleared fields using the member that matches the value', () => {
		const union: JSONSchema = {
			anyOf: [
				{ type: 'string' },
				{ type: 'object', properties: { label: { type: 'string' }, count: { type: 'integer' } } }
			]
		};
		expect(pruneClearedProperties(union, { label: '', count: 2 })).toEqual({ count: 2 });
		expect(pruneClearedProperties(union, 'text')).toBe('text');
	});

	it('falls back to the first member when the matching member cannot render', () => {
		const union: JSONSchema = {
			anyOf: [{ type: 'string' }, { type: 'object', properties: { nested: {} } }]
		};
		expect(resolveUnionSchema(union, { nested: 1 })?.schema.type).toBe('string');
		expect(supportsGeneratedForm(union)).toBe(true);
	});

	it('reports values the generated form cannot show without misrepresenting them', () => {
		const union: JSONSchema = {
			anyOf: [{ type: 'string' }, { type: 'object', properties: { nested: {} } }]
		};
		const tool: JSONSchema = { type: 'object', properties: { value: union } };
		// A valid object for the member that cannot render would show as an empty string.
		expect(generatedFormRepresents(tool, { value: { nested: 1 } })).toBe(false);
		expect(generatedFormRepresents({ type: 'array', items: union }, ['a', { nested: 1 }])).toBe(
			false
		);
		// Values the rendered member shows, unset values, and invalid values the form
		// reports as errors are all fine.
		expect(generatedFormRepresents(tool, { value: 'text' })).toBe(true);
		expect(generatedFormRepresents(tool, {})).toBe(true);
		expect(generatedFormRepresents(tool, { value: 3 })).toBe(true);
		expect(generatedFormRepresents({ anyOf: [{ type: 'string' }, { type: 'integer' }] }, 2.5)).toBe(
			true
		);
		expect(generatedFormRepresents({ type: ['string', 'null'] }, null)).toBe(true);
		// Null allowed without a null toggle would show as an empty string.
		expect(generatedFormRepresents({ anyOf: [{ type: 'string' }, { const: null }] }, null)).toBe(
			false
		);
		expect(generatedFormRepresents({ type: 'object', properties: { value: union } }, null)).toBe(
			true
		);
	});

	it('picks the same-typed member the value satisfies and represents it', () => {
		const union: JSONSchema = {
			anyOf: [
				{ type: 'object', properties: { a: { type: 'string' } }, required: ['a'] },
				{ type: 'object', properties: { b: { type: 'string' } }, required: ['b'] }
			]
		};
		expect(resolveUnionSchema(union, { b: 'x' })?.schema.required).toEqual(['b']);
		expect(resolveUnionSchema(union, { a: 'x' })?.schema.required).toEqual(['a']);
		// Mid-edit values that satisfy neither member keep the remembered same-typed
		// member, and otherwise fall back to the first type match.
		expect(resolveUnionSchema(union, {}, 1)?.schema.required).toEqual(['b']);
		expect(resolveUnionSchema(union, {})?.schema.required).toEqual(['a']);
		expect(generatedFormRepresents(union, { b: 'x' })).toBe(true);
		// A value valid only through a member the form renders differently is not.
		const typeless: JSONSchema = { anyOf: [{ type: 'string', minLength: 3 }, { enum: [5] }] };
		expect(generatedFormRepresents(typeless, 5)).toBe(false);
		expect(generatedFormRepresents(typeless, 'abc')).toBe(true);
	});

	it('leaves members ruled out by a shared enum or const out of the hint', () => {
		const members = [{ type: 'boolean' }, { type: 'string' }] as JSONSchema[];
		expect(resolveUnionSchema({ anyOf: members, enum: [true, false] })?.alternatives).toEqual([]);
		expect(resolveUnionSchema({ anyOf: members, const: true })?.alternatives).toEqual([]);
		expect(resolveUnionSchema({ anyOf: members, enum: [true, 'yes'] })?.alternatives).toEqual([
			'string'
		]);
	});

	it('still declines unions whose first member cannot render or conflicts with the union', () => {
		expect(supportsGeneratedForm({ anyOf: [{ enum: [1, 2] }, { type: 'string' }] })).toBe(false);
		expect(
			resolveUnionSchema({
				anyOf: [{ type: 'string', minLength: 2 }, { type: 'integer' }],
				minLength: 5
			})
		).toBeUndefined();
		expect(
			resolveUnionSchema({ anyOf: [{ type: 'string' }, { enum: [1, 2] }] })?.alternatives
		).toEqual(['another type']);
	});

	it.each([
		{ type: 'string', constraints: { minLength: 3 }, invalid: 'x', valid: 'abc' },
		{ type: 'number', constraints: { minimum: 3 }, invalid: 2, valid: 3 },
		{ type: 'array', constraints: { minItems: 1 }, invalid: [], valid: ['x'] },
		{ type: 'object', constraints: { required: ['name'] }, invalid: {}, valid: { name: 'x' } },
		{
			type: 'object',
			constraints: { properties: { name: { minLength: 3 } } },
			invalid: { name: 'x' },
			valid: { name: 'abc' }
		}
	])(
		'enforces type-specific anyOf sibling constraints: %j',
		({ type, constraints, invalid, valid }) => {
			const nullable: JSONSchema = {
				anyOf: [{ type }, { type: 'null' }],
				...constraints
			};
			expect(validateJSONSchema(nullable, invalid)).not.toEqual([]);
			expect(validateJSONSchema(nullable, valid)).toEqual([]);
			expect(validateJSONSchema(nullable, null)).toEqual([]);
			// These keywords constrain matching values; they do not require that type.
			expect(validateJSONSchema(constraints, false)).toEqual([]);
		}
	);

	it.each([
		{ siblingMinimum: 2, branchMinimum: 5 },
		{ siblingMinimum: 5, branchMinimum: 2 }
	])(
		'retains overlapping sibling and branch constraints: %j',
		({ siblingMinimum, branchMinimum }) => {
			const nullable: JSONSchema = {
				anyOf: [{ type: 'number', minimum: branchMinimum }, { type: 'null' }],
				minimum: siblingMinimum
			};
			expect(validateJSONSchema(nullable, 3)).not.toEqual([]);
			expect(validateJSONSchema(nullable, 5)).toEqual([]);
			expect(validateJSONSchema(nullable, null)).toEqual([]);
		}
	);

	it.each([{ enum: [null] }, { const: null }])(
		'keeps null-only nullable schemas in Raw JSON: %j',
		(constraint) => {
			for (const union of [
				{ anyOf: [{ type: 'string' }, { type: 'null' }] },
				{ type: ['string', 'null'] }
			]) {
				const nullable: JSONSchema = { ...union, ...constraint };
				expect(supportsGeneratedForm(nullable)).toBe(false);
				expect(defaultJSONSchemaValue(nullable)).toBeNull();
				expect(validateJSONSchema(nullable, null)).toEqual([]);
				expect(validateJSONSchema(nullable, 'x')).not.toEqual([]);
			}
		}
	);

	it.each([
		{
			branch: { type: 'object', properties: { first: { type: 'string' } } },
			sibling: { properties: { second: { type: 'string' } } }
		},
		{ branch: { type: 'object', required: ['first'] }, sibling: { required: ['second'] } },
		{
			branch: { type: 'array', items: { type: 'string', minLength: 3 } },
			sibling: { items: { type: 'string', maxLength: 5 } }
		},
		{ branch: { type: 'number', minimum: 5 }, sibling: { minimum: 2 } },
		{ branch: { type: 'string', enum: ['first'] }, sibling: { enum: ['second', null] } }
	])(
		'declines nullable forms that would overwrite branch constraints: %j',
		({ branch, sibling }) => {
			const nullable: JSONSchema = { anyOf: [branch, { type: 'null' }], ...sibling };
			expect(resolveUnionSchema(nullable)).toBeUndefined();
			expect(supportsGeneratedForm(nullable)).toBe(false);
		}
	);

	it('retains identical constraints and allows union metadata to override branch metadata', () => {
		const nullable: JSONSchema = {
			anyOf: [
				{ type: 'string', title: 'Branch', description: 'Branch hint', minLength: 3 },
				{ type: 'null' }
			],
			title: 'Union',
			description: 'Union hint',
			minLength: 3
		};
		expect(supportsGeneratedForm(nullable)).toBe(true);
		expect(resolveUnionSchema(nullable)).toEqual({
			schema: { type: 'string', title: 'Union', description: 'Union hint', minLength: 3 },
			nullable: true,
			alternatives: [],
			index: 0
		});
	});

	it.each([{ enum: ['allowed'] }, { const: 'allowed' }])(
		'declines nullable forms when constraints forbid null: %j',
		(constraint) => {
			for (const union of [
				{ anyOf: [{ type: 'string' }, { type: 'null' }] },
				{ type: ['string', 'null'] }
			]) {
				const nullable: JSONSchema = { ...union, ...constraint };
				expect(resolveUnionSchema(nullable)).toBeUndefined();
				expect(supportsGeneratedForm(nullable)).toBe(false);
				expect(validateJSONSchema(nullable, 'allowed')).toEqual([]);
				expect(validateJSONSchema(nullable, null)).not.toEqual([]);
			}
		}
	);

	it('checks null branch constraints without applying non-null branch constraints to null', () => {
		expect(supportsGeneratedForm({ anyOf: [{ type: 'string' }, { type: 'null', enum: [] }] })).toBe(
			false
		);
		expect(
			supportsGeneratedForm({ anyOf: [{ type: 'string', enum: ['allowed'] }, { type: 'null' }] })
		).toBe(true);
		expect(supportsGeneratedForm({ type: ['string', 'null'], enum: ['allowed', null] })).toBe(true);
	});

	it('compares const and enum JSON values structurally', () => {
		const objectConst: JSONSchema = {
			type: 'object',
			const: { enabled: true, nested: ['one', { count: 2 }] }
		};
		const arrayEnum: JSONSchema = {
			type: 'array',
			enum: [['one', { count: 2 }]]
		};

		expect(validateJSONSchema(objectConst, defaultJSONSchemaValue(objectConst))).toEqual([]);
		expect(
			validateJSONSchema(objectConst, { nested: ['one', { count: 2 }], enabled: true })
		).toEqual([]);
		expect(
			validateJSONSchema(objectConst, { enabled: false, nested: ['one', { count: 2 }] })
		).toEqual(['Value must equal {"enabled":true,"nested":["one",{"count":2}]}']);
		expect(validateJSONSchema(arrayEnum, defaultJSONSchemaValue(arrayEnum))).toEqual([]);
		expect(jsonValuesEqual(['one', { count: 2 }], ['one', { count: 3 }])).toBe(false);
	});

	it('validates optional properties the user typed into and then cleared', () => {
		const optional: JSONSchema = {
			type: 'object',
			properties: { nickname: { type: 'string', minLength: 3 } }
		};

		expect(validateJSONSchema(optional, { nickname: '' })).toEqual([
			'nickname must contain at least 3 characters'
		]);
		expect(pruneClearedProperties(optional, { nickname: '' })).toEqual({});
		expect(
			validateJSONSchema(optional, pruneClearedProperties(optional, { nickname: '' }))
		).toEqual([]);
	});

	it('prunes cleared text inputs for untyped properties but keeps empty strings for other types', () => {
		const optional: JSONSchema = {
			type: 'object',
			properties: {
				untyped: { description: 'Rendered as a text input' },
				union: { anyOf: [{ type: 'string' }, { type: 'number' }] },
				count: { type: 'number' }
			}
		};

		expect(pruneClearedProperties(optional, { untyped: '', union: '', count: '' })).toEqual({
			count: ''
		});
	});

	it('keeps cleared required properties so they report as required, and prunes nested optionals', () => {
		const nested: JSONSchema = {
			type: 'object',
			required: ['name'],
			properties: {
				name: { type: 'string', minLength: 3 },
				settings: {
					type: 'object',
					properties: { note: { type: 'string', minLength: 3 } }
				},
				tags: { type: 'array', items: { type: 'string' } }
			}
		};
		const pruned = pruneClearedProperties(nested, {
			name: '',
			settings: { note: '' },
			tags: ['']
		});

		expect(pruned).toEqual({ name: '', settings: {}, tags: [''] });
		expect(validateJSONSchema(nested, pruned)).toEqual(['name is required']);
	});

	it('accepts decimal multiples that binary floating point would reject', () => {
		const tenths: JSONSchema = { type: 'number', multipleOf: 0.1 };

		expect(0.3 % 0.1).not.toBe(0);
		for (const value of [0.1, 0.3, 0.7, 1.1, 12.3, -0.3]) {
			expect(validateJSONSchema(tenths, value)).toEqual([]);
		}
		expect(validateJSONSchema(tenths, 0.35)).toEqual(['Value must be a multiple of 0.1']);
		expect(validateJSONSchema({ type: 'integer', multipleOf: 3 }, 7)).toEqual([
			'Value must be a multiple of 3'
		]);
		expect(validateJSONSchema({ type: 'number', multipleOf: 0 }, 5)).toEqual([]);
	});
});
