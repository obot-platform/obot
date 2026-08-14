import type { MCPConfigurationOption, MCPSubField } from '$lib/services';

export type ConfigurationSelectOption = MCPConfigurationOption & { id: string; label: string };

export function configurationSelectOptions(
	options?: MCPConfigurationOption[]
): ConfigurationSelectOption[] {
	return (options ?? []).map((option) => ({ ...option, id: option.value, label: option.name }));
}

export function selectedConfigurationOption(
	field: Pick<MCPSubField, 'options' | 'value'>
): MCPConfigurationOption | undefined {
	return field.options?.find((option) => option.value === field.value);
}

export function isMissingRequiredConfigurationField(
	field: Pick<MCPSubField, 'options' | 'required' | 'value'>,
	editable = true
): boolean {
	if (!editable) return false;
	if (!field.value) return field.required;
	return Boolean(
		field.options?.length && !field.options.some((option) => option.value === field.value)
	);
}
