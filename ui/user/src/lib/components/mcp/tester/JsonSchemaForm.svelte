<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import { m } from '$lib/i18n';
	import JsonSchemaField from './JsonSchemaField.svelte';
	import {
		defaultJSONSchemaValue,
		pruneClearedProperties,
		supportsGeneratedForm,
		validateJSONSchema,
		type JSONSchema
	} from './json-schema';
	import { untrack } from 'svelte';

	interface Props {
		schema: JSONSchema;
		disabled?: boolean;
		onvalidchange?: (value: Record<string, unknown> | undefined) => void;
	}

	let { schema, disabled = false, onvalidchange }: Props = $props();
	let generatedSupported = $derived(supportsGeneratedForm(schema));
	let mode = $state<'form' | 'raw'>(
		untrack(() => (supportsGeneratedForm(schema) ? 'form' : 'raw'))
	);
	let formValue = $state<unknown>(untrack(() => defaultJSONSchemaValue(schema)));
	let rawValue = $state(untrack(() => JSON.stringify(defaultJSONSchemaValue(schema), null, 2)));
	let rawParseError = $state<string>();
	let submittedValue = $derived(
		mode === 'form' ? pruneClearedProperties(schema, formValue) : formValue
	);
	let validationErrors = $derived(validateJSONSchema(schema, submittedValue));
	let validObject = $derived.by(() => {
		if (
			validationErrors.length ||
			typeof submittedValue !== 'object' ||
			submittedValue === null ||
			Array.isArray(submittedValue)
		) {
			return undefined;
		}
		return submittedValue as Record<string, unknown>;
	});

	function setMode(next: 'form' | 'raw') {
		if (next === 'raw') {
			rawValue = JSON.stringify(submittedValue, null, 2);
		} else {
			rawParseError = undefined;
		}
		mode = next;
	}

	function updateRaw(next: string) {
		rawValue = next;
		try {
			formValue = JSON.parse(next) as unknown;
			rawParseError = undefined;
		} catch (error) {
			rawParseError = error instanceof Error ? error.message : m.mcps_tester_invalid_json();
		}
	}

	$effect(() => {
		onvalidchange?.(rawParseError ? undefined : validObject);
	});
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center gap-2" aria-label={m.mcps_tester_argument_input_mode()}>
		{#if generatedSupported}
			<button
				type="button"
				class="btn btn-sm"
				class:btn-primary={mode === 'form'}
				class:btn-ghost={mode !== 'form'}
				onclick={() => setMode('form')}>{m.mcps_tester_generated_form()}</button
			>
		{/if}
		<button
			type="button"
			class="btn btn-sm"
			class:btn-primary={mode === 'raw'}
			class:btn-ghost={mode !== 'raw'}
			onclick={() => setMode('raw')}>{m.mcps_tester_raw_json()}</button
		>
	</div>

	{#if mode === 'form'}
		<JsonSchemaField
			{schema}
			value={formValue}
			label={m.mcps_runtime_arguments()}
			path="arguments"
			required
			{disabled}
			onchange={(value) => (formValue = value)}
		/>
	{:else}
		<label for="mcp-tester-raw-arguments" class="block text-sm font-medium"
			>{m.mcps_tester_arguments_json()}</label
		>
		<textarea
			id="mcp-tester-raw-arguments"
			class="text-input-filled min-h-40 w-full font-mono text-sm"
			value={rawValue}
			{disabled}
			aria-invalid={Boolean(rawParseError)}
			oninput={(event) => updateRaw(event.currentTarget.value)}></textarea>
	{/if}

	{#if rawParseError}
		<p class="text-sm text-error" role="alert">
			{m.mcps_tester_invalid_json_error({ error: rawParseError })}
		</p>
	{:else if validationErrors.length}
		<ul
			class="list-disc space-y-1 pl-5 text-sm text-error"
			aria-label={m.mcps_tester_argument_validation_errors()}
		>
			{#each validationErrors as error (error)}
				<li>{error}</li>
			{/each}
		</ul>
	{/if}

	<details>
		<summary class="cursor-pointer text-sm font-medium">{m.mcps_tester_input_schema()}</summary>
		<JsonPreview value={schema} class="mt-2" ariaLabel={m.mcps_tester_tool_input_schema()} />
	</details>
</div>
