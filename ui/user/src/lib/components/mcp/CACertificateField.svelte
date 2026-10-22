<script lang="ts">
	import { m } from '$lib/i18n';
	import { isValidCACertificate } from '$lib/services/user/mcp';
	import type { Runtime } from '$lib/services/user/types';
	import Label from './CatalogFormLabel.svelte';

	interface Props {
		id: string;
		errorId: string;
		value?: string;
		runtime?: Runtime;
		hasCommand?: boolean;
		readonly?: boolean;
		showInvalid?: boolean;
	}

	let {
		id,
		errorId,
		value = $bindable(''),
		runtime,
		hasCommand = false,
		readonly = false,
		showInvalid = false
	}: Props = $props();

	const invalid = $derived(showInvalid || !isValidCACertificate(value));
</script>

<div class="flex flex-col gap-1">
	<Label title={m.mcps_catalog_ca_certificate_label()} forInput={id} showError={invalid} />
	<textarea
		{id}
		class="text-input-filled bg-base-100 w-full font-mono shadow-none"
		rows="6"
		disabled={readonly}
		bind:value
		placeholder={m.mcps_catalog_ca_certificate_placeholder()}
		aria-invalid={invalid}
		aria-describedby={invalid ? errorId : undefined}></textarea>
	{#if invalid}
		<p id={errorId} class="text-xs text-error" role="alert">
			{m.mcps_catalog_ca_certificate_error()}
		</p>
	{:else}
		<p class="text-xs text-muted-content">{m.mcps_catalog_ca_certificate_help()}</p>
	{/if}
	{#if runtime === 'containerized' && !hasCommand}
		<p class="text-xs text-muted-content">{m.mcps_catalog_ca_certificate_trust_store_note()}</p>
	{/if}
</div>
