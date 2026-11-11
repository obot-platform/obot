<script lang="ts">
	import { page } from '$app/state';
	import ConfigureProviderDialog from '$lib/components/llm-gateway/ConfigureProviderDialog.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { providerLabel } from '$lib/services/llm-gateway/providers';
	import { goto } from '$lib/url';
	import { onMount } from 'svelte';

	const PROVIDERS_URL = '/models?view=providers';

	let name = $derived(decodeURIComponent(page.params.name ?? ''));
	let dialog = $state<ReturnType<typeof ConfigureProviderDialog>>();
	let opening = $state(true);

	onMount(async () => {
		try {
			if (name) await dialog?.open(name);
		} finally {
			opening = false;
		}
	});
</script>

<svelte:head>
	<title>Obot | {providerLabel(name)}</title>
</svelte:head>

{#if opening}
	<div class="flex justify-center py-16">
		<Loading />
	</div>
{/if}

<ConfigureProviderDialog bind:this={dialog} onClosed={() => goto(PROVIDERS_URL)} />
