import {
	defaultJSONSchemaValue,
	jsonValuesEqual,
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

	it('accepts every member of a union type and leaves unions to Raw JSON', () => {
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
});
