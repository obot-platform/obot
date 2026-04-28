<script lang="ts">
	import type { MCPFilterInput, MCPFilterResource, MCPFilterWebhookSelector } from '$lib/services';
	import FilterView from '../../FilterView.svelte';
	import { untrack } from 'svelte';

	let { data } = $props();
	let filter = $state<MCPFilterInput>(
		untrack(() => {
			const e = data.entry;
			if (!e) {
				throw new Error('Missing catalog entry');
			}
			return {
				name: e.manifest?.name || '',
				resources: [{ id: 'default', type: 'mcpCatalog' } satisfies MCPFilterResource],
				url: '',
				secret: '',
				selectors: [] as MCPFilterWebhookSelector[],
				toolName: e.manifest?.filterConfig?.toolName || '',
				allowedToMutate: true,
				mcpServerManifest: e.manifest as NonNullable<MCPFilterInput['mcpServerManifest']>,
				systemMCPServerCatalogEntryID: e.id,
				created: '',
				type: '',
				hasSecret: false,
				configured: false
			};
		})
	);
	let title = 'Create Filter';
</script>

<FilterView {title} {filter} entry={data.entry} />

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
