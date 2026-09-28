<script lang="ts">
	import type { OpenAPIRuntimeConfig } from '$lib/services';
	import type { OpenAPIImportDraft } from '$lib/services/openapi';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import OpenAPIRuntimeForm from './OpenAPIRuntimeForm.svelte';
	import { onMount } from 'svelte';

	let {
		id,
		entity,
		onComplete,
		onCancel
	}: {
		id: string;
		entity: 'catalog' | 'workspace';
		onComplete: (draft: OpenAPIImportDraft) => void;
		onCancel: () => void;
	} = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let config = $state<OpenAPIRuntimeConfig>({ source: { url: '' } });
	let finished = false;

	function cancel() {
		if (finished) return;
		finished = true;
		onCancel();
	}
	onMount(() => dialog?.open());
</script>

<ResponsiveDialog
	bind:this={dialog}
	title="Import OpenAPI schema"
	onClose={cancel}
	disableClickOutside
	animate={null}
>
	<OpenAPIRuntimeForm
		{id}
		{entity}
		bind:config
		importOnly
		onImported={(result) => {
			if (finished) return;
			finished = true;
			onComplete({ config, result });
		}}
	/>
	<div class="mt-4 flex justify-end">
		<button type="button" class="btn btn-ghost" onclick={cancel}>Cancel</button>
	</div>
</ResponsiveDialog>
