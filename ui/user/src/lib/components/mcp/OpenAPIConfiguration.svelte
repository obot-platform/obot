<script lang="ts">
	import { m } from '$lib/i18n';
	import type { OpenAPIRuntimeConfig } from '$lib/services';
	import {
		type OpenAPIDestinationValidation,
		type OpenAPIImportResult,
		type OpenAPIMetadata
	} from '$lib/services/openapi';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import OpenAPIRuntimeForm from './OpenAPIRuntimeForm.svelte';
	import { untrack } from 'svelte';

	interface Props {
		config: OpenAPIRuntimeConfig;
		imported?: boolean;
		readonly?: boolean;
		showEgressDomains?: boolean;
		defaultDenyAllEgress?: boolean;
		importedBaseURL?: string;
		destinationValidation?: OpenAPIDestinationValidation;
		overrideValidation?: OpenAPIDestinationValidation;
		showBaseURLError?: boolean;
		onBaseURLChange?: () => void;
		onEditingChange?: (editing: boolean) => void;
		current: OpenAPIMetadata;
		onComplete: (
			config: OpenAPIRuntimeConfig,
			result: OpenAPIImportResult,
			metadata: OpenAPIMetadata
		) => void;
	}
	let {
		config = $bindable(),
		current,
		imported = false,
		onComplete,
		readonly = false,
		showEgressDomains = false,
		defaultDenyAllEgress = false,
		importedBaseURL = '',
		destinationValidation,
		overrideValidation,
		showBaseURLError = false,
		onBaseURLChange,
		onEditingChange
	}: Props = $props();
	let editing = $state(untrack(() => !imported));
	// Lets the parent block saving while a replacement schema import is in progress.
	$effect(() => {
		onEditingChange?.(editing);
	});
	let draft = $state<OpenAPIRuntimeConfig>(untrack(() => structuredClone($state.snapshot(config))));
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let result = $state<OpenAPIImportResult>();
	let suggestions = $state<OpenAPIMetadata>({});
	let selected = $state<OpenAPIMetadata>({});
	const fields = [
		{ key: 'name', label: m.core_name() },
		{ key: 'shortDescription', label: m.mcps_catalog_form_short_description() },
		{ key: 'description', label: m.core_description() },
		{ key: 'icon', label: m.mcps_catalog_form_icon_url() }
	] as const;
	function complete(metadata: OpenAPIMetadata) {
		if (!result) return;
		onComplete(draft, result, metadata);
		imported = true;
		result = undefined;
		editing = false;
		dialog?.close();
	}

	function handleImported(value: OpenAPIImportResult) {
		result = value;
		if (imported) {
			complete({});
			return;
		}
		suggestions = value.suggestedMetadata;
		selected = {};
		for (const { key } of fields) {
			if (!current[key]?.trim() && suggestions[key]) selected[key] = suggestions[key];
		}
		// An empty form can be prefilled immediately. Existing content is opt-in.
		if (!imported && !fields.some(({ key }) => current[key]?.trim())) {
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
		importOnly
		onImported={handleImported}
		{onBaseURLChange}
		{showBaseURLError}
	/>
	{#if imported}
		<button
			type="button"
			class="btn btn-ghost self-start"
			onclick={() => {
				editing = false;
			}}>{m.mcps_openapi_cancel_import()}</button
		>
	{/if}
{:else}
	<OpenAPIRuntimeForm
		bind:config
		{imported}
		{readonly}
		{showEgressDomains}
		{defaultDenyAllEgress}
		{importedBaseURL}
		{destinationValidation}
		{overrideValidation}
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
	title={m.mcps_openapi_metadata_title()}
	onClose={() => complete({})}
	disableClickOutside
	animate={null}
>
	{#if result}
		<p class="text-muted-content my-4 text-sm">
			{m.mcps_openapi_metadata_help()}
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
								{m.mcps_openapi_metadata_current({ value: current[key] ?? '' })}
							</p>{/if}
						<p class="mt-2 text-sm whitespace-pre-wrap break-all">
							{m.mcps_openapi_metadata_suggested({ value: suggestions[key] ?? '' })}
						</p>
					</div>
				{/if}
			{/each}
		</div>
		<div class="mt-4 flex justify-end gap-2">
			<button type="button" class="btn btn-ghost" onclick={() => complete({})}
				>{m.mcps_openapi_metadata_keep_current()}</button
			>
			<button type="button" class="btn btn-primary" onclick={() => complete(selected)}
				>{m.mcps_openapi_metadata_apply_selected()}</button
			>
		</div>
	{/if}
</ResponsiveDialog>
