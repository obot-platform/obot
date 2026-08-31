<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import JsonSchemaField from './JsonSchemaField.svelte';
	import {
		defaultJSONSchemaValue,
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
	let validationErrors = $derived(validateJSONSchema(schema, formValue));
	let validObject = $derived.by(() => {
		if (
			validationErrors.length ||
			typeof formValue !== 'object' ||
			formValue === null ||
			Array.isArray(formValue)
		) {
			return undefined;
		}
		return formValue as Record<string, unknown>;
	});

	function setMode(next: 'form' | 'raw') {
		if (next === 'raw') rawValue = JSON.stringify(formValue, null, 2);
		mode = next;
	}

	function updateRaw(next: string) {
		rawValue = next;
		try {
			formValue = JSON.parse(next) as unknown;
			rawParseError = undefined;
		} catch (error) {
			rawParseError = error instanceof Error ? error.message : 'Invalid JSON';
		}
	}

	$effect(() => {
		onvalidchange?.(rawParseError ? undefined : validObject);
	});
</script>

<div class="space-y-4">
	<div class="flex flex-wrap items-center gap-2" aria-label="Argument input mode">
		{#if generatedSupported}
			<button
				type="button"
				class="btn btn-sm"
				class:btn-primary={mode === 'form'}
				class:btn-ghost={mode !== 'form'}
				onclick={() => setMode('form')}>Generated form</button
			>
		{/if}
		<button
			type="button"
			class="btn btn-sm"
			class:btn-primary={mode === 'raw'}
			class:btn-ghost={mode !== 'raw'}
			onclick={() => setMode('raw')}>Raw JSON</button
		>
	</div>

	{#if mode === 'form'}
		<JsonSchemaField
			{schema}
			value={formValue}
			label="Arguments"
			path="arguments"
			required
			{disabled}
			onchange={(value) => (formValue = value)}
		/>
	{:else}
		<label for="mcp-tester-raw-arguments" class="block text-sm font-medium">Arguments JSON</label>
		<textarea
			id="mcp-tester-raw-arguments"
			class="text-input-filled min-h-40 w-full font-mono text-sm"
			value={rawValue}
			{disabled}
			aria-invalid={Boolean(rawParseError)}
			oninput={(event) => updateRaw(event.currentTarget.value)}
		></textarea>
	{/if}

	{#if rawParseError}
		<p class="text-sm text-error" role="alert">Invalid JSON: {rawParseError}</p>
	{:else if validationErrors.length}
		<ul class="list-disc space-y-1 pl-5 text-sm text-error" aria-label="Argument validation errors">
			{#each validationErrors as error (error)}
				<li>{error}</li>
			{/each}
		</ul>
	{/if}

	<details>
		<summary class="cursor-pointer text-sm font-medium">Input schema</summary>
		<JsonPreview value={schema} class="mt-2" ariaLabel="Tool input schema" />
	</details>
</div>
