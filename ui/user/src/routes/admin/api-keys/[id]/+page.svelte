<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { fly } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import ApiKeyDetails from '$lib/components/api-keys/ApiKeyDetails.svelte';

	let { data } = $props();
	const { apiKey } = $derived(data);
	let title = $derived(apiKey?.name ?? 'API Key');
	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if apiKey}
			<ApiKeyDetails apiKey={{ ...apiKey, prefix: `ok1-${apiKey.userId}-${apiKey.id}-*****` }} />
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
