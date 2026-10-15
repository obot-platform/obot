<script lang="ts">
	import { m } from '$lib/i18n';
	import type { OpenAPIRuntimeConfig } from '$lib/services';
	import type { OpenAPIImportDraft } from '$lib/services/openapi';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import OpenAPIRuntimeForm from './OpenAPIRuntimeForm.svelte';
	import { onMount } from 'svelte';

	let {
		onComplete,
		onCancel
	}: {
		onComplete: (draft: OpenAPIImportDraft) => void;
		onCancel: () => void;
	} = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let form = $state<ReturnType<typeof OpenAPIRuntimeForm>>();
	let config = $state<OpenAPIRuntimeConfig>({ source: { url: '' } });
	let importing = $state(false);
	let finished = false;
	const canImport = $derived(Boolean(config.source.url?.trim() || config.source.content));

	function cancel() {
		if (finished) return;
		finished = true;
		onCancel();
	}

	function submitImport() {
		if (finished || importing || !canImport) return;
		form?.importSchema();
	}
	onMount(() => dialog?.open());
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={m.mcps_openapi_import_title()}
	onClose={cancel}
	disableClickOutside
>
	<OpenAPIRuntimeForm
		bind:this={form}
		bind:config
		bind:busy={importing}
		importOnly
		hideImportButton
		onImported={(result) => {
			if (finished) return;
			finished = true;
			onComplete({ config, result });
		}}
		classes={{
			root: 'bg-transparent p-0 border-none'
		}}
		showTitle={false}
	/>
	<div class="mt-4 flex gap-4 justify-end">
		<button type="button" class="btn btn-secondary" onclick={cancel}>{m.common_cancel()}</button>
		<button
			type="button"
			class="btn btn-primary"
			onclick={submitImport}
			disabled={importing || !canImport}
		>
			{importing ? m.mcps_openapi_importing() : m.mcps_openapi_import()}
		</button>
	</div>
</ResponsiveDialog>
