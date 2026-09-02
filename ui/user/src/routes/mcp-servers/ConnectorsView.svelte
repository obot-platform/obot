<script lang="ts">
	import { page } from '$app/state';
	import Search from '$lib/components/Search.svelte';
	import Connectors from '$lib/components/mcp/Connectors.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { Server } from '@lucide/svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		workspaceId?: string;
	}

	let { workspaceId }: Props = $props();
	let query = $derived(page.url.searchParams.get('query') ?? '');
	const updateSearchQuery = (value: string) => {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	};
</script>

<div class="flex flex-col" in:fade={{ duration: PAGE_TRANSITION_DURATION }}>
	<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
		<div class="mb-2">
			<Search
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				value={query}
				onChange={updateSearchQuery}
				placeholder="Search servers..."
			/>
		</div>
	</div>
	<Connectors id={workspaceId} entity="workspace" {query}>
		{#snippet noDataContent()}
			<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<Server class="text-base-content/80 size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">No created MCP servers</h4>
				<p class="text-muted-content text-sm font-light">
					There are no servers available to connect to yet. <br />
					Please check back later or contact your administrator.
				</p>
			</div>
		{/snippet}
	</Connectors>
</div>
