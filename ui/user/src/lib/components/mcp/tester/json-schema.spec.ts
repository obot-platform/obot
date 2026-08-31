import {
	defaultJSONSchemaValue,
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
				'name has an invalid format',
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
});
