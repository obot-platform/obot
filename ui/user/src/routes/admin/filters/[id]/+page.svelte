<script lang="ts">
	import { page } from '$app/state';
	import type { MCPFilter } from '$lib/services/admin/types';
	import { goto } from '$lib/url';
	import FilterView from '../FilterView.svelte';

	let { data }: { data: { filter: MCPFilter } } = $props();
	let { filter } = $derived(data);
	let title = $derived(filter?.name ?? 'Filter');
	let selected = $derived<string>((page.url.searchParams.get('view') as string) || 'configuration');

	function handleSelectionChange(newSelection: string) {
		if (newSelection !== selected) {
			const url = new URL(window.location.href);
			url.searchParams.set('view', newSelection);
			goto(url, { replaceState: true });
		}
	}
</script>

<FilterView {title} {filter} {selected} onSelectionChange={handleSelectionChange} />

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
