<script lang="ts">
	import { getLocale, m } from '$lib/i18n';
	import JsonSchemaField from './JsonSchemaField.svelte';
	import { defaultJSONSchemaValue, resolveUnionSchema, type JSONSchema } from './json-schema';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';

	interface Props {
		schema: JSONSchema;
		value: unknown;
		label: string;
		path: string;
		required?: boolean;
		disabled?: boolean;
		// IDs of text outside this field that describes its control, such as an
		// enclosing union's hint.
		describedBy?: string;
		onchange: (value: unknown) => void;
		// Parse errors are separate from values so malformed text cannot select
		// another union member or make an optional field appear unset.
		onparseerrorchange?: (fieldID: string, invalid: boolean) => void;
	}

	let {
		schema,
		value,
		label,
		path,
		required = false,
		disabled = false,
		describedBy,
		onchange,
		onparseerrorchange
	}: Props = $props();
	// Derived from a per-instance unique id rather than `path`: `-` is already the path
	// separator, so sibling `a-b` and nested `a`/`b` would otherwise share a DOM id.
	const uid = $props.id();
	const id = `mcp-tester-field-${uid}`;
	// The member last rendered for this schema, kept so that clearing or retyping a
	// value that momentarily matches no member does not swap the control under the user.
	let chosen = $state.raw<{ schema: JSONSchema; index: number }>();
	let union = $derived(
		resolveUnionSchema(schema, value, chosen?.schema === schema ? chosen.index : undefined)
	);
	$effect(() => {
		if (union && (chosen?.schema !== schema || chosen.index !== union.index)) {
			chosen = { schema, index: union.index };
		}
	});
	let objectJSON = $state(untrack(() => JSON.stringify(value ?? {}, null, 2)));
	let objectJSONError = $state<string>();
	let lastEmittedObjectValue: string | undefined;
	let hasPendingObjectValue = false;
	$effect(() => {
		const invalid = !disabled && Boolean(objectJSONError);
		const notify = onparseerrorchange;
		untrack(() => notify?.(id, invalid));
	});
	onDestroy(() => onparseerrorchange?.(id, false));
	let enumIndex = $derived(schema.enum?.findIndex((entry) => Object.is(entry, value)) ?? -1);
	let type = $derived(
		Array.isArray(schema.type) ? schema.type.find((item) => item !== 'null') : schema.type
	);
	let alternativesId = $derived(union?.alternatives.length ? `${id}-alternatives` : undefined);
	let controlDescribedBy = $derived(
		[schema.description ? `${id}-description` : undefined, describedBy].filter(Boolean).join(' ') ||
			undefined
	);
	let textDescribedBy = $derived(
		[controlDescribedBy, schema.pattern ? `${id}-pattern` : undefined].filter(Boolean).join(' ') ||
			undefined
	);

	function objectValue(): Record<string, unknown> {
		return typeof value === 'object' && value !== null && !Array.isArray(value)
			? (value as Record<string, unknown>)
			: {};
	}

	function updateProperty(name: string, next: unknown) {
		onchange({ ...objectValue(), [name]: next });
	}

	function updateObjectJSON(next: string) {
		objectJSON = next;

		try {
			const parsed = JSON.parse(next) as unknown;
			hasPendingObjectValue = true;
			lastEmittedObjectValue = JSON.stringify(parsed);
			onchange(parsed);
			objectJSONError = undefined;
		} catch (error) {
			// Retain the last parsed value so the union keeps its object editor.
			// The separate parse-error callback prevents that value being submitted.
			objectJSONError = error instanceof Error ? error.message : m.mcps_tester_invalid_json();
		}
	}

	$effect(() => {
		const serialized = JSON.stringify(value);
		if (hasPendingObjectValue && serialized === lastEmittedObjectValue) {
			hasPendingObjectValue = false;
			return;
		}

		hasPendingObjectValue = false;
		objectJSON = JSON.stringify(value ?? {}, null, 2);
		objectJSONError = undefined;
	});

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

{#if union}
	<div class="space-y-2">
		{#if union.nullable}
			<label class="flex items-center gap-2 text-sm">
				<input
					type="checkbox"
					class="checkbox checkbox-sm"
					checked={value === null}
					{disabled}
					onchange={(event) =>
						onchange(event.currentTarget.checked ? null : defaultJSONSchemaValue(union.schema))}
				/>
				{m.mcps_tester_use_null_for({ label })}
			</label>
		{/if}
		<JsonSchemaField
			schema={union.schema}
			{value}
			{label}
			{path}
			{required}
			disabled={disabled || (union.nullable && value === null)}
			describedBy={[describedBy, alternativesId].filter(Boolean).join(' ') || undefined}
			{onchange}
			{onparseerrorchange}
		/>
		{#if alternativesId}
			<p id={alternativesId} class="text-xs text-muted-content">
				{m.mcps_tester_union_also_accepts({
					types: new Intl.ListFormat(getLocale(), { type: 'disjunction' }).format(
						union.alternatives
					)
				})}
			</p>
		{/if}
	</div>
{:else if type === 'object'}
	{#if Object.keys(schema.properties ?? {}).length === 0}
		<div class="space-y-2">
			<label for={id} class="block text-sm font-medium">{label}{required ? ' *' : ''}</label>
			{#if schema.description}
				<p id={`${id}-description`} class="text-xs text-muted-content">{schema.description}</p>
			{/if}
			<textarea
				{id}
				class="text-input-filled min-h-32 w-full font-mono text-sm"
				value={objectJSON}
				{disabled}
				aria-invalid={Boolean(objectJSONError)}
				aria-describedby={[controlDescribedBy, objectJSONError ? `${id}-error` : undefined]
					.filter(Boolean)
					.join(' ') || undefined}
				oninput={(event) => updateObjectJSON(event.currentTarget.value)}></textarea>
			{#if objectJSONError}
				<p id={`${id}-error`} class="text-sm text-error" role="alert">
					{m.mcps_tester_invalid_json_error({ error: objectJSONError })}
				</p>
			{/if}
		</div>
	{:else}
		<fieldset
			class="border-base-300 dark:border-base-400 space-y-4 rounded-lg border p-4"
			aria-describedby={describedBy}
		>
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
					{onparseerrorchange}
				/>
			{/each}
		</fieldset>
	{/if}
{:else if type === 'array'}
	<fieldset
		class="border-base-300 dark:border-base-400 space-y-3 rounded-lg border p-4"
		aria-describedby={describedBy}
	>
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
						label={m.mcps_tester_array_item({ label, index: index + 1 })}
						path={`${path}-${index}`}
						required
						{disabled}
						onchange={(next) => updateArray(index, next)}
						{onparseerrorchange}
					/>
				</div>
				<button
					type="button"
					class="btn btn-ghost btn-square btn-sm mt-7"
					onclick={() => removeArrayItem(index)}
					aria-label={m.mcps_tester_remove_array_item({ label, index: index + 1 })}
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
			<Plus class="size-4" aria-hidden="true" />
			{m.mcps_tester_add_item()}
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
				aria-describedby={controlDescribedBy}
				{disabled}
				onchange={(event) => onchange(schema.enum?.[Number(event.currentTarget.value)])}
			>
				{#if !required || enumIndex < 0}
					<option value={-1} selected={enumIndex < 0}>{m.mcps_tester_not_set()}</option>
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
				aria-describedby={controlDescribedBy}
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
				aria-describedby={controlDescribedBy}
				oninput={(event) =>
					onchange(
						event.currentTarget.value === '' ? undefined : event.currentTarget.valueAsNumber
					)}
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
					{m.mcps_tester_must_match_prefix()}<code class="break-all">{schema.pattern}</code>
				</p>
			{/if}
		{/if}
	</div>
{/if}
