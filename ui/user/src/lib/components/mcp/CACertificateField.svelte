<script lang="ts">
	import { m } from '$lib/i18n';
	import { isValidCACertificate } from '$lib/services/user/mcp';
	import type { Runtime } from '$lib/services/user/types';
	import Toggle from '../Toggle.svelte';
	import Label from './CatalogFormLabel.svelte';

	interface Props {
		id: string;
		errorId: string;
		value?: string;
		installCATrustStore?: boolean;
		runtime?: Runtime;
		hasCommand?: boolean;
		readonly?: boolean;
	}

	let {
		id,
		errorId,
		value = $bindable(''),
		installCATrustStore = $bindable(false),
		runtime,
		hasCommand = false,
		readonly = false
	}: Props = $props();

	// Live content validation only - a parent-level "some other field is invalid"
	// flag should not mark this one invalid when its own content is fine.
	const invalid = $derived(!isValidCACertificate(value));
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
	<p class="text-xs text-muted-content">{m.mcps_catalog_ca_certificate_replace_warning()}</p>
	{#if runtime === 'containerized'}
		<Toggle
			classes={{ label: 'text-inherit' }}
			disabled={readonly}
			label={m.mcps_catalog_ca_certificate_trust_store_label()}
			labelInline
			checked={installCATrustStore}
			onChange={(checked) => (installCATrustStore = checked)}
		/>
		<p class="text-xs text-muted-content">
			{m.mcps_catalog_ca_certificate_trust_store_help()}
		</p>
		{#if installCATrustStore && !hasCommand}
			<p class="text-xs text-error" role="alert">
				{m.mcps_catalog_ca_certificate_trust_store_needs_command()}
			</p>
		{/if}
	{/if}
</div>
