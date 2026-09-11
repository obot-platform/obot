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

// A single non-null member can use an ordinary control plus a null toggle.
// Keep other unions in Raw JSON, where every member can be represented.
export function nonNullableJSONSchema(schema: JSONSchema): JSONSchema | undefined {
	let result: JSONSchema;
	if (schema.anyOf) {
		const members = schema.anyOf.filter((member) => member.type !== 'null');
		if (schema.anyOf.length !== 2 || members.length !== 1 || schema.type !== undefined) {
			return undefined;
		}
		const base = { ...schema };
		delete base.anyOf;
		result = { ...members[0], ...base };
	} else if (Array.isArray(schema.type)) {
		const members = schema.type.filter((type) => type !== 'null');
		if (schema.type.length !== 2 || members.length !== 1) return undefined;
		result = { ...schema, type: members[0] };
	} else {
		return undefined;
	}
	if (result.default === null) delete result.default;
	if (result.enum) result.enum = result.enum.filter((value) => value !== null);
	// There is no usable non-null control when constraints allow only null.
	if (result.const === null || result.enum?.length === 0) return undefined;
	return result;
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
	const nonNullable = nonNullableJSONSchema(schema);
	if (nonNullable) return defaultJSONSchemaValue(nonNullable);
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
	schema = nonNullableJSONSchema(schema) ?? schema;
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
	return path || 'Value';
}

export function validateJSONSchema(schema: JSONSchema, value: unknown, path = ''): string[] {
	const label = labelPath(path);

	if (schema.anyOf) {
		const { anyOf, ...base } = schema;
		const errors = validateJSONSchema(base, value, path);
		if (!anyOf.some((member) => validateJSONSchema(member, value, path).length === 0)) {
			errors.push(`${label} must match one of the allowed schemas`);
		}
		return errors;
	}

	// Union types are validated against each member so a nullable schema such as
	// `type: ['string', 'null']` accepts null.
	if (Array.isArray(schema.type)) {
		if (value === null) {
			return schema.type.includes('null')
				? validateJSONSchema({ ...schema, type: 'null' }, value, path)
				: [`${label} must not be null`];
		}
		const members = schema.type.filter((type) => type !== 'null');
		if (!members.length) return [`${label} must be null`];
		const attempts = members.map((type) => validateJSONSchema({ ...schema, type }, value, path));
		if (attempts.some((memberErrors) => memberErrors.length === 0)) return [];
		return attempts.length === 1
			? attempts[0]
			: [`${label} must be one of these types: ${members.join(', ')}`];
	}
	const errors: string[] = [];
	// Type-specific keywords also apply without an explicit type, including beside
	// anyOf. Use the value's type in that case so sibling and branch constraints
	// can be checked independently without merging away overlapping constraints.
	const type =
		schema.type ?? (value === null ? 'null' : Array.isArray(value) ? 'array' : typeof value);

	if (schema.const !== undefined && !jsonValuesEqual(value, schema.const)) {
		errors.push(`${label} must equal ${JSON.stringify(schema.const)}`);
	}
	if (schema.enum && !schema.enum.some((entry) => jsonValuesEqual(entry, value))) {
		errors.push(`${label} must be one of the allowed values`);
	}
	if (type === 'null') {
		return value === null ? errors : [...errors, `${label} must be null`];
	}

	if (type === 'object') {
		if (typeof value !== 'object' || value === null || Array.isArray(value)) {
			return [...errors, `${label} must be an object`];
		}
		const object = value as Record<string, unknown>;
		for (const required of schema.required ?? []) {
			if (!(required in object) || object[required] === undefined || object[required] === '') {
				errors.push(`${path ? `${path}.` : ''}${required} is required`);
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
			errors.push(`${label} must contain at least ${schema.minProperties} properties`);
		}
		if (schema.maxProperties !== undefined && count > schema.maxProperties) {
			errors.push(`${label} must contain at most ${schema.maxProperties} properties`);
		}
		return errors;
	}

	if (type === 'array') {
		if (!Array.isArray(value)) return [...errors, `${label} must be an array`];
		if (schema.minItems !== undefined && value.length < schema.minItems) {
			errors.push(`${label} must contain at least ${schema.minItems} items`);
		}
		if (schema.maxItems !== undefined && value.length > schema.maxItems) {
			errors.push(`${label} must contain at most ${schema.maxItems} items`);
		}
		if (schema.items) {
			value.forEach((item, index) => {
				errors.push(...validateJSONSchema(schema.items!, item, `${label}[${index}]`));
			});
		}
		return errors;
	}

	if (type === 'string') {
		if (typeof value !== 'string') return [...errors, `${label} must be a string`];
		if (schema.minLength !== undefined && value.length < schema.minLength) {
			errors.push(`${label} must contain at least ${schema.minLength} characters`);
		}
		if (schema.maxLength !== undefined && value.length > schema.maxLength) {
			errors.push(`${label} must contain at most ${schema.maxLength} characters`);
		}
		// schema.pattern is deliberately not evaluated, in case it could freeze the tab.
		return errors;
	}

	if (type === 'number' || type === 'integer') {
		if (typeof value !== 'number' || !Number.isFinite(value)) {
			return [...errors, `${label} must be a number`];
		}
		if (type === 'integer' && !Number.isInteger(value)) errors.push(`${label} must be an integer`);
		if (schema.minimum !== undefined && value < schema.minimum) {
			errors.push(`${label} must be at least ${schema.minimum}`);
		}
		if (schema.maximum !== undefined && value > schema.maximum) {
			errors.push(`${label} must be at most ${schema.maximum}`);
		}
		if (schema.exclusiveMinimum !== undefined && value <= schema.exclusiveMinimum) {
			errors.push(`${label} must be greater than ${schema.exclusiveMinimum}`);
		}
		if (schema.exclusiveMaximum !== undefined && value >= schema.exclusiveMaximum) {
			errors.push(`${label} must be less than ${schema.exclusiveMaximum}`);
		}
		if (schema.multipleOf !== undefined && !isMultipleOf(value, schema.multipleOf)) {
			errors.push(`${label} must be a multiple of ${schema.multipleOf}`);
		}
		return errors;
	}

	if (type === 'boolean' && typeof value !== 'boolean') {
		errors.push(`${label} must be true or false`);
	}
	return errors;
}

export function supportsGeneratedForm(schema: JSONSchema): boolean {
	if (schema.anyOf || Array.isArray(schema.type)) {
		const nonNullable = nonNullableJSONSchema(schema);
		return nonNullable !== undefined && supportsGeneratedForm(nonNullable);
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
