<script lang="ts">
	import { goto } from '$app/navigation';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import BackLink from '$lib/components/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte.js';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { browser } from '$app/environment';
	import type { MCPFilter } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';

	let { data }: { data: { filter: MCPFilter } } = $props();
	const { filter: initialFilter } = data;
	let filter = $state(initialFilter);
	const duration = PAGE_TRANSITION_DURATION;
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	let fromURL = $state('/admin/filters');

	onMount(() => {
		if (browser) {
			const urlParams = new URLSearchParams(window.location.search);
			fromURL = urlParams.get('from') || '/admin/filters';
		}
	});

	initMcpServerAndEntries();
	onMount(async () => {
		await fetchMcpServerAndEntries(defaultCatalogId);
	});
</script>

<Layout>
	<div class="my-4 h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<FilterForm
			{filter}
			onUpdate={() => {
				goto('/admin/filters');
			}}
			mcpEntriesContextFn={getAdminMcpServerAndEntries}
			readonly={profile.current.isAdminReadonly?.()}
		>
			{#snippet topContent()}
				<BackLink currentLabel={filter?.name ?? 'Filter'} {fromURL} />
			{/snippet}
		</FilterForm>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {filter?.name ?? 'Filter'}</title>
</svelte:head>
