// Generic MDM form helpers. Field and target metadata comes from
// the selected MDM asset; nothing platform-specific lives in the UI.
import { m } from '$lib/i18n';
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
		return required.has(name) ? m.inventory_enforcement_field_required() : undefined;
	}
	if (field.type !== 'integer' && field.type !== 'number') return;

	const numeric = Number(value);
	if (Number.isNaN(numeric)) return m.inventory_enforcement_field_must_be_number();
	if (field.type === 'integer' && !Number.isInteger(numeric))
		return m.inventory_enforcement_field_must_be_whole_number();
	if (field.minimum !== undefined && numeric < field.minimum) {
		return m.inventory_enforcement_field_min({ min: field.minimum });
	}
	if (field.maximum !== undefined && numeric > field.maximum) {
		return m.inventory_enforcement_field_max({ max: field.maximum });
	}
}

export function submittedMDMValues(values: Record<string, unknown>): Record<string, unknown> {
	return Object.fromEntries(
		Object.entries(values).filter(
			([, value]) => value !== undefined && value !== null && value !== ''
		)
	);
}
