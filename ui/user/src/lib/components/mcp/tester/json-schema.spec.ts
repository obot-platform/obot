import {
	defaultJSONSchemaValue,
	jsonValuesEqual,
	nonNullableJSONSchema,
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

	it('accepts every member of a union type and leaves multiple non-null types to Raw JSON', () => {
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
		expect(supportsGeneratedForm(nullable)).toBe(false);
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
		expect(defaultJSONSchemaValue(nonNullableJSONSchema(nullable)!)).toBe(5);
		expect(defaultJSONSchemaValue(nonNullableJSONSchema({ ...nullable, default: 0 })!)).toBe(0);
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

	it('keeps other anyOf unions in Raw JSON and validates sibling constraints', () => {
		expect(supportsGeneratedForm({ anyOf: [{ type: 'string' }, { type: 'integer' }] })).toBe(false);
		expect(
			supportsGeneratedForm({
				anyOf: [{ type: 'string' }, { type: 'integer' }, { type: 'null' }]
			})
		).toBe(false);
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
			expect(nonNullableJSONSchema(nullable)).toBeUndefined();
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
		expect(nonNullableJSONSchema(nullable)).toEqual({
			type: 'string',
			title: 'Union',
			description: 'Union hint',
			minLength: 3
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
				expect(nonNullableJSONSchema(nullable)).toBeUndefined();
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
