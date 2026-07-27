// Generic MDM form helpers. Field and target metadata comes from
// the selected MDM asset; nothing platform-specific lives in the UI.
import type { MDMAssetField, MDMAssetFields } from '$lib/services';

export function defaultMDMValues(fields: MDMAssetFields): Record<string, unknown> {
	const values: Record<string, unknown> = {};
	for (const [name, field] of Object.entries(fields.properties ?? {})) {
		if (!field.readOnly && !field.hidden && field.default !== undefined) {
			values[name] = field.default;
		}
	}
	return values;
}

export function editableMDMValues(
	fields: MDMAssetFields,
	source: Record<string, unknown>
): Record<string, unknown> {
	const values = defaultMDMValues(fields);
	for (const [name, field] of Object.entries(fields.properties ?? {})) {
		if (!field.readOnly && !field.hidden && source[name] !== undefined) {
			values[name] = source[name];
		}
	}
	return values;
}

export function mdmFieldProblem(
	name: string,
	field: MDMAssetField,
	value: unknown,
	required: Set<string>
): string | undefined {
	if (value === undefined || value === null || value === '') {
		return required.has(name) ? 'Required.' : undefined;
	}
	if (field.type !== 'integer' && field.type !== 'number') return;

	const numeric = Number(value);
	if (Number.isNaN(numeric)) return 'Must be a number.';
	if (field.type === 'integer' && !Number.isInteger(numeric)) return 'Must be a whole number.';
	if (field.minimum !== undefined && numeric < field.minimum) {
		return `Must be at least ${field.minimum}.`;
	}
	if (field.maximum !== undefined && numeric > field.maximum) {
		return `Must be at most ${field.maximum}.`;
	}
}

export function submittedMDMValues(values: Record<string, unknown>): Record<string, unknown> {
	return Object.fromEntries(
		Object.entries(values).filter(
			([, value]) => value !== undefined && value !== null && value !== ''
		)
	);
}
