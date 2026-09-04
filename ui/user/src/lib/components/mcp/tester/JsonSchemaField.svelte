<script lang="ts">
	import JsonSchemaField from './JsonSchemaField.svelte';
	import { defaultJSONSchemaValue, type JSONSchema } from './json-schema';
	import { Plus, Trash2 } from '@lucide/svelte';

	interface Props {
		schema: JSONSchema;
		value: unknown;
		label: string;
		path: string;
		required?: boolean;
		disabled?: boolean;
		onchange: (value: unknown) => void;
	}

	let {
		schema,
		value,
		label,
		path,
		required = false,
		disabled = false,
		onchange
	}: Props = $props();
	let id = $derived(`mcp-tester-field-${path.replace(/[^a-zA-Z0-9_-]/g, '-')}`);
	let enumIndex = $derived(schema.enum?.findIndex((entry) => Object.is(entry, value)) ?? -1);
	let type = $derived(
		Array.isArray(schema.type) ? schema.type.find((item) => item !== 'null') : schema.type
	);
	let textDescribedBy = $derived(
		[
			schema.description ? `${id}-description` : undefined,
			schema.pattern ? `${id}-pattern` : undefined
		]
			.filter(Boolean)
			.join(' ') || undefined
	);

	function objectValue(): Record<string, unknown> {
		return typeof value === 'object' && value !== null && !Array.isArray(value)
			? (value as Record<string, unknown>)
			: {};
	}

	function updateProperty(name: string, next: unknown) {
		onchange({ ...objectValue(), [name]: next });
	}

	function arrayValue(): unknown[] {
		return Array.isArray(value) ? value : [];
	}

	function updateArray(index: number, next: unknown) {
		const items = [...arrayValue()];
		items[index] = next;
		onchange(items);
	}

	function removeArrayItem(index: number) {
		onchange(arrayValue().filter((_, itemIndex) => itemIndex !== index));
	}
</script>

{#if type === 'object'}
	<fieldset class="border-base-300 dark:border-base-400 space-y-4 rounded-lg border p-4">
		<legend class="px-1 text-sm font-medium">{label}{required ? ' *' : ''}</legend>
		{#if schema.description}
			<p class="text-xs text-muted-content">{schema.description}</p>
		{/if}
		{#each Object.entries(schema.properties ?? {}) as [name, property] (name)}
			<JsonSchemaField
				schema={property}
				value={objectValue()[name]}
				label={property.title || name}
				path={`${path}-${name}`}
				required={schema.required?.includes(name)}
				{disabled}
				onchange={(next) => updateProperty(name, next)}
			/>
		{/each}
		{#if Object.keys(schema.properties ?? {}).length === 0}
			<p class="text-sm text-muted-content">No declared properties.</p>
		{/if}
	</fieldset>
{:else if type === 'array'}
	<fieldset class="border-base-300 dark:border-base-400 space-y-3 rounded-lg border p-4">
		<legend class="px-1 text-sm font-medium">{label}{required ? ' *' : ''}</legend>
		{#if schema.description}
			<p class="text-xs text-muted-content">{schema.description}</p>
		{/if}
		{#each arrayValue() as item, index (`${path}-${index}`)}
			<div class="flex items-start gap-2">
				<div class="min-w-0 flex-1">
					<JsonSchemaField
						schema={schema.items ?? { type: 'string' }}
						value={item}
						label={`${label} item ${index + 1}`}
						path={`${path}-${index}`}
						required
						{disabled}
						onchange={(next) => updateArray(index, next)}
					/>
				</div>
				<button
					type="button"
					class="btn btn-ghost btn-square btn-sm mt-7"
					onclick={() => removeArrayItem(index)}
					aria-label={`Remove ${label} item ${index + 1}`}
					{disabled}
				>
					<Trash2 class="size-4" aria-hidden="true" />
				</button>
			</div>
		{/each}
		<button
			type="button"
			class="btn btn-secondary btn-sm"
			disabled={disabled ||
				(schema.maxItems !== undefined && arrayValue().length >= schema.maxItems)}
			onclick={() =>
				onchange([...arrayValue(), defaultJSONSchemaValue(schema.items ?? { type: 'string' })])}
		>
			<Plus class="size-4" aria-hidden="true" /> Add item
		</button>
	</fieldset>
{:else}
	<div class="space-y-1">
		<label for={id} class="block text-sm font-medium">{label}{required ? ' *' : ''}</label>
		{#if schema.description}
			<p id={`${id}-description`} class="text-xs text-muted-content">{schema.description}</p>
		{/if}
		{#if schema.enum}
			<select
				{id}
				class="text-input-filled w-full"
				aria-describedby={schema.description ? `${id}-description` : undefined}
				{disabled}
				onchange={(event) => onchange(schema.enum?.[Number(event.currentTarget.value)])}
			>
				{#if !required || enumIndex < 0}
					<option value={-1} selected={enumIndex < 0}>Not set</option>
				{/if}
				{#each schema.enum as option, index (index)}
					<option value={index} selected={index === enumIndex}
						>{typeof option === 'string' ? option : JSON.stringify(option)}</option
					>
				{/each}
			</select>
		{:else if type === 'boolean'}
			<input
				{id}
				type="checkbox"
				class="toggle"
				checked={value === true}
				aria-describedby={schema.description ? `${id}-description` : undefined}
				{disabled}
				onchange={(event) => onchange(event.currentTarget.checked)}
			/>
		{:else if type === 'number' || type === 'integer'}
			<input
				{id}
				type="number"
				class="text-input-filled w-full"
				value={typeof value === 'number' ? value : ''}
				min={schema.minimum}
				max={schema.maximum}
				step={type === 'integer' ? 1 : (schema.multipleOf ?? 'any')}
				{required}
				{disabled}
				aria-describedby={schema.description ? `${id}-description` : undefined}
				oninput={(event) => onchange(event.currentTarget.valueAsNumber)}
			/>
		{:else}
			<input
				{id}
				type={schema.format === 'email' ? 'email' : schema.format === 'uri' ? 'url' : 'text'}
				class="text-input-filled w-full"
				value={typeof value === 'string' ? value : ''}
				minlength={schema.minLength}
				maxlength={schema.maxLength}
				{required}
				{disabled}
				aria-describedby={textDescribedBy}
				oninput={(event) => onchange(event.currentTarget.value)}
			/>
			{#if schema.pattern}
				<!-- We don't evaluate the pattern here in case it freezes the tab. -->
				<p id={`${id}-pattern`} class="text-xs text-muted-content">
					Must match <code class="break-all">{schema.pattern}</code>
				</p>
			{/if}
		{/if}
	</div>
{/if}
