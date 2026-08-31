export interface JSONSchema {
	type?: string | string[];
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

function schemaType(schema: JSONSchema): string | undefined {
	return Array.isArray(schema.type) ? schema.type.find((type) => type !== 'null') : schema.type;
}

export function defaultJSONSchemaValue(schema: JSONSchema): unknown {
	if (schema.default !== undefined) return structuredClone(schema.default);
	if (schema.const !== undefined) return structuredClone(schema.const);
	if (schema.enum?.length) return structuredClone(schema.enum[0]);
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

function labelPath(path: string): string {
	return path || 'Value';
}

export function validateJSONSchema(schema: JSONSchema, value: unknown, path = ''): string[] {
	const errors: string[] = [];
	const type = schemaType(schema);
	const label = labelPath(path);

	if (schema.const !== undefined && value !== schema.const) {
		errors.push(`${label} must equal ${JSON.stringify(schema.const)}`);
	}
	if (schema.enum && !schema.enum.some((entry) => Object.is(entry, value))) {
		errors.push(`${label} must be one of the allowed values`);
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
			if (object[name] !== undefined && object[name] !== '') {
				errors.push(...validateJSONSchema(property, object[name], path ? `${path}.${name}` : name));
			}
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
		if (schema.pattern) {
			try {
				if (!new RegExp(schema.pattern).test(value)) errors.push(`${label} has an invalid format`);
			} catch {
				// The server remains authoritative when it supplies an invalid pattern.
			}
		}
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
		if (schema.multipleOf !== undefined && value % schema.multipleOf !== 0) {
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
	const type = schemaType(schema);
	if (!type || !['object', 'array', 'string', 'number', 'integer', 'boolean'].includes(type)) {
		return false;
	}
	if (type === 'object') {
		return Object.values(schema.properties ?? {}).every(supportsGeneratedForm);
	}
	return type !== 'array' || !schema.items || supportsGeneratedForm(schema.items);
}
