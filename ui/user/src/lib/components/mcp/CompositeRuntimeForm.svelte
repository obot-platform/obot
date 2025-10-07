<script lang="ts">
	import { Plus, Server, Trash2 } from 'lucide-svelte';
	import SearchMcpServers from '../admin/SearchMcpServers.svelte';
	import { onMount } from 'svelte';
	import { AdminService, type MCPCatalogEntry } from '$lib/services';
	import type { AdminMcpServerAndEntriesContext } from '$lib/context/admin/mcpServerAndEntries.svelte';

	interface Props {
		compositeConfig: { componentCatalogEntries: string[] };
		readonly?: boolean;
		catalogId?: string;
		mcpEntriesContextFn?: () => AdminMcpServerAndEntriesContext;
	}

	let { compositeConfig = $bindable(), readonly, catalogId, mcpEntriesContextFn }: Props = $props();
	let searchDialog = $state<ReturnType<typeof SearchMcpServers>>();
	let componentEntries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(false);

	// Load full catalog entry details for display
	async function loadComponentEntries() {
		if (!compositeConfig?.componentCatalogEntries || !catalogId) return;

		loading = true;
		try {
			const entries = await Promise.all(
				compositeConfig.componentCatalogEntries.map(async (entryId) => {
					try {
						return await AdminService.getMCPCatalogEntry(catalogId, entryId);
					} catch (e) {
						console.error(`Failed to load component entry ${entryId}:`, e);
						return null;
					}
				})
			);
			componentEntries = entries.filter((e): e is MCPCatalogEntry => e !== null);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadComponentEntries();
	});

	function handleAdd(mcpCatalogEntryIds: string[]) {
		if (!compositeConfig) {
			compositeConfig = { componentCatalogEntries: [] };
		}
		// Add new entries that aren't already in the list
		const newEntries = mcpCatalogEntryIds.filter(
			(id) => !compositeConfig.componentCatalogEntries.includes(id)
		);
		compositeConfig.componentCatalogEntries = [
			...compositeConfig.componentCatalogEntries,
			...newEntries
		];
	}

	function removeServer(entryId: string) {
		compositeConfig.componentCatalogEntries = compositeConfig.componentCatalogEntries.filter(
			(id) => id !== entryId
		);
	}
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<h4 class="text-sm font-semibold">Component MCP Servers</h4>

	<div class="flex flex-col gap-2">
		{#if loading}
			<div class="text-sm text-gray-500">Loading component servers...</div>
		{:else if componentEntries.length > 0}
			{#each componentEntries as entry (entry.id)}
				<div
					class="dark:bg-surface2 dark:border-surface3 flex items-center gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3"
				>
					{#if entry.manifest?.icon}
						<img src={entry.manifest.icon} alt={entry.manifest.name} class="size-8" />
					{:else}
						<Server class="size-8 text-gray-400" />
					{/if}
					<div class="flex-1">
						<div class="font-medium">{entry.manifest?.name || 'Unnamed Server'}</div>
						{#if entry.manifest?.description}
							<div class="text-sm text-gray-500 dark:text-gray-400">
								{entry.manifest.description}
							</div>
						{/if}
					</div>
					{#if !readonly}
						<button
							type="button"
							onclick={() => removeServer(entry.id)}
							class="text-red-500 hover:text-red-700"
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
				</div>
			{/each}
		{:else}
			<div class="text-sm text-gray-500 dark:text-gray-400">
				No component servers added yet. Click the button below to add servers.
			</div>
		{/if}
	</div>

	{#if !readonly}
		<button
			type="button"
			onclick={() => searchDialog?.open()}
			class="dark:bg-surface2 dark:border-surface3 dark:hover:bg-surface3 flex items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white p-2 text-sm font-medium hover:bg-gray-50"
		>
			<Plus class="size-4" />
			Add MCP Server
		</button>
	{/if}

	<p class="text-xs text-gray-500 dark:text-gray-400">
		Select one or more MCP catalog entries to combine into a single composite server. Users will
		see this as a single server with aggregated tools and resources.
	</p>
</div>

<SearchMcpServers
	bind:this={searchDialog}
	onAdd={(mcpCatalogEntryIds) => handleAdd(mcpCatalogEntryIds)}
	exclude={compositeConfig?.componentCatalogEntries}
	type="acr"
	{mcpEntriesContextFn}
/>
