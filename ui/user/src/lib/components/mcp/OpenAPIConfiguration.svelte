<script lang="ts">
	import type { OpenAPIRuntimeConfig } from '$lib/services';
	import { type OpenAPIImportResult, type OpenAPIMetadata } from '$lib/services/openapi';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import OpenAPIRuntimeForm from './OpenAPIRuntimeForm.svelte';
	import { untrack } from 'svelte';

	interface Props {
		id: string;
		entity: 'catalog' | 'workspace';
		config: OpenAPIRuntimeConfig;
		readonly?: boolean;
		importedBaseURL?: string;
		showBaseURLError?: boolean;
		onBaseURLChange?: () => void;
		current: OpenAPIMetadata;
		onComplete: (
			config: OpenAPIRuntimeConfig,
			result: OpenAPIImportResult,
			metadata: OpenAPIMetadata
		) => void;
	}
	let {
		id,
		entity,
		config = $bindable(),
		current,
		onComplete,
		readonly = false,
		importedBaseURL = '',
		showBaseURLError = false,
		onBaseURLChange
	}: Props = $props();
	let editing = $state(untrack(() => !config.schema));
	let draft = $state<OpenAPIRuntimeConfig>(untrack(() => structuredClone($state.snapshot(config))));
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let result = $state<OpenAPIImportResult>();
	let suggestions = $state<OpenAPIMetadata>({});
	let selected = $state<OpenAPIMetadata>({});
	const fields = [
		{ key: 'name', label: 'Name' },
		{ key: 'shortDescription', label: 'Short description' },
		{ key: 'description', label: 'Description' },
		{ key: 'icon', label: 'Icon URL' }
	] as const;
	function complete(metadata: OpenAPIMetadata) {
		if (!result) return;
		onComplete(draft, result, metadata);
		result = undefined;
		editing = false;
		dialog?.close();
	}

	function imported(value: OpenAPIImportResult) {
		result = value;
		suggestions = value.suggestedMetadata;
		selected = {};
		for (const { key } of fields) {
			if (!current[key]?.trim() && suggestions[key]) selected[key] = suggestions[key];
		}
		// An empty form can be prefilled immediately. Existing content is opt-in.
		if (!config.schema && !fields.some(({ key }) => current[key]?.trim())) {
			complete(suggestions);
		} else if (!Object.values(suggestions).some(Boolean)) {
			complete({});
		} else {
			dialog?.open();
		}
	}
</script>

{#if editing && !readonly}
	<OpenAPIRuntimeForm
		bind:config={draft}
		{id}
		{entity}
		importOnly
		onImported={imported}
		{onBaseURLChange}
		{showBaseURLError}
	/>
	{#if config.schema}
		<button
			type="button"
			class="btn btn-ghost self-start"
			onclick={() => {
				editing = false;
			}}>Cancel import</button
		>
	{/if}
{:else}
	<OpenAPIRuntimeForm
		bind:config
		{id}
		{entity}
		{readonly}
		{importedBaseURL}
		{showBaseURLError}
		{onBaseURLChange}
		onReplace={() => {
			draft = structuredClone($state.snapshot(config));
			editing = true;
		}}
	/>
{/if}

<ResponsiveDialog
	bind:this={dialog}
	title="Use details from schema?"
	onClose={() => complete({})}
	disableClickOutside
	animate={null}
>
	{#if result}
		<p class="text-muted-content my-4 text-sm">
			The schema is ready. Choose which details to use. Unselected fields keep their current values.
		</p>
		<div class="flex flex-col gap-4 overflow-y-auto">
			{#each fields as { key, label } (key)}
				{#if suggestions[key]}
					<div class="border-base-300 rounded-lg border p-3">
						<label class="flex items-center gap-2 text-sm font-medium">
							<input
								type="checkbox"
								class="checkbox checkbox-sm"
								checked={Boolean(selected[key])}
								onchange={(event) => {
									selected[key] = event.currentTarget.checked ? suggestions[key] : undefined;
								}}
							/>
							{label}
						</label>
						{#if current[key]}<p
								class="text-muted-content mt-2 text-xs whitespace-pre-wrap break-all"
							>
								Current: {current[key]}
							</p>{/if}
						<p class="mt-2 text-sm whitespace-pre-wrap break-all">Suggested: {suggestions[key]}</p>
					</div>
				{/if}
			{/each}
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<button type="button" class="btn btn-ghost" onclick={() => complete({})}
				>Keep current values</button
			>
			<button type="button" class="btn btn-primary" onclick={() => complete(selected)}
				>Apply selected</button
			>
		</div>
	{/if}
</ResponsiveDialog>
