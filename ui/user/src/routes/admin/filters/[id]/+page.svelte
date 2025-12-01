<script lang="ts">
	import { goto } from '$lib/url';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { fly } from 'svelte/transition';
	import type { MCPFilter } from '$lib/services/admin/types';
	import { mcpServersAndEntries, profile } from '$lib/stores';

	let { data }: { data: { filter: MCPFilter } } = $props();
	const { filter: initialFilter } = data;
	let filter = $state(initialFilter);
	const duration = PAGE_TRANSITION_DURATION;

	let title = $derived(filter?.name ?? 'Filter');
</script>

<Layout {title} showBackButton>
	<div class="h-full w-full" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<FilterForm
			{filter}
			onUpdate={() => {
				goto('/admin/filters');
			}}
			mcpEntriesContextFn={() => mcpServersAndEntries.current}
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
