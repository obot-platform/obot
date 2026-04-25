<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$lib/url';
	import FilterView from '../../../../FilterView.svelte';

	let { data } = $props();
	let { filter, entry } = $derived(data);
	let title = $derived(entry?.manifest.name ?? filter?.name ?? 'Filter');
	let selected = $derived<string>((page.url.searchParams.get('view') as string) || 'configuration');

	function handleSelectionChange(newSelection: string) {
		if (newSelection !== selected) {
			const url = new URL(window.location.href);
			url.searchParams.set('view', newSelection);
			goto(url, { replaceState: true });
		}
	}
</script>

{#if filter && entry}
	<FilterView {title} {filter} {entry} {selected} onSelectionChange={handleSelectionChange} />
{/if}

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
