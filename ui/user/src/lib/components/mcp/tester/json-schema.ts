import { m } from '$lib/i18n';

export interface JSONSchema {
	type?: string | string[];
	anyOf?: JSONSchema[];
	title?: string;
	description?: string;
	default?: unknown;
	enum?: unknown[];
	const?: unknown;
	properties?: Record<string, JSONSchema>;
	required?: string[];
	items?: JSONSchema;
	minLength?: number;
	maxLength?: number;
	pattern?: string;
	minimum?: number;
	maximum?: number;
	exclusiveMinimum?: number;
	exclusiveMaximum?: number;
	multipleOf?: number;
	minItems?: number;
	maxItems?: number;
	minProperties?: number;
	maxProperties?: number;
	format?: string;
}

export interface ResolvedUnion {
	// The member to render, merged with the union's shared constraints.
	schema: JSONSchema;
	// Whether null is also permitted, in which case the field offers a null toggle.
	nullable: boolean;
	// Types of the other non-null members, which stay reachable through Raw JSON.
	alternatives: string[];
	// Position of the rendered member among the non-null members.
	index: number;
}

function jsonTypeOf(value: unknown): string | undefined {
	if (value === undefined) return undefined;
	if (value === null) return 'null';
	if (Array.isArray(value)) return 'array';
	return typeof value;
}

function memberAcceptsValue(member: JSONSchema, value: unknown): boolean {
	const actual = jsonTypeOf(value);
	if (actual === undefined || typeof member.type !== 'string') return false;
	if (member.type === 'integer') return actual === 'number' && Number.isInteger(value);
	return member.type === actual;
}

// A shallow merge is only safe when shared constraints agree. In particular,
// replacing properties, required, or items can leave required inputs unrendered.
// Metadata may still be overridden by the union.
function mergeUnionMember(base: JSONSchema, member: JSONSchema): JSONSchema | undefined {
	if (
		(Object.keys(base) as (keyof JSONSchema)[]).some(
			(key) =>
				!['title', 'description', 'default'].includes(key) &&
				base[key] !== undefined &&
				member[key] !== undefined &&
				!jsonValuesEqual(base[key], member[key])
		)
	)
		return undefined;
	return { ...member, ...base };
}

// A union renders an ordinary control for one non-null member: the member whose
// type matches the current value, otherwise the previously rendered member (so a
// cleared or mid-edit input keeps its control), otherwise the member matching the
// schema default, otherwise the first one. Null, when permitted, is offered
// through a toggle, and the other members remain available in Raw JSON, where
// every member can be represented.
export function resolveUnionSchema(
	schema: JSONSchema,
	value?: unknown,
	previous?: number
): ResolvedUnion | undefined {
	let members: JSONSchema[];
	let candidates: (JSONSchema | undefined)[];
	let nullable: boolean;
	if (schema.anyOf) {
		if (schema.type !== undefined) return undefined;
		members = schema.anyOf.filter((member) => member.type !== 'null');
		nullable = members.length < schema.anyOf.length;
		const base = { ...schema };
		delete base.anyOf;
		// The union's null default applies initially, but the non-null control
		// should still use its own branch default when the user enables it.
		if (base.default === null) delete base.default;
		candidates = members.map((member) => mergeUnionMember(base, member));
	} else if (Array.isArray(schema.type)) {
		const types = schema.type.filter((type) => type !== 'null');
		nullable = types.length < schema.type.length;
		members = types.map((type) => ({ type }));
		candidates = types.map((type) => ({ ...schema, type }));
	} else {
		return undefined;
	}
	// There is no usable non-null control when only null is allowed.
	if (!members.length) return undefined;
	// The null toggle must represent a permitted value, including any enum/const
	// constraints on the union or its null branch.
	if (nullable && validateJSONSchema(schema, null).length) return undefined;

	const renderable = (index: number) => {
		const candidate = candidates[index];
		return candidate !== undefined && supportsGeneratedForm(candidate);
	};
	const matching = (probe: unknown) =>
		candidates.findIndex(
			(_, index) => memberAcceptsValue(members[index], probe) && renderable(index)
		);
	let index = matching(value);
	if (index < 0 && previous !== undefined && previous < candidates.length && renderable(previous))
		index = previous;
	if (index < 0) index = matching(schema.default);
	if (index < 0) index = 0;
	const candidate = candidates[index];
	if (!candidate) return undefined;
	const result = { ...candidate };
	if (result.default === null) delete result.default;
	if (result.enum) result.enum = result.enum.filter((entry) => entry !== null);
	// There is no usable non-null control when constraints allow only null.
	if (result.const === null || result.enum?.length === 0) return undefined;
	const alternatives = members
		.filter((_, memberIndex) => memberIndex !== index)
		.map((member) =>
			typeof member.type === 'string' ? member.type : m.mcps_tester_union_another_type()
		);
	return { schema: result, nullable, alternatives: [...new Set(alternatives)], index };
}

function schemaType(schema: JSONSchema): string | undefined {
	return Array.isArray(schema.type) ? schema.type.find((type) => type !== 'null') : schema.type;
}

export function jsonValuesEqual(left: unknown, right: unknown): boolean {
	if (left === right) return true;
	if (Array.isArray(left)) {
		return (
			Array.isArray(right) &&
			left.length === right.length &&
			left.every((value, index) => jsonValuesEqual(value, right[index]))
		);
	}
	if (
		typeof left !== 'object' ||
		left === null ||
		typeof right !== 'object' ||
		right === null ||
		Array.isArray(right)
	) {
		return false;
	}

	const leftObject = left as Record<string, unknown>;
	const rightObject = right as Record<string, unknown>;
	const keys = Object.keys(leftObject);
	return (
		keys.length === Object.keys(rightObject).length &&
		keys.every(
			(key) =>
				Object.prototype.hasOwnProperty.call(rightObject, key) &&
				jsonValuesEqual(leftObject[key], rightObject[key])
		)
	);
}

export function defaultJSONSchemaValue(schema: JSONSchema): unknown {
	if (schema.default !== undefined) return structuredClone(schema.default);
	if (schema.const !== undefined) return structuredClone(schema.const);
	if (schema.enum?.length) return structuredClone(schema.enum[0]);
	const union = resolveUnionSchema(schema);
	if (union) return defaultJSONSchemaValue(union.schema);
	switch (schemaType(schema)) {
		case 'object':
			return Object.fromEntries(
				Object.entries(schema.properties ?? {})
					.filter(
						([name, property]) => schema.required?.includes(name) || property.default !== undefined
					)
					.map(([name, property]) => [name, defaultJSONSchemaValue(property)])
			);
		case 'array':
			return [];
		case 'boolean':
			return false;
		case 'integer':
		case 'number':
			return schema.minimum ?? 0;
		default:
			return '';
	}
}

function isMultipleOf(value: number, multiple: number): boolean {
	if (!Number.isFinite(multiple) || multiple === 0) return true;
	const quotient = value / multiple;
	return Math.abs(quotient - Math.round(quotient)) < 1e-9 * Math.max(1, Math.abs(quotient));
}

export function pruneClearedProperties(schema: JSONSchema, value: unknown): unknown {
	schema = resolveUnionSchema(schema, value)?.schema ?? schema;
	const type = schemaType(schema);
	if (type === 'object') {
		if (typeof value !== 'object' || value === null || Array.isArray(value)) return value;
		const pruned: Record<string, unknown> = {};
		for (const [name, propertyValue] of Object.entries(value as Record<string, unknown>)) {
			const cleared = propertyValue === undefined || propertyValue === '';
			if (cleared && !schema.required?.includes(name)) continue;
			const property = schema.properties?.[name];
			pruned[name] = property ? pruneClearedProperties(property, propertyValue) : propertyValue;
		}
		return pruned;
	}
	if (type === 'array' && Array.isArray(value) && schema.items) {
		return value.map((item) => pruneClearedProperties(schema.items!, item));
	}
	return value;
}

function labelPath(path: string): string {
	return path || m.core_col_value();
}

export function validateJSONSchema(schema: JSONSchema, value: unknown, path = ''): string[] {
	const label = labelPath(path);

	if (schema.anyOf) {
		const { anyOf, ...base } = schema;
		const errors = validateJSONSchema(base, value, path);
		if (!anyOf.some((member) => validateJSONSchema(member, value, path).length === 0)) {
			errors.push(m.mcps_tester_schema_must_match_any({ label }));
		}
		return errors;
	}

	// Union types are validated against each member so a nullable schema such as
	// `type: ['string', 'null']` accepts null.
	if (Array.isArray(schema.type)) {
		if (value === null) {
			return schema.type.includes('null')
				? validateJSONSchema({ ...schema, type: 'null' }, value, path)
				: [m.mcps_tester_schema_must_not_be_null({ label })];
		}
		const members = schema.type.filter((type) => type !== 'null');
		if (!members.length) return [m.mcps_tester_schema_must_be_null({ label })];
		const attempts = members.map((type) => validateJSONSchema({ ...schema, type }, value, path));
		if (attempts.some((memberErrors) => memberErrors.length === 0)) return [];
		return attempts.length === 1
			? attempts[0]
			: [m.mcps_tester_schema_must_be_one_of_types({ label, types: members.join(', ') })];
	}
	const errors: string[] = [];
	// Type-specific keywords also apply without an explicit type, including beside
	// anyOf. Use the value's type in that case so sibling and branch constraints
	// can be checked independently without merging away overlapping constraints.
	const type =
		schema.type ?? (value === null ? 'null' : Array.isArray(value) ? 'array' : typeof value);

	if (schema.const !== undefined && !jsonValuesEqual(value, schema.const)) {
		errors.push(m.mcps_tester_schema_must_equal({ label, value: JSON.stringify(schema.const) }));
	}
	if (schema.enum && !schema.enum.some((entry) => jsonValuesEqual(entry, value))) {
		errors.push(m.mcps_tester_schema_must_be_allowed_value({ label }));
	}
	if (type === 'null') {
		return value === null ? errors : [...errors, m.mcps_tester_schema_must_be_null({ label })];
	}

	if (type === 'object') {
		if (typeof value !== 'object' || value === null || Array.isArray(value)) {
			return [...errors, m.mcps_tester_schema_must_be_object({ label })];
		}
		const object = value as Record<string, unknown>;
		for (const required of schema.required ?? []) {
			if (!(required in object) || object[required] === undefined || object[required] === '') {
				errors.push(
					m.mcps_tester_schema_is_required({ name: `${path ? `${path}.` : ''}${required}` })
				);
			}
		}
		for (const [name, property] of Object.entries(schema.properties ?? {})) {
			const propertyValue = object[name];
			if (propertyValue === undefined) continue;
			if (propertyValue === '' && schema.required?.includes(name)) continue;
			errors.push(...validateJSONSchema(property, propertyValue, path ? `${path}.${name}` : name));
		}
		const count = Object.keys(object).length;
		if (schema.minProperties !== undefined && count < schema.minProperties) {
			errors.push(m.mcps_tester_schema_min_properties({ label, count: schema.minProperties }));
		}
		if (schema.maxProperties !== undefined && count > schema.maxProperties) {
			errors.push(m.mcps_tester_schema_max_properties({ label, count: schema.maxProperties }));
		}
		return errors;
	}

	if (type === 'array') {
		if (!Array.isArray(value)) return [...errors, m.mcps_tester_schema_must_be_array({ label })];
		if (schema.minItems !== undefined && value.length < schema.minItems) {
			errors.push(m.mcps_tester_schema_min_items({ label, count: schema.minItems }));
		}
		if (schema.maxItems !== undefined && value.length > schema.maxItems) {
			errors.push(m.mcps_tester_schema_max_items({ label, count: schema.maxItems }));
		}
		if (schema.items) {
			value.forEach((item, index) => {
				errors.push(...validateJSONSchema(schema.items!, item, `${label}[${index}]`));
			});
		}
		return errors;
	}

	if (type === 'string') {
		if (typeof value !== 'string')
			return [...errors, m.mcps_tester_schema_must_be_string({ label })];
		if (schema.minLength !== undefined && value.length < schema.minLength) {
			errors.push(m.mcps_tester_schema_min_length({ label, count: schema.minLength }));
		}
		if (schema.maxLength !== undefined && value.length > schema.maxLength) {
			errors.push(m.mcps_tester_schema_max_length({ label, count: schema.maxLength }));
		}
		// schema.pattern is deliberately not evaluated, in case it could freeze the tab.
		return errors;
	}

	if (type === 'number' || type === 'integer') {
		if (typeof value !== 'number' || !Number.isFinite(value)) {
			return [...errors, m.mcps_tester_schema_must_be_number({ label })];
		}
		if (type === 'integer' && !Number.isInteger(value))
			errors.push(m.mcps_tester_schema_must_be_integer({ label }));
		if (schema.minimum !== undefined && value < schema.minimum) {
			errors.push(m.mcps_tester_schema_minimum({ label, value: schema.minimum }));
		}
		if (schema.maximum !== undefined && value > schema.maximum) {
			errors.push(m.mcps_tester_schema_maximum({ label, value: schema.maximum }));
		}
		if (schema.exclusiveMinimum !== undefined && value <= schema.exclusiveMinimum) {
			errors.push(
				m.mcps_tester_schema_exclusive_minimum({ label, value: schema.exclusiveMinimum })
			);
		}
		if (schema.exclusiveMaximum !== undefined && value >= schema.exclusiveMaximum) {
			errors.push(
				m.mcps_tester_schema_exclusive_maximum({ label, value: schema.exclusiveMaximum })
			);
		}
		if (schema.multipleOf !== undefined && !isMultipleOf(value, schema.multipleOf)) {
			errors.push(m.mcps_tester_schema_multiple_of({ label, value: schema.multipleOf }));
		}
		return errors;
	}

	if (type === 'boolean' && typeof value !== 'boolean') {
		errors.push(m.mcps_tester_schema_must_be_boolean({ label }));
	}
	return errors;
}

export function supportsGeneratedForm(schema: JSONSchema): boolean {
	if (schema.anyOf || Array.isArray(schema.type)) {
		const union = resolveUnionSchema(schema);
		return union !== undefined && supportsGeneratedForm(union.schema);
	}
	const type = schema.type;
	if (!type || !['object', 'array', 'string', 'number', 'integer', 'boolean'].includes(type)) {
		return false;
	}
	if (type === 'object') {
		return Object.values(schema.properties ?? {}).every(supportsGeneratedForm);
	}
	return type !== 'array' || !schema.items || supportsGeneratedForm(schema.items);
}
