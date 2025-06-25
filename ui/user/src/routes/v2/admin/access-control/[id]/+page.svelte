<script lang="ts">
	import CatalogForm from '$lib/components/admin/CatalogForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { ChevronLeft } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { mcpCatalog: initialMcpCatalog } = data;
	let mcpCatalog = $state(initialMcpCatalog);
	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-8" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<CatalogForm {mcpCatalog}>
			{#snippet topContent()}
				<a
					href={`/v2/admin/access-control`}
					class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
				>
					<ChevronLeft class="size-6" />
					Back to Access Control
				</a>
			{/snippet}
		</CatalogForm>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {mcpCatalog?.displayName ?? 'MCP Catalog'}</title>
</svelte:head>
