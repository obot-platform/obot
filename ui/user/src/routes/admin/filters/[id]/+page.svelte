<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import type { MCPFilter } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import { fly } from 'svelte/transition';

	let { data }: { data: { filter: MCPFilter } } = $props();
	let { filter } = $derived(data);
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
			readonly={profile.current.isAdminReadonly?.()}
		/>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
