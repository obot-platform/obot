<script lang="ts">
	import { goto } from '$app/navigation';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte.js';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import type { MCPFilter } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';

	let { data }: { data: { filter: MCPFilter } } = $props();
	const { filter: initialFilter } = data;
	let filter = $state(initialFilter);
	const duration = PAGE_TRANSITION_DURATION;
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	initMcpServerAndEntries();
	onMount(async () => {
		await fetchMcpServerAndEntries(defaultCatalogId);
	});

	let title = $derived(filter?.name ?? 'Filter');
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<FilterForm
			{filter}
			onUpdate={() => {
				goto('/admin/filters');
			}}
			mcpEntriesContextFn={getAdminMcpServerAndEntries}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
